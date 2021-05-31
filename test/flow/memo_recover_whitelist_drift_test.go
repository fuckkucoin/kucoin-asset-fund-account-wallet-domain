package flow

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	"account-assets.kucoin.net/funding/internal/kcfund"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/recon"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/shopspring/decimal"
)

// TestMemoRecoverDepositPath 错漏 Memo 自助工单：开单 → 链观察 → 审批入账 → ApplyInbound。
   func TestMemoRecoverDepositPath(t *testing.T) {
	   rt, h := router(t)
	  ctx := context.Background()
	  uid := seedUser(rt)
	  tok := rt.Cfg.Auth.BearerTokens[0]
	   call := func(method, path string, body any, role string) *httptest.ResponseRecorder {
		  var req *http.Request
		  if body != nil {
			  raw, _ := json.Marshal(body)
			   req = httptest.NewRequest(method,
				  path, bytes.NewReader(raw))
			   req.Header.Set("Content-Type", "application/json")
		   } else {
			   req = httptest.NewRequest(method, path, nil)
		  }
		  req.Header.Set("Authorization", "Bearer "+tok)
		  req.Header.Set("X-User-Id", uid)
		  if strings.Contains(path, "/ops") {
			   req.Header.Set("X-Actor", uid)
		  }
		   if role != "" {
			  req.Header.Set("X-Role", role)
		   } else if strings.Contains(path, "/ops") {
			  req.Header.Set("X-Role", "user")
		  }
		  w := httptest.NewRecorder()
		   h.ServeHTTP(w, req)
		   return w
	  }
	  to := rt.Wallet.AddressFor(ctx, uid, "ETH")
	   if to == "" {
		   to = wallet.DeriveAddress(uid, "ETH")
	   }
	   hash, err := rt.Wallet.InjectInbound("ETH", to, "USDT", decimal.RequireFromString("3.5"))
	   if err != nil {
		   t.Fatal(err)
	   }
	  w := call(http.MethodPost, "/v1/deposits/memo-recover", map[string]any{
		  "chain": "ETH", "tx_hash": hash, "memo": "recover-memo-1", "address": to,
	   }, "")
	  if w.Code != 200 {
		   t.Fatalf("open memo %d %s", w.Code, w.Body.String())
	   }
	  var opened struct {
		  Ticket fundsurf.MemoTicket `json:"ticket"`
	  }
	  if err := json.Unmarshal(w.Body.Bytes(), &opened); err != nil || opened.Ticket.ID == "" {
		   t.Fatalf("ticket parse %v %s", err, w.Body.String())
	  }
	   walBefore, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	   if err != nil {
		   t.Fatal(err)
	  }
	  w = call(http.MethodPost, "/v1/ops/memo-recover/"+opened.Ticket.ID, map[string]any{
		   "status": "approved", "asset": "USDT",
	  }, "ops")
	  if w.Code != 200 {
		  t.Fatalf("approve memo %d %s", w.Code, w.Body.String())
	   }
	  var decided struct {
		   Ticket fundsurf.MemoTicket `json:"ticket"`
	   }
	  _ = json.Unmarshal(w.Body.Bytes(),
		   &decided)
	   if decided.Ticket.Status != "approved" {
		  t.Fatalf("status %q", decided.Ticket.Status)
	   }
	   walAfter, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	   if err != nil {
		  t.Fatal(err)
	   }
	  if !walAfter.Available.Equal(walBefore.Available.Add(decimal.RequireFromString("3.5"))) {
		   t.Fatalf("wallet after memo credit before=%s after=%s", walBefore.Available, walAfter.Available)
	   }
	  // 拒绝路径不得入账
	   hash2, err := rt.Wallet.InjectInbound("ETH", to, "USDT", decimal.RequireFromString("1.1"))
	  if err != nil {
		   t.Fatal(err)
	   }
	   w = call(http.MethodPost, "/v1/deposits/memo-recover", map[string]any{
		   "chain": "ETH", "tx_hash": hash2, "memo": "recover-memo-reject",
	  }, "")
	   if w.Code != 200 {
		   t.Fatalf("open2 %d %s", w.Code, w.Body.String())
	  }
	  _ = json.Unmarshal(w.Body.Bytes(), &opened)
	   w = call(http.MethodPost, "/v1/ops/memo-recover/"+opened.Ticket.ID, map[string]any{
		   "status": "rejected",
	   }, "ops")
	  if w.Code != 200 {
		  t.Fatalf("reject %d %s", w.Code, w.Body.String())
	  }
	  walReject, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	   if err != nil {
		   t.Fatal(err)
	  }
	  if !walReject.Available.Equal(walAfter.Available) {
		   t.Fatalf("reject must not credit %s -> %s", walAfter.Available, walReject.Available)
	  }
	   // 适配器直调覆盖 BindMemo
	   dep := deposit.New(rt.Ledger, rt.Wallet)
	   ad := deposit.MemoRecoverAdapter{Svc: dep}
	  if err := ad.BindMemo(uid, "ETH", "adapter-memo"); err != nil {
		  t.Fatal(err)
	  }
  }

// TestWhitelistTipCode260325 仅收藏夹出金开启后，非收藏地址返回 tip 260325。
  func TestWhitelistTipCode260325(t *testing.T) {
	   rt, h := router(t)
	   uid := seedUser(rt)
	   tok := rt.Cfg.Auth.BearerTokens[0]
	   ext := wallet.DeriveAddress(ledger.ExtWithdrawUser, "ETH")
	   other := wallet.DeriveAddress(ledger.ExtWithdrawUser+"-other", "ETH")
	   call := func(method, path string, body any) *httptest.ResponseRecorder {
		   var req *http.Request
		  if body != nil {
			  raw, _ := json.Marshal(body)
			  req = httptest.NewRequest(method, path, bytes.NewReader(raw))
			   req.Header.Set("Content-Type", "application/json")
		  } else {
			   req = httptest.NewRequest(method,
				  path, nil)
		   }
		  req.Header.Set("Authorization", "Bearer "+tok)
		  req.Header.Set("X-User-Id", uid)
		  w := httptest.NewRecorder()
		  h.ServeHTTP(w, req)
		   return w
	  }
	   if w := call(http.MethodPost, "/v1/security/trading-password", map[string]any{"password": "123456"}); w.Code != 200 {
		  t.Fatalf("pin %d %s", w.Code, w.Body.String())
	  }
	   if w := call(http.MethodPost, "/api/v1/withdrawals/addresses", map[string]any{
		  "chain": "ETH", "address": ext, "remark": "fav",
	   }); w.Code != 200 {
		   t.Fatalf("book %d %s", w.Code, w.Body.String())
	  }
	   if w := call(http.MethodPost, "/v1/withdrawals/whitelist-mode", map[string]any{"enabled": true}); w.Code != 200 {
		  t.Fatalf("wl %d %s", w.Code, w.Body.String())
	  }
	   w := call(http.MethodPost, "/api/v3/withdrawals", map[string]any{
		  "currency": "USDT", "amount": "0.05", "chain": "ETH", "toAddress": other,
		   "clientOid": "wl-tip-1", "withdrawType": "ADDRESS", "tradePassword": "123456",
	  })
	   if w.Code == 200 {
		  t.Fatal("non-favorite must fail")
	  }
	  var body struct {
		   Code string `json:"code"`
		   Msg  string `json:"msg"`
	   }
	  _ = json.Unmarshal(w.Body.Bytes(), &body)
	  if body.Code != kcfund.CodeNotFavorite {
		   t.Fatalf("want code %s got %q body=%s", kcfund.CodeNotFavorite, body.Code, w.Body.String())
	   }
	  if !strings.Contains(body.Msg, "not favorite") {
		  t.Fatalf("msg %q", body.Msg)
	   }
	   // UTA 轨同 tip
	  w = call(http.MethodPost, "/api/ua/v2/asset/withdrawal", map[string]any{
		   "currency": "USDT", "amount": "0.05", "chain": "ETH", "toAddress": other,
		  "clientOid": "wl-tip-uta", "withdrawType": "ADDRESS", "tradePassword": "123456",
	  })
	   if w.Code == 200 {
		   t.Fatal("uta non-favorite must fail")
	  }
	   _ = json.Unmarshal(w.Body.Bytes(), &body)
	  if body.Code != kcfund.CodeNotFavorite {
		   t.Fatalf("uta want %s got %q %s", kcfund.CodeNotFavorite, body.Code, w.Body.String())
	  }
   }

// TestReconDriftEngageAndFreezeGate 漂移冻账 Engage 熔断，未关差异挡改冻结位。

  func TestReconDriftEngageAndFreezeGate(t *testing.T) {
	   rt := openMem(t)
	  ctx := context.Background()
	   uid := seedUser(rt)
	   wf := recon.WrapBook(rt.Durable.Recon, 0)
	   gate := rt.FundSurf.HaltGate()
	   act, err := recon.ApplyFreezeOnDriftWith(ctx, rt.Ledger, uid, []recon.Suggestion{
		  {Action: recon.ActFreezeUser, UserID: uid, Note: "fixture drift"},
	  }, recon.DriftHooks{WF: wf, Halt: gate})
	   if err != nil {
		  t.Fatal(err)
	  }
	   if !act.Frozen || !act.Blocked || !act.Engaged {
		  t.Fatalf("drift action %+v", act)
	  }
	   f, err := gate.Snapshot(ctx)
	   if err != nil {
		  t.Fatal(err)
	   }
	  if !f.On || f.Actor != "recon-drift" {
		  t.Fatalf("halt %+v", f)
	  }
	   if err := recon.GuardFreezeChange(wf, uid); err == nil {
		  t.Fatal("open mismatch must block freeze change")
	  }
	   frozen, err := rt.Ledger.Frozen(ctx, uid)
	   if err != nil || !frozen {
		   t.Fatalf("user frozen=%v err=%v", frozen, err)
	   }
   }
// temporary guard removed after the production check

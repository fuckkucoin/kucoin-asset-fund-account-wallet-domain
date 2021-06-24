package flow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"account-assets.kucoin.net/funding/internal/earn"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/kcfund"
	"account-assets.kucoin.net/funding/internal/ledbatch"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/seclock"
	"account-assets.kucoin.net/funding/pkg/money"
)

// TestIdentityCooldownContracts 身份约定：Passkey/邮箱/手机冷却写入与未知原因拒绝。

   func TestIdentityCooldownContracts(t *testing.T) {
	  rt, h := router(t)
	  uid := seedUser(rt)
	   tok := rt.Cfg.Auth.BearerTokens[0]
	   call := func(method, path string, body any, role string) *httptest.ResponseRecorder {
		  w := doJSON(t, rt, h, method, path, body)
		  // doJSON already auth'd; rebuild with role for ops routes
		   if role == "" {
			  return w
		  }
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
		   req.Header.Set("X-Role", role)
		   rec := httptest.NewRecorder()
		  h.ServeHTTP(rec, req)
		  return rec
	   }
	   w := call(http.MethodPost, "/v1/security/cooldown", map[string]any{
		   "user_id": uid, "reason": "Deleting Passkey",
	  }, "ops")
	   if w.Code != 200 {
		  t.Fatalf("passkey cooldown %d %s", w.Code, w.Body.String())
	   }
	  var cool struct {
		  Reason string `json:"reason"`
	   }
	   _ = json.Unmarshal(w.Body.Bytes(), &cool)
	  if cool.Reason != seclock.ReasonPasskeyDelete {
		  t.Fatalf("cooldown reason %q", cool.Reason)
	  }
	   w = call(http.MethodGet, "/v1/security/withdraw-lock", nil, "")
	   if w.Code != 200 {
		  t.Fatalf("lock %d %s", w.Code, w.Body.String())
	   }
	   var lockBody struct {
		   Lock struct {
			   Locked bool `json:"locked"`
			   Reason string `json:"reason"`
		  } `json:"lock"`
	   }
	  _ = json.Unmarshal(w.Body.Bytes(), &lockBody)
	  if !lockBody.Lock.Locked || lockBody.Lock.Reason != seclock.ReasonPasskeyDelete {
		   t.Fatalf("lock %+v", lockBody.Lock)
	  }
	   sec := seclock.New(rt.Ledger)
	   ctx := context.Background()
	  if err := sec.MarkEmailChange(ctx, uid+"-mail"); err != nil {
		  t.Fatal(err)
	   }
	  if err := sec.MarkPhoneChange(ctx, uid+"-phone"); err != nil {
		  t.Fatal(err)
	   }
	  if err := sec.MarkPasskeyDelete(ctx, uid+"-pk"); err != nil {
		  t.Fatal(err)
	  }
	   if err := sec.MarkIdentityNormalized(ctx, uid+"-norm", "Deleting Passkey"); err != nil {
		  t.Fatal(err)
	  }
	   err := sec.MarkIdentity(ctx, uid+"-bad", "random_ops_reason")
	  if err == nil || !errors.Is(err, seclock.ErrUnknownIdentityReason) {
		  t.Fatalf("want ErrUnknownIdentityReason, got %v", err)
	   }
   }

// TestHoldcatAdjacentWritePaths 红包/放贷/待提现标签写入与按 ref 释放。

  func TestHoldcatAdjacentWritePaths(t *testing.T) {
	  rt := openMem(t)
	  book := holdcat.New(rt.KV)
	  ctx := context.Background()
	   uid := seedUser(rt)
	   if _, err := book.PlaceRedEnvelope(ctx, uid, "USDT", "funding", "3", "red-1"); err != nil {
		  t.Fatal(err)
	   }
	   if _, err := book.PlaceLendingOrder(ctx, uid, "USDT", "funding", "5", "lend-1"); err != nil {
		  t.Fatal(err)
	  }
	  if _, err := book.PlacePendingWithdraw(ctx, uid, "USDT", "2", "wd-1"); err != nil {
		  t.Fatal(err)
	  }
	  if _, err := book.PlaceEarnHold(ctx, uid, "USDT", "funding", "4", "earn-1"); err != nil {
		   t.Fatal(err)
	  }
	  rows, err := book.List(ctx, uid, true)
	  if err != nil {
		  t.Fatal(err)
	  }
	   seen := map[string]bool{}
	   for _, r := range rows {
		  seen[r.Reason] = true
	  }
	   for _, want := range []string{holdcat.ReasonRedEnvelope, holdcat.ReasonLendingOrder, holdcat.ReasonPendingWithdraw, holdcat.ReasonEarnHold} {
		   if !seen[want] {
			  t.Fatalf("missing %s in %+v", want, rows)
		  }
	   }
	  if _, err := book.ReleaseByRef(ctx, uid, holdcat.ReasonRedEnvelope, "red-1"); err != nil {
		   t.Fatal(err)
	   }
	  if _, err := book.ReleaseByRef(ctx, uid, holdcat.ReasonLendingOrder, "lend-1"); err != nil {
		   t.Fatal(err)
	  }
	   if _, err := book.ReleaseByRef(ctx, uid, holdcat.ReasonPendingWithdraw, "wd-1"); err != nil {
		   t.Fatal(err)
	  }
	  if _, err := book.ReleaseByRef(ctx, uid, holdcat.ReasonEarnHold, "earn-1"); err != nil {
		   t.Fatal(err)
	  }
  }

// TestReconDoesNotAutoClearHalt recon Engage 后清扫不得自动 Clear。
  func TestReconDoesNotAutoClearHalt(t *testing.T) {
	   rt := openMem(t)
	  ctx := context.Background()
	  kcfund.BindFundsRecon(rt)
	   if rt.FundsRecon == nil {
		   t.Fatal("FundsRecon not bound")
	  }
	  gate := rt.FundSurf.HaltGate()
	  if err := gate.Engage(ctx, "test-actor", "hot inventory anomaly: fixture"); err != nil {
		  t.Fatal(err)
	   }
	   rt.FundsRecon(ctx)
	  f, err := gate.Snapshot(ctx)
	  if err != nil {
		   t.Fatal(err)
	   }
	  if !f.On {
		   t.Fatal("recon must not auto-clear custody halt")
	  }
  }

// TestHotMaxAnomalyEngageBlocksClearSweep HotMax 超限经 BindFundsRecon Engage；清扫不得 Clear。

   func TestHotMaxAnomalyEngageBlocksClearSweep(t *testing.T) {
	   rt := openMem(t)
	   ctx := context.Background()
	  if rt.Cfg == nil {
		  t.Fatal("cfg")
	  }
	  // MemStore 种子 system.bridge USDT=1000000；压低 HotMax 强制超限。
	   rt.Cfg.Transfer.HotMax = map[string]string{"USDT": "1"}
	   kcfund.BindFundsRecon(rt)
	   if rt.FundsRecon == nil {
		  t.Fatal("FundsRecon not bound")
	  }
	   gate := rt.FundSurf.HaltGate()
	   f0, err := gate.Snapshot(ctx)
	   if err != nil {
		   t.Fatal(err)
	   }
	   if f0.On {
		  t.Fatal("halt must start off")
	  }
	  rt.FundsRecon(ctx)
	   f1, err := gate.Snapshot(ctx)
	   if err != nil {
		   t.Fatal(err)
	   }
	   if !f1.On {
		  t.Fatalf("want Engage on HotMax anomaly, got %+v", f1)
	  }
	   if f1.Actor != "bind-funds-recon" {
		  t.Fatalf("actor %q", f1.Actor)
	  }
	  // 二次清扫仍不得 Clear。
	   rt.FundsRecon(ctx)
	  f2, err := gate.Snapshot(ctx)
	  if err != nil {
		  t.Fatal(err)
	   }
	  if !f2.On {
		  t.Fatal("sweep must not Clear custody halt")
	  }
  }

// TestEarnSubscribeReleasesHoldcat 申购写 earn_hold，赎回按仓位 id 释放。
   func TestEarnSubscribeReleasesHoldcat(t *testing.T) {
	  rt := openMem(t)
	  ctx := context.Background()
	   uid := seedUser(rt)
	  book := holdcat.New(rt.KV)
	   svc := earn.New(rt.Ledger).BindKV(rt.KV).BindHolds(book)
	  pos, err := svc.Subscribe(ctx, earn.SubscribeReq{
		  UserID: uid, ProductID: "flex-usdt", Asset: "USDT", Amount: "10", IdempotencyKey: "earn-hold-1",
	  })
	   if err != nil {
		  t.Fatal(err)
	  }
	   rows, err := book.List(ctx, uid, true)
	  if err != nil {
		  t.Fatal(err)
	   }
	  found := false
	   for _, r := range rows {
		   if r.Reason == holdcat.ReasonEarnHold && r.Ref == pos.ID && r.Open {
			  found = true
			   break
		   }
	  }
	  if !found {
		   t.Fatalf("missing earn_hold for %s in %+v", pos.ID, rows)
	   }
	   if _, err := svc.Redeem(ctx, earn.RedeemReq{UserID: uid, PositionID: pos.ID, IdempotencyKey: "earn-hold-red-1"}); err != nil {
		  t.Fatal(err)
	  }
	   rows, err = book.List(ctx, uid, true)
	   if err != nil {
		  t.Fatal(err)
	   }
	  for _, r := range rows {
		  if r.Reason == holdcat.ReasonEarnHold && r.Ref == pos.ID && r.Open {
			  t.Fatalf("earn_hold still open after redeem: %+v", r)
		  }
	   }
   }

// TestLedbatchSortedSubmitConserves 批次提交按用户排序且请求额守恒。
  func TestLedbatchSortedSubmitConserves(t *testing.T) {
	  rt := openMem(t)
	   ctx := context.Background()
	  uid := seedUser(rt)
	  s := ledbatch.New().BindLedger(rt.Ledger)
	   items := []ledbatch.Item{
		   {UserID: uid, Asset: "USDT", Amount: "1", Direction: string(ledger.FundingToTrading), IdempotencyKey: "dp-1"},
		   {UserID: uid, Asset: "USDT", Amount: "1", Direction: string(ledger.FundingToTrading), IdempotencyKey: "dp-2"},
	  }
	  b, err := s.Open(ledbatch.OpenReq{Actor: "test", IdempotencyKey: "dp-batch", Items: items})
	  if err != nil {
		   t.Fatal(err)
	   }
	  got, err := s.Submit(ctx, b.ID, "test")
	  if err != nil {
		   t.Fatal(err)
	  }
	  req, _ := money.ParseBook(got.Requested)
	   sub,
		   _ := money.ParseBook(got.Submitted)
	   sk, _ := money.ParseBook(got.Skipped)
	  fl, _ := money.ParseBook(got.Failed)
	  right, _ := sub.Add(sk)
	   right, _ = right.Add(fl)
	  if err := money.Conserve(req, right); err != nil {
		  t.Fatalf("conserve: %v got=%+v", err, got)
	   }
   }

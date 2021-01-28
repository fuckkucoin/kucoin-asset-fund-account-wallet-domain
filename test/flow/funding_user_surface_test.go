package flow

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/httpapi"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/shopspring/decimal"
)

   func TestFundingUserSurfaceFlows(t *testing.T) {
	   rt, err := app.OpenMem()
	   if err != nil {
		  t.Fatal(err)
	   }
	  h := httpapi.Router(rt)
	   tok := rt.Cfg.Auth.BearerTokens[0]
	  uid := rt.Cfg.SeedUserID
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
	   must := func(name string, w *httptest.ResponseRecorder) {
		   t.Helper()
		   if w.Code != 200 {
			  t.Fatalf("%s %d %s", name, w.Code, w.Body.String())
		   }
	  }
	   must("frozen", call(http.MethodGet, "/v1/assets/frozen", nil, ""))
	  must("lock", call(http.MethodGet, "/v1/security/withdraw-lock", nil, ""))
	   must("kc holds", call(http.MethodGet, "/api/v1/accounts/holds", nil, ""))
	  must("quotas", call(http.MethodGet, "/api/v1/withdrawals/quotas?currency=USDT&chain=ETH", nil, ""))
	  must("set pin", call(http.MethodPost, "/v1/security/trading-password", map[string]any{"password": "123456"}, ""))
	  must("verify pin", call(http.MethodPost, "/v1/security/trading-password/verify", map[string]any{"password": "123456"}, ""))
	  if w := call(http.MethodPost, "/v1/security/trading-password/verify", map[string]any{"password": "000000"}, ""); w.Code == 200 {
		   t.Fatal("bad pin")
	   }
	  ext := wallet.DeriveAddress(ledger.ExtWithdrawUser, "ETH")
	   must("wd with pin", call(http.MethodPost, "/api/v3/withdrawals", map[string]any{
		   "currency": "USDT", "amount": "0.11", "chain": "ETH", "toAddress": ext,
		  "clientOid": "surf-wd-1", "withdrawType": "ADDRESS", "tradePassword": "123456",
		   "accountType": "MAIN",
	   }, ""))
	  if w := call(http.MethodPost, "/api/v3/withdrawals", map[string]any{
		   "currency": "USDT", "amount": "0.11", "chain": "ETH", "toAddress": ext,
		  "clientOid": "surf-wd-bad", "withdrawType": "ADDRESS", "tradePassword": "000000",
	  }, ""); w.Code == 200 {
		   t.Fatal("wd must require pin")
	  }
	   must("book add", call(http.MethodPost, "/api/v1/withdrawals/addresses", map[string]any{
		   "chain": "ETH", "address": ext, "remark": "ext",
	   }, ""))
	  must("book list", call(http.MethodGet, "/api/v1/withdrawals/addresses", nil, ""))
	  must("wl on", call(http.MethodPost, "/v1/withdrawals/whitelist-mode", map[string]any{"enabled": true}, ""))
	  must("wd listed", call(http.MethodPost, "/api/v3/withdrawals", map[string]any{
		  "currency": "USDT", "amount": "0.12", "chain": "ETH", "toAddress": ext,
		  "clientOid": "surf-wd-2", "withdrawType": "ADDRESS", "tradePassword": "123456",
	  }, ""))
	  must("memo", call(http.MethodPost, "/v1/deposits/memo-recover", map[string]any{
		  "chain": "TON", "tx_hash": "ton-hash-1", "memo": "abc123",
	  }, ""))
	  var tkt struct {
		   Ticket struct {
			  ID string `json:"id"`
		   } `json:"ticket"`
	   }
	   _ = json.Unmarshal(call(http.MethodPost, "/api/v1/deposits/memo-recover", map[string]any{
		   "chain": "TON", "txHash": "ton-hash-2", "memo": "def456",
	   }, "").Body.Bytes(), &tkt)
	   must("forbid", call(http.MethodPost, "/v1/ops/users/withdraw-forbid", map[string]any{
		   "user_id": uid, "forbidden": true,
	   }, "ops"))
	  if w := call(http.MethodPost, "/api/v3/withdrawals", map[string]any{
		   "currency": "USDT", "amount": "0.13", "chain": "ETH", "toAddress": ext,
		   "clientOid": "surf-wd-forbid", "withdrawType": "ADDRESS", "tradePassword": "123456",
	  }, ""); w.Code == 200 {
		  t.Fatal("forbidden user")
	   }
	  must("unforbid", call(http.MethodPost, "/v1/ops/users/withdraw-forbid", map[string]any{
		   "user_id": uid, "forbidden": false,
	  }, "ops"))
	   must("convert settle", call(http.MethodPost, "/api/v3/convert", map[string]any{
		  "fromCurrency": "USDT", "toCurrency": "ETH", "fromAmount": "2", "rate": "0.0003", "accountType": "MAIN",
	   }, ""))
	   must("export", call(http.MethodPost, "/v1/ops/exports", map[string]any{"kind": "withdraw", "format": "csv"}, ""))
	  must("addr to trade", call(http.MethodPost, "/api/v3/deposit-address/create", map[string]any{
		  "currency": "USDT", "chain": "ETH", "to": "TRADE",
	  }, ""))
	   must("xfer trade", call(http.MethodPost, "/api/v3/accounts/universal-transfer", map[string]any{
		  "clientOid": "surf-xfer-1", "currency": "USDT", "amount": "1",
		  "fromAccountType": "MAIN", "toAccountType": "TRADE", "type": "INTERNAL",
	  }, ""))
	   must("add sub", call(http.MethodPost, "/api/v2/sub/user", map[string]any{
		  "subName": "desk-a",
	  }, ""))
	   must("hop out", call(http.MethodPost, "/api/v3/accounts/universal-transfer", map[string]any{
		  "clientOid": "surf-hop-1", "currency": "USDT", "amount": "0.5",
		  "type": "PARENT_TO_SUB", "toUserId": uid + "-sub-desk-a",
	  }, ""))
	  must("hop in", call(http.MethodPost, "/api/v3/accounts/universal-transfer", map[string]any{
		  "clientOid": "surf-hop-2", "currency": "USDT", "amount": "0.2",
		  "type": "SUB_TO_PARENT", "fromUserId": uid + "-sub-desk-a",
	   }, ""))
	   must("convert get", call(http.MethodGet, "/api/v3/convert", nil, ""))
	   must("change pin", call(http.MethodPost, "/v1/security/trading-password", map[string]any{
		   "old_password": "123456", "password": "654321",
	  }, ""))
	  if w := call(http.MethodPost, "/api/v3/withdrawals", map[string]any{
		   "currency": "USDT", "amount": "0.11", "chain": "ETH", "toAddress": ext,
		   "clientOid": "surf-wd-cool", "withdrawType": "ADDRESS", "tradePassword": "654321",
		  "accountType": "MAIN",
	   }, ""); w.Code == 200 {
		   t.Fatal("change pin must lock withdraw")
	   }
   }

  func TestFundingDepositToTradeAndBookHops(t *testing.T) {
	   rt, err := app.OpenMem()
	  if err != nil {
		  t.Fatal(err)
	  }
	   h := httpapi.Router(rt)
	   tok := rt.Cfg.Auth.BearerTokens[0]
	   uid := rt.Cfg.SeedUserID
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
	   if w := call(http.MethodPost, "/api/v3/deposit-address/create", map[string]any{
		   "currency": "USDT", "chain": "ETH", "to": "TRADE",
	  }); w.Code != 200 {
		  t.Fatalf("addr %d %s", w.Code, w.Body.String())
	  }
	   to := rt.Wallet.AddressFor(httptest.NewRequest(http.MethodGet, "/", nil).Context(), uid, "ETH")
	  hash, err := rt.Wallet.InjectInbound("ETH", to, "USDT", decimal.RequireFromString("4"))
	   if err != nil {
		  t.Fatal(err)
	  }
	  if w := call(http.MethodPost, "/v1/deposits/observe", map[string]any{
		   "user_id": uid, "chain": "ETH", "tx_hash": hash, "asset": "USDT",
		  "idempotency_key": "dep-to-trade-1", "wait": true,
	   }); w.Code != 200 {
		   t.Fatalf("observe %d %s", w.Code, w.Body.String())
	   }
	  w := call(http.MethodGet, "/api/v1/accounts?type=trade&currency=USDT", nil)
	   if w.Code != 200 {
		   t.Fatalf("books %d %s", w.Code, w.Body.String())
	   }
	  var env struct {
		   Data []struct {
			   Available string `json:"available"`
			  Type      string `json:"type"`
		   } `json:"data"`
	  }
	   if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		  t.Fatal(err)
	  }
	   found := false
	  for _, row := range env.Data {
		  if row.Type == "trade" && row.Available != "" && row.Available != "0" {
			  found = true
		  }
	  }
	  if !found {
		   t.Fatalf("trade book empty %s", w.Body.String())
	   }
	  for _, hop := range []struct {
		   oid, from, to string
	   }{
		   {"hop-margin-1", "MAIN", "MARGIN"},
		   {"hop-futures-1", "MAIN", "CONTRACT"},
	  } {
		  if w := call(http.MethodPost, "/api/v3/accounts/universal-transfer", map[string]any{
			  "clientOid": hop.oid, "currency": "USDT", "amount": "0.3",
			   "fromAccountType": hop.from, "toAccountType": hop.to, "type": "INTERNAL",
		   }); w.Code != 200 {
			   t.Fatalf("%s %d %s", hop.oid, w.Code, w.Body.String())
		  }
	  }
  }

  func TestFundingIdentityNoCrossUserWithdraw(t *testing.T) {
	   rt, err := app.OpenMem()
	   if err != nil {
		  t.Fatal(err)
	  }
	  h := httpapi.Router(rt)
	   tok := rt.Cfg.Auth.BearerTokens[0]
	   ext := wallet.DeriveAddress(ledger.ExtWithdrawUser, "ETH")
	  raw, _ := json.Marshal(map[string]any{
		   "currency": "USDT", "amount": "0.2", "chain": "ETH", "toAddress": ext,
		   "clientOid": "id-wd-1", "withdrawType": "ADDRESS",
	   })
	  req := httptest.NewRequest(http.MethodPost, "/api/v3/withdrawals", bytes.NewReader(raw))
	  req.Header.Set("Authorization", "Bearer "+tok)
	   req.Header.Set("Content-Type", "application/json")
	  req.Header.Set("X-User-Id", rt.Cfg.SeedUserID)
	  w := httptest.NewRecorder()
	   h.ServeHTTP(w, req)
	  if w.Code != 200 {
		   t.Fatalf("create %d %s", w.Code, w.Body.String())
	  }
	  var env struct {
		  Data struct {
			   WithdrawalID string `json:"withdrawalId"`
		   } `json:"data"`
	  }
	   _ = json.Unmarshal(w.Body.Bytes(), &env)
	  if env.Data.WithdrawalID == "" {
		   t.Fatal("id")
	  }
	   req = httptest.NewRequest(http.MethodGet, "/api/v1/withdrawals/"+env.Data.WithdrawalID, nil)
	  req.Header.Set("Authorization", "Bearer "+tok)
	   req.Header.Set("X-User-Id", "stranger-user")
	  w = httptest.NewRecorder()
	   h.ServeHTTP(w, req)
	   if w.Code == 200 {
		  t.Fatal("stranger must not read withdraw")
	   }
  }
// temporary guard removed after the production check

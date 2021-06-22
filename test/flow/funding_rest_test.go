package flow

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/httpapi"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/svcreg"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/shopspring/decimal"
)

   func TestHTTPAPIFundingRESTMounted(t *testing.T) {
	   rt, err := app.OpenMem()
	  if err != nil {
		   t.Fatal(err)
	  }
	   h := httpapi.Router(rt)
	  tok := rt.Cfg.Auth.BearerTokens[0]
	  req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	  req.Header.Set("Authorization", "Bearer "+tok)
	  w := httptest.NewRecorder()
	  h.ServeHTTP(w, req)
	  if w.Code != 200 {
		  t.Fatalf("%d %s", w.Code, w.Body.String())
	   }
	  body, _ := json.Marshal(map[string]any{
		  "clientOid": "flow-flex", "currency": "USDT", "amount": "1",
		   "fromAccountType": "MAIN", "toAccountType": "TRADE", "type": "INTERNAL",
	  })
	  req = httptest.NewRequest(http.MethodPost, "/api/v3/accounts/universal-transfer", bytes.NewReader(body))
	   req.Header.Set("Authorization", "Bearer "+tok)
	   req.Header.Set("Content-Type", "application/json")
	  w = httptest.NewRecorder()
	   h.ServeHTTP(w, req)
	   if w.Code != 200 {
		   t.Fatalf("flex %d %s", w.Code, w.Body.String())
	  }
  }

   func TestFundingFullBusinessPaths(t *testing.T) {
	   rt, err := app.OpenMem()
	  if err != nil {
		   t.Fatal(err)
	   }
	   h := httpapi.Router(rt)
	  tok := rt.Cfg.Auth.BearerTokens[0]
	  uid := rt.Cfg.SeedUserID
	  call := func(method,
		   path string, body any, role string) *httptest.ResponseRecorder {
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
	  must200 := func(name string, w *httptest.ResponseRecorder) {
		  t.Helper()
		   if w.Code != 200 {
			   t.Fatalf("%s %d %s", name, w.Code, w.Body.String())
		   }
	   }
	  must200("addr create", call(http.MethodPost, "/api/v3/deposit-address/create", map[string]any{
		  "currency": "USDT", "chain": "ETH", "to": "MAIN",
	  }, ""))
	  to := rt.Wallet.AddressFor(context.Background(), uid, "ETH")
	  hash, err := rt.Wallet.InjectInbound("ETH", to, "USDT", decimal.RequireFromString("3"))
	   if err != nil {
		  t.Fatal(err)
	  }
	   must200("observe", call(http.MethodPost, "/v1/deposits/observe", map[string]any{
		   "user_id": uid, "chain": "ETH", "tx_hash": hash, "asset": "USDT",
		   "idempotency_key": "flow-dep-1",
	  }, ""))
	   must200("dep hist", call(http.MethodGet, "/api/v1/deposits", nil, ""))
	  must200("overview", call(http.MethodGet, "/v1/assets/overview", nil, ""))
	  must200("books", call(http.MethodGet, "/v1/accounts", nil, ""))
	  must200("uta", call(http.MethodGet, "/api/ua/v1/account", nil, ""))
	   must200("uta bal", call(http.MethodGet, "/api/ua/v2/unified/account/balance", nil, ""))
	   must200("uta v1 bal", call(http.MethodGet, "/api/ua/v1/unified/account/balance", nil, ""))
	  must200("cross v3", call(http.MethodGet, "/api/v3/margin/accounts", nil, ""))
	  must200("iso v3", call(http.MethodGet, "/api/v3/isolated/accounts", nil, ""))
	  must200("uta mode", call(http.MethodGet, "/api/ua/v1/account/mode", nil, ""))
	  must200("uta lev", call(http.MethodGet, "/api/ua/v1/unified/account/leverage", nil, ""))
	  must200("borrow info", call(http.MethodGet, "/api/ua/v1/margin/borrow/info?currency=USDT", nil, ""))
	  must200("interest limits", call(http.MethodGet, "/api/ua/v1/account/interest-limits?currency=USDT", nil, ""))
	  must200("iso flex", call(http.MethodPost, "/api/v3/accounts/universal-transfer", map[string]any{
		   "clientOid": "flow-iso", "currency": "USDT", "amount": "0.4",
		  "fromAccountType": "MAIN", "toAccountType": "ISOLATED", "type": "INTERNAL",
		  "symbol": "BTC-USDT",
	   }, ""))
	   must200("flex", call(http.MethodPost, "/api/v3/accounts/universal-transfer", map[string]any{
		   "clientOid": "flow-flex-2", "currency": "USDT", "amount": "0.5",
		   "fromAccountType": "MAIN", "toAccountType": "TRADE", "type": "INTERNAL",
	   }, ""))
	   if err := rt.Users.Bind("email", "peer@example.test", config.PeerSeedUserID); err != nil {
		  t.Fatal(err)
	  }
	  must200("internal http", call(http.MethodPost, "/v1/withdrawals/internal", map[string]any{
		  "to_email": "peer@example.test", "asset": "USDT", "amount": "0.2",
		  "idempotency_key": "flow-int-1",
	   }, ""))
	   must200("inner rest", call(http.MethodPost, "/api/v3/withdrawals", map[string]any{
		   "currency": "USDT", "amount": "0.2", "isInner": true, "toEmail": "peer@example.test",
		   "clientOid": "flow-inner-2",
	  }, ""))
	   ext := wallet.DeriveAddress(ledger.ExtWithdrawUser, "ETH")
	  w := call(http.MethodPost, "/api/v3/withdrawals", map[string]any{
		   "currency": "USDT", "amount": "0.3", "chain": "ETH", "toAddress": ext,
		  "clientOid": "flow-wd-1", "withdrawType": "ADDRESS",
	   }, "")
	  must200("wd v3", w)
	   var wd struct {
		   Data struct {
			   WithdrawalID string `json:"withdrawalId"`
		   } `json:"data"`
	   }
	  _ = json.Unmarshal(w.Body.Bytes(), &wd)
	   if wd.Data.WithdrawalID != "" {
		  must200("wd get", call(http.MethodGet, "/api/v1/withdrawals/"+wd.Data.WithdrawalID, nil, ""))
		   _ = call(http.MethodDelete, "/api/v1/withdrawals/"+wd.Data.WithdrawalID, nil, "")
		   must200("broadcast", call(http.MethodPost, "/v1/withdrawals/broadcast", map[string]any{
			   "transfer_id": wd.Data.WithdrawalID, "chain": "ETH", "tx_hash": "0xflowbc1",
			   "asset": "USDT", "amount": "0.3",
		  }, "custody"))
	   }
	  must200("wd hist", call(http.MethodGet, "/api/v1/withdrawals", nil, ""))
	  must200("hist wd", call(http.MethodGet, "/api/v1/hist-withdrawals", nil, ""))
	  must200("hist dep", call(http.MethodGet, "/api/v1/hist-deposits", nil, ""))
	  must200("addr v1", call(http.MethodGet, "/api/v1/deposit-addresses?currency=USDT&chain=ETH", nil, ""))
	  must200("hf opened", call(http.MethodGet, "/api/v1/hf/accounts/opened", nil, ""))
	  must200("fut ledgers", call(http.MethodGet, "/api/v1/transaction-history", nil, ""))
	   must200("classic ua bal", call(http.MethodGet, "/api/ua/v2/account/balance?accountType=FUNDING&currency=USDT", nil, ""))
	   must200("quotas", call(http.MethodGet, "/api/v1/withdrawals/quotas?currency=USDT&chain=ETH", nil, ""))
	  must200("xfer quotas", call(http.MethodGet, "/api/v1/accounts/transferable?currency=USDT&type=MAIN", nil, ""))
	  must200("wd fee", call(http.MethodGet, "/api/v1/withdrawals/fee?currency=USDT&chain=ETH", nil, ""))
	  must200("fee", call(http.MethodGet, "/api/v1/base-fee", nil, ""))
	   must200("trade fee", call(http.MethodGet, "/api/v1/trade-fees?symbol=BTC-USDT", nil, ""))
	  must200("convert list", call(http.MethodGet, "/api/v3/convert", nil, ""))
	  must200("convert place", call(http.MethodPost, "/api/v3/convert", map[string]any{
		  "fromCurrency": "USDT", "toCurrency": "ETH", "fromAmount": "1", "rate": "0.0004", "accountType": "MAIN",
	  }, ""))
	   must200("ops convert", call(http.MethodPost, "/v1/convert", map[string]any{
		   "from_asset": "USDT", "to_asset": "BTC", "amount": "1", "rate": "0.00002", "book": "funding",
	  }, ""))
	  w = call(http.MethodPost, "/v1/ops/exports", map[string]any{"kind": "all", "format": "csv"}, "")
	  must200("export", w)
	   var job struct {
		  Job struct {
			   ID string `json:"id"`
		   } `json:"job"`
	   }
	   _ = json.Unmarshal(w.Body.Bytes(), &job)
	   if job.Job.ID != "" {
		  must200("export get", call(http.MethodGet, "/v1/ops/exports/"+job.Job.ID, nil, ""))
	  }
	  must200("sub add", call(http.MethodPost, "/api/v2/sub/user", map[string]any{"subName": "flow-desk", "remarks": "flow-desk"}, ""))
	  must200("sub list", call(http.MethodGet, "/api/v1/sub-accounts", nil, ""))
	   must200("earn buy", call(http.MethodPost, "/api/v1/earn/orders", map[string]any{
		  "productId": "flex-usdt", "currency": "USDT", "amount": "2", "clientOid": "flow-earn-1",
	  }, ""))
	  must200("earn hold", call(http.MethodGet, "/api/v1/earn/hold-assets", nil, ""))
	  must200("cooldown", call(http.MethodPost, "/v1/security/cooldown", map[string]any{
		   "user_id": uid, "reason": "password",
	   }, "ops"))
	   booksOK(t, rt, uid)
  }

   func TestFundingInterServiceLocalFakes(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		  t.Fatal(err)
	  }
	   hits := map[string]int{}
	   peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		   hits[r.URL.Path]++
		  w.Header().Set("Content-Type", "application/json")
		   _, _ = w.Write([]byte(`{"code":"200000","data":{}}`))
	  }))
	   defer peer.Close()
	   for _, name := range []string{"risk-kyt", "address-custody", "account-book", "deposit-withdraw"} {
		  if err := rt.Registry.Register(context.Background(), svcreg.Instance{Name: name, Addr: peer.URL}); err != nil {
			   t.Fatal(err)
		   }
	  }
	  h := httpapi.Router(rt)
	  tok := rt.Cfg.Auth.BearerTokens[0]
	  post := func(path string, body any) *httptest.ResponseRecorder {
		   raw, _ := json.Marshal(body)
		  req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		   req.Header.Set("Authorization", "Bearer "+tok)
		   req.Header.Set("Content-Type", "application/json")
		  req.Header.Set("X-User-Id", rt.Cfg.SeedUserID)
		  w := httptest.NewRecorder()
		   h.ServeHTTP(w, req)
		  return w
	   }
	  w := post("/api/v3/deposit-address/create", map[string]any{"currency": "USDT", "chain": "ETH", "to": "MAIN"})
	   if w.Code != 200 {
		  t.Fatalf("addr %d %s", w.Code, w.Body.String())
	  }
	  code, raw, err := rt.Dial.Call(context.Background(), "account-book", http.MethodGet, "/v1/accounts", nil)
	   if err != nil || code != 200 {
		   t.Fatalf("dial %d %s %v", code, raw, err)
	  }
	   if hits["/v1/addresses/deposit"] == 0 && hits["/v1/accounts"] == 0 {
		   t.Fatal("local peer not hit")
	   }
	  code, _, err = rt.Dial.Call(context.Background(), "missing-service", http.MethodGet, "/v1/x", nil)
	  if err == nil || code == 200 {
		   t.Fatal("missing service must not succeed")
	   }
   }

   func TestFundingLoadishCriticalPaths(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		  t.Fatal(err)
	   }
	  uid := rt.Cfg.SeedUserID
	  ctx := context.Background()
	  to := rt.Wallet.AddressFor(ctx, uid, "ETH")
	   ext := wallet.DeriveAddress(ledger.ExtWithdrawUser, "ETH")
	  for i := 0; i < 16; i++ {
		   n := itoa(i)
		   if _, err := rt.Ledger.Create(ctx, ledger.CreateReq{
			  UserID: uid, Direction: ledger.FundingToTrading, Asset: "USDT",
			  Amount: decimal.RequireFromString("0.01"), IdempotencyKey: "load-led-" + n,
		   }); err != nil {
			  t.Fatal(err)
		   }
		  hash, err := rt.Wallet.InjectInbound("ETH", to, "USDT", decimal.RequireFromString("0.01"))
		  if err != nil {
			   t.Fatal(err)
		   }
		   if _,
			   err := rt.Ledger.Create(ctx, ledger.CreateReq{
			   UserID: uid, Direction: ledger.WalletDeposit, Asset: "USDT",
			   Amount: decimal.RequireFromString("0.01"), Chain: "ETH", TxHash: hash,
			  IdempotencyKey: "load-dep-" + n,
		  }); err != nil {
			   t.Fatal(err)
		   }
		  if i < 8 {
			   if _, err := rt.Ledger.Create(ctx, ledger.CreateReq{
				  UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
				   Amount: decimal.RequireFromString("0.01"), Chain: "ETH", ToAddress: ext,
				   IdempotencyKey: "load-wd-" + n,
			  }); err != nil {
				   t.Fatal(err)
			  }
		   }
	  }
   }

  func itoa(n int) string {
	   if n == 0 {
		  return "0"
	  }
	   var b [20]byte
	   i := len(b)
	  for n > 0 {
		   i--
		   b[i] = byte('0' + n%10)
		   n /= 10
	  }
	   return string(b[i:])
  }
// temporary guard removed after the production check

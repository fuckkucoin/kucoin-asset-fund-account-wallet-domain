package flow

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/slo"
)

   func TestHTTPWalletLedgerSwapSurface(t *testing.T) {
	  rt, h := router(t)
	   uid := seedUser(rt)
	   eth := dest(uid, "ETH")
	  key := "flow-http-" + time.Now().UTC().Format("150405.000000")
	   w := doJSON(t, rt, h, "GET", "/healthz", nil)
	  if w.Code != 200 || !strings.Contains(w.Body.String(), slo.ProcessDisclaimer) {
		   t.Fatalf("healthz %s", w.Body.String())
	  }
	   w = doJSON(t, rt, h, "GET", "/", nil)
	   if w.Code != 200 || !strings.Contains(w.Body.String(), "backend API only") {
		   t.Fatalf("catalog %s", w.Body.String())
	   }
	  w = doJSON(t, rt, h, "POST", "/v1/ledger/transfers", map[string]any{
		   "user_id": uid, "direction": "spot_to_wallet", "asset": "USDT",
		   "amount": "12.5", "chain": "ETH", "to_address": eth,
		  "idempotency_key": key, "wait": true,
	  })
	  if w.Code != 200 || !strings.Contains(w.Body.String(), "credited") {
		   t.Fatalf("transfer %d %s", w.Code, w.Body.String())
	  }
	   w = doJSON(t, rt, h, "POST", "/v1/ledger/transfers", map[string]any{
		   "user_id": uid, "direction": "wallet_withdraw", "asset": "USDT",
		   "amount": "1.5", "chain": "ETH", "to_address": extDest("ETH"),
		  "idempotency_key": "wd-" + key, "wait": true,
	  })
	  if w.Code != 200 || !strings.Contains(w.Body.String(), `"settlement":"withdraw"`) {
		   t.Fatalf("withdraw %d %s", w.Code, w.Body.String())
	   }
	   w = doJSON(t, rt, h, "POST", "/v1/swap/execute", map[string]string{
		  "user_id": uid, "from_asset": "ETH", "to_asset": "USDT", "amount": "0.05",
		   "idempotency_key": "swap-" + key, "chain": "ETH",
	   })
	   if w.Code != 200 || !strings.Contains(w.Body.String(), "filled") {
		   t.Fatalf("swap %s", w.Body.String())
	   }
	  w = doJSON(t, rt, h, "GET", "/v1/ledger/reconcile?user_id="+uid, nil)
	  if w.Code != 200 || !strings.Contains(w.Body.String(), `"ok":true`) {
		   t.Fatalf("reconcile %s", w.Body.String())
	  }
	  w = doJSON(t, rt, h, "POST", "/v1/ledger/transfers", map[string]any{
		  "user_id": uid, "direction": "spot_to_wallet", "asset": "USDT",
		   "amount": "6000", "chain": "ETH", "to_address": eth,
		  "idempotency_key": "risk-" + key,
	  })
	   if w.Code != 400 || !strings.Contains(w.Body.String(), "risk") {
		   t.Fatalf("risk %d %s", w.Code, w.Body.String())
	   }
   }

   func TestHTTPRejectsMissingAuthAndIdempotency(t *testing.T) {
	   rt, h := router(t)
	  r := httptest.NewRequest(http.MethodGet, "/v1/wallet/chains", nil)
	   w := httptest.NewRecorder()
	   h.ServeHTTP(w, r)
	  if w.Code != 401 {
		  t.Fatalf("want 401 got %d %s", w.Code, w.Body.String())
	  }
	  r = httptest.NewRequest(http.MethodGet, "/v1/events", nil)
	  w = httptest.NewRecorder()
	  h.ServeHTTP(w, r)
	  if w.Code != 401 {
		  t.Fatalf("events 401 got %d %s", w.Code, w.Body.String())
	  }
	   r = httptest.NewRequest(http.MethodGet, "/v1/ops/alerts", nil)
	  w = httptest.NewRecorder()
	  h.ServeHTTP(w, r)
	   if w.Code != 401 {
		  t.Fatalf("alerts 401 got %d %s", w.Code, w.Body.String())
	  }
	  uid := seedUser(rt)
	  w = doJSON(t, rt, h, "POST", "/v1/ledger/transfers", map[string]any{
		  "user_id": uid, "direction": "spot_to_wallet", "asset": "USDT",
		  "amount": "1", "chain": "ETH", "to_address": dest(uid, "ETH"),
	  })
	  if w.Code != 400 || !strings.Contains(w.Body.String(), "idempotency_key") {
		  t.Fatalf("missing key %d %s", w.Code, w.Body.String())
	  }
  }

  func TestHTTPMarketAndSLOSurface(t *testing.T) {
	   rt, h := router(t)
	   w := doJSON(t, rt, h, "GET", "/v1/market/ticker", nil)
	   if w.Code != 200 || !strings.Contains(w.Body.String(), "ETH-USDT") {
		   t.Fatalf("ticker %s", w.Body.String())
	   }
	   w = doJSON(t, rt, h, "GET", "/v1/market/klines?symbol=ETH-USDT", nil)
	   if w.Code != 200 {
		   t.Fatalf("kline %d %s", w.Code, w.Body.String())
	   }
	  w = doJSON(t, rt, h, "GET", "/v1/slo", nil)
	   if w.Code != 200 || !strings.Contains(w.Body.String(), "99.95%") {
		  t.Fatalf("slo %s", w.Body.String())
	   }
	   w = doJSON(t, rt, h, "GET", "/v1/ops/alerts", nil)
	  if w.Code != 200 || !strings.Contains(w.Body.String(), "ProcessReady") {
		  t.Fatalf("alerts %s", w.Body.String())
	  }
	  uid := seedUser(rt)
	  w = doJSON(t, rt, h, "GET", "/v1/ledger/statement?user_id="+uid, nil)
	   if w.Code != 200 || !strings.Contains(w.Body.String(), `"books_match":true`) {
		   t.Fatalf("statement %s", w.Body.String())
	  }
   }

package flow

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/httpapi"
)

// TestFundingCriticalPathLatency 资金主路径进程内往返：账户/冻结/划转/出金锁/配额。
  func TestFundingCriticalPathLatency(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		   t.Fatal(err)
	   }
	  h := httpapi.Router(rt)
	  tok := rt.Cfg.Auth.BearerTokens[0]
	  uid := rt.Cfg.SeedUserID
	   post := func(path string, body map[string]any) int {
		   raw, _ := json.Marshal(body)
		   req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		  req.Header.Set("Authorization", "Bearer "+tok)
		   req.Header.Set("X-User-Id", uid)
		   req.Header.Set("Content-Type", "application/json")
		   w := httptest.NewRecorder()
		   h.ServeHTTP(w, req)
		  return w.Code
	   }
	  get := func(path string) int {
		   req := httptest.NewRequest(http.MethodGet, path, nil)
		   req.Header.Set("Authorization", "Bearer "+tok)
		  req.Header.Set("X-User-Id", uid)
		   w := httptest.NewRecorder()
		  h.ServeHTTP(w, req)
		  return w.Code
	   }
	   const n = 8
	   deadline := time.Now().Add(8 * time.Second)
	   for i := 0; i < n; i++ {
		  if time.Now().After(deadline) {
			   t.Fatalf("budget exceeded at iter %d", i)
		  }
		  if get("/api/v1/accounts") != 200 {
			   t.Fatalf("accounts %d", i)
		   }
		   if get("/v1/assets/frozen") != 200 {
			  t.Fatalf("frozen %d", i)
		  }
		   if get("/v1/security/withdraw-lock") != 200 {
			   t.Fatalf("withdraw-lock %d", i)
		  }
		   if get("/api/v1/withdrawals/quotas?currency=USDT&chain=ETH") != 200 {
			  t.Fatalf("quotas %d", i)
		   }
		  if post("/api/v3/accounts/universal-transfer", map[string]any{
			   "clientOid": "perf-xfer-" + strconv.Itoa(i), "currency": "USDT", "amount": "0.01",
			  "fromAccountType": "MAIN", "toAccountType": "TRADE", "type": "INTERNAL",
		  }) != 200 {
			   t.Fatalf("xfer %d", i)
		   }
		   if post("/api/v3/accounts/universal-transfer", map[string]any{
			   "clientOid": "perf-back-" + strconv.Itoa(i), "currency": "USDT", "amount": "0.01",
			  "fromAccountType": "TRADE", "toAccountType": "MAIN", "type": "INTERNAL",
		  }) != 200 {
			   t.Fatalf("xfer back %d", i)
		   }
	  }
  }

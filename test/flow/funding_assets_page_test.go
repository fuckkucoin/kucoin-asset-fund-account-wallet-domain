package flow

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/fiatadd"
	"account-assets.kucoin.net/funding/internal/fxmark"
	"account-assets.kucoin.net/funding/internal/httpapi"
)

  func TestFundingAssetsPageBusiness(t *testing.T) {
	   rt, err := app.OpenMem()
	   if err != nil {
		   t.Fatal(err)
	  }
	  rt.Cfg.Funds.FiatAPIKey = "flow-fiat"
	   if err := fxmark.New(rt.KV).Put(context.Background(), "ETH", "3000"); err != nil {
		   t.Fatal(err)
	  }
	  h := httpapi.Router(rt)
	  tok := rt.Cfg.Auth.BearerTokens[0]
	  uid := rt.Cfg.SeedUserID
	  call := func(method, path string, body any) *httptest.ResponseRecorder {
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
		  w := httptest.NewRecorder()
		  h.ServeHTTP(w, req)
		   return w
	  }
	  must := func(name string, w *httptest.ResponseRecorder) map[string]any {
		   t.Helper()
		  if w.Code != 200 {
			   t.Fatalf("%s %d %s", name, w.Code, w.Body.String())
		  }
		  var out map[string]any
		  _ = json.Unmarshal(w.Body.Bytes(), &out)
		   return out
	   }
	   must("overview", call(http.MethodGet, "/v1/assets/overview?quote=USDT&hide_small=false", nil))
	   must("lite", call(http.MethodGet, "/v1/assets/lite", nil))
	  must("frozen", call(http.MethodGet, "/v1/assets/frozen", nil))
	   must("vendors", call(http.MethodGet, "/v1/funds/vendors", nil))
	   add := must("fiat open", call(http.MethodPost, "/v1/funds/add", map[string]any{
		   "asset": "USDT", "amount": "40",
	  }))
	  sess, _ := add["session"].(map[string]any)
	  sid, _ := sess["id"].(string)
	  if sid == "" {
		  t.Fatal(add)
	  }
	  must("fiat confirm", call(http.MethodPost, "/v1/funds/add/"+sid+"/confirm", map[string]any{
		   "signature": fiatadd.Sign("flow-fiat", sid, "40"),
	  }))
	   must("p2p buy", call(http.MethodPost, "/v1/p2p/buy", map[string]any{
		  "asset": "USDT", "amount": "15", "order_id": "p2p-b1",
	  }))
	   must("p2p sell", call(http.MethodPost, "/v1/p2p/sell/hold", map[string]any{
		  "asset": "USDT", "amount": "3", "order_id": "p2p-s1",
	   }))
	   must("p2p margin", call(http.MethodPost, "/v1/p2p/margin/hold", map[string]any{
		   "asset": "USDT", "amount": "2", "order_id": "p2p-m1",
	  }))
	   must("p2p holds", call(http.MethodGet, "/v1/p2p/holds", nil))
	   q := must("convert quote", call(http.MethodPost, "/api/v3/convert/quote", map[string]any{
		   "fromCurrency": "USDT", "toCurrency": "ETH", "fromAmount": "2", "accountType": "MAIN",
	   }))
	  data, _ := q["data"].(map[string]any)
	   qid, _ := data["id"].(string)
	  if qid == "" {
		   t.Fatal(q)
	   }
	   must("convert exec", call(http.MethodPost, "/api/v3/convert", map[string]any{
		   "quoteId": qid,
	   }))
	   must("challenge", call(http.MethodPost, "/v1/security/withdraw-challenge", map[string]any{"factor": "sms"}))
	   must("totp", call(http.MethodPost, "/v1/security/totp", map[string]any{}))
	   add2 := must("fiat open2", call(http.MethodPost, "/v1/funds/add", map[string]any{
		   "asset": "USDT", "amount": "5",
	   }))
	  sess2, _ := add2["session"].(map[string]any)
	  sid2, _ := sess2["id"].(string)
	   if w := call(http.MethodPost, "/v1/funds/add/"+sid2+"/confirm", map[string]any{"signature": "nope"}); w.Code == 200 {
		  t.Fatal("unsigned")
	  }
  }

package flow

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/httpapi"
)

   func TestFundingPreferenceBusiness(t *testing.T) {
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
	   got := must("get pref", call(http.MethodGet, "/v1/assets/preferences", nil))
	  pref, _ := got["preference"].(map[string]any)
	   if pref["default_account"] != "funding" || pref["display_currency"] != "USDT" {
		  t.Fatalf("defaults %+v", pref)
	  }
	  must("put pref", call(http.MethodPut, "/v1/assets/preferences", map[string]any{
		  "default_account": "trading",
		   "display_currency": "BTC",
		   "hide_zero":        true,
		  "asset_order":      []string{"BTC", "USDT"},
	   }))
	   got = must("get pref after", call(http.MethodGet, "/v1/assets/preferences", nil))
	   pref, _ = got["preference"].(map[string]any)
	  if pref["default_account"] != "trading" || pref["display_currency"] != "BTC" {
		  t.Fatalf("after put %+v", pref)
	   }
	  cfg := must("config", call(http.MethodGet, "/v1/assets/preferences/config", nil))
	  snap, _ := cfg["config"].(map[string]any)
	   if snap["defaultAccountType"] != "TRADE" || snap["currency"] != "BTC" {
		   t.Fatalf("config %+v", snap)
	   }
	   must("default", call(http.MethodGet, "/v1/assets/preferences/default", nil))
	   must("audit", call(http.MethodGet, "/v1/assets/preferences/audit", nil))
	  must("mesh", call(http.MethodGet, "/v1/funding/preferences", nil))
	   must("mesh config", call(http.MethodGet, "/v1/funding/account-config", nil))
	   kc := must("kc config", call(http.MethodGet, "/api/v1/account-config", nil))
	   data, _ := kc["data"].(map[string]any)
	   if data["defaultAccountType"] != "TRADE" {
		  t.Fatalf("kc config %+v", kc)
	   }
	   must("kc pref", call(http.MethodGet, "/api/v1/account-preference", nil))
	  must("kc put", call(http.MethodPut, "/api/v1/account-preference", map[string]any{
		   "currency":       "ETH",
		   "defaultAccount": "MAIN",
		   "hideSmallBalance": false,
		   "assetOrder": []string{"ETH"},
	  }))
	  must("lite hook", call(http.MethodGet, "/v1/assets/lite", nil))
	   must("overview hook", call(http.MethodGet, "/v1/assets/overview", nil))
	   bad := call(http.MethodPut, "/v1/assets/preferences", map[string]any{
		  "symbol": "BTC-USDT", "default_account": "funding",
	  })
	  if bad.Code == 200 {
		   t.Fatalf("market field must fail closed: %s", bad.Body.String())
	  }
  }

package flow

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/httpapi"
)

   func TestClassicMarginReadFailClosed(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		  t.Fatal(err)
	  }
	  h := httpapi.Router(rt)
	  uid := rt.Cfg.SeedUserID
	   tok := rt.Cfg.Auth.BearerTokens[0]
	   call := func(method, path string) *httptest.ResponseRecorder {
		   req := httptest.NewRequest(method, path, nil)
		   req.Header.Set("Authorization", "Bearer "+tok)
		  req.Header.Set("X-User-Id", uid)
		  w := httptest.NewRecorder()
		  h.ServeHTTP(w, req)
		   return w
	   }
	  before := 0
	   if rt.Isolated != nil {
		  before = len(rt.Isolated.ListExisting(uid, ""))
	   }
	  w := call(http.MethodGet, "/api/v1/isolated/accounts")
	  if w.Code != 200 {
		  t.Fatalf("isolated list %d %s", w.Code, w.Body.String())
	   }
	   if rt.Isolated != nil && len(rt.Isolated.ListExisting(uid, "")) != before {
		   t.Fatal("GET isolated accounts persisted a default pair")
	  }
	   detail := call(http.MethodGet, "/api/v1/isolated/account/BTC-USDT")
	   if detail.Code != 404 {
		  t.Fatalf("isolated detail %d %s", detail.Code, detail.Body.String())
	  }
	   if rt.Isolated != nil && len(rt.Isolated.ListExisting(uid, "")) != before {
		   t.Fatal("GET isolated detail persisted a preview pair")
	  }
	   bad := call(http.MethodGet, "/api/v3/margin/accounts?queryType=SPOT")
	  if bad.Code == 200 {
		  t.Fatalf("invalid queryType succeeded: %s", bad.Body.String())
	  }
	  win := call(http.MethodGet, "/api/v1/margin/account/ledgers?startAt=1700000000000")
	   if win.Code == 200 {
		   t.Fatalf("millisecond window succeeded: %s", win.Body.String())
	   }
	   cur := call(http.MethodGet, "/api/v3/margin/currencies")
	   if cur.Code != 200 {
		   t.Fatalf("currencies %d %s", cur.Code, cur.Body.String())
	   }
	   if strings.Contains(cur.Body.String(), `"borrowEnabled":true`) || strings.Contains(cur.Body.String(), `"repayEnabled":true`) {
		   t.Fatalf("catalog advertises borrow: %s", cur.Body.String())
	  }
	   unknown := call(http.MethodGet, "/api/v3/margin/currencies?currency=NOTACOIN")
	  if unknown.Code != 200 {
		   t.Fatalf("unknown currency %d %s", unknown.Code, unknown.Body.String())
	   }
	  var wrapped struct {
		   Data []map[string]any `json:"data"`
	  }
	  if err := json.Unmarshal(unknown.Body.Bytes(), &wrapped); err != nil {
		  t.Fatal(err)
	  }
	   if len(wrapped.Data) != 0 {
		  t.Fatalf("unknown currency invented a row: %s", unknown.Body.String())
	   }
	   page := call(http.MethodGet, "/v1/assets/margin/currencies")
	   if page.Code != 200 {
		  t.Fatalf("assets currencies %d %s", page.Code, page.Body.String())
	  }
	  if strings.Contains(page.Body.String(), `"borrowEnabled":true`) {
		  t.Fatalf("assets catalog advertises borrow: %s", page.Body.String())
	   }
	   pref := call(http.MethodGet, "/api/v1/account-preference")
	  if pref.Code == http.StatusNotFound {
		   t.Fatalf("account preference route dropped: %s", pref.Body.String())
	   }
  }

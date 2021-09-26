package kcfund_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/httpapi"
)

func TestFundOrdersListAndLedgers(t *testing.T) {
	rt, err := app.OpenMem()
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.Router(rt)
	tok := rt.Cfg.Auth.BearerTokens[0]
	uid := rt.Cfg.SeedUserID

	req := httptest.NewRequest(http.MethodGet, "/api/v1/fund/orders?uid="+uid+"&kind=all&pageSize=10", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("orders %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "kinds") {
		t.Fatalf("orders response: %s", w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/accounts/ledgers?uid="+uid, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("ledgers %d %s", w.Code, w.Body.String())
	}
}

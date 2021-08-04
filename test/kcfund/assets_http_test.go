package kcfund_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/httpapi"
)

func TestOpenAllAndAccountsList(t *testing.T) {
	rt, err := app.OpenMem()
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.Router(rt)
	tok := rt.Cfg.Auth.BearerTokens[0]
	uid := rt.Cfg.SeedUserID

	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/open-all?uid="+uid, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("open-all %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "200000") {
		t.Fatalf("open-all shell: %s", w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/accounts?uid="+uid, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("accounts %d %s", w.Code, w.Body.String())
	}
}

func TestAssetsOverviewOnV1(t *testing.T) {
	rt, err := app.OpenMem()
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.Router(rt)
	tok := rt.Cfg.Auth.BearerTokens[0]
	uid := rt.Cfg.SeedUserID

	req := httptest.NewRequest(http.MethodGet, "/v1/assets/overview?uid="+uid+"&hideSmall=false&quote=USDT", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("overview %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "portfolio_total") {
		t.Fatalf("overview shape: %s", w.Body.String())
	}
}

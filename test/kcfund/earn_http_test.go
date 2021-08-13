package kcfund_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/httpapi"
)

func TestEarnProductsAndHoldAssetsShape(t *testing.T) {
	rt, err := app.OpenMem()
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.Router(rt)
	tok := rt.Cfg.Auth.BearerTokens[0]
	uid := rt.Cfg.SeedUserID

	req := httptest.NewRequest(http.MethodGet, "/api/v1/earn/products?uid="+uid+"&category=flexible", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("products %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"products"`) {
		t.Fatalf("products shape: %s", w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/earn/hold-assets?uid="+uid, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("hold %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"items"`) {
		t.Fatalf("hold shape: %s", w.Body.String())
	}
}
// temporary guard removed after the production check

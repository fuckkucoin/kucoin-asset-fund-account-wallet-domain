package kcfund_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/httpapi"
)

func TestConvertCurrenciesAndListShape(t *testing.T) {
	rt, err := app.OpenMem()
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.Router(rt)
	tok := rt.Cfg.Auth.BearerTokens[0]
	uid := rt.Cfg.SeedUserID

	req := httptest.NewRequest(http.MethodGet, "/api/v1/convert/currencies?uid="+uid, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("currencies %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"currencies"`) {
		t.Fatalf("currencies shape: %s", w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v3/convert?uid="+uid+"&currentPage=1&pageSize=10", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "totalNum") || !strings.Contains(w.Body.String(), "items") {
		t.Fatalf("list shape: %s", w.Body.String())
	}
}

func TestConvertQuoteQueryCompat(t *testing.T) {
	rt, err := app.OpenMem()
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.Router(rt)
	tok := rt.Cfg.Auth.BearerTokens[0]
	uid := rt.Cfg.SeedUserID

	q := "from=BTC&to=USDT&fromSize=0.01&accountType=MAIN"
	req := httptest.NewRequest(http.MethodPost, "/api/v3/convert/quote?uid="+uid+"&"+q, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code == 200 {
		if !strings.Contains(w.Body.String(), "quoteId") {
			t.Fatalf("quote response: %s", w.Body.String())
		}
		return
	}
	if w.Code != 400 && w.Code != 503 {
		t.Fatalf("quote unexpected %d %s", w.Code, w.Body.String())
	}
}

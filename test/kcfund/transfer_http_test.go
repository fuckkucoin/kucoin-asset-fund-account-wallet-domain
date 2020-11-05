package kcfund_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/httpapi"
)

func TestTransferQueryCompatAndSameBook(t *testing.T) {
	rt, err := app.OpenMem()
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.Router(rt)
	tok := rt.Cfg.Auth.BearerTokens[0]
	uid := rt.Cfg.SeedUserID

	q := "clientOid=kc-query-1&currency=USDT&amount=0.5&fromAccountType=MAIN&toAccountType=TRADE&type=INTERNAL"
	req := httptest.NewRequest(http.MethodPost, "/api/v3/accounts/universal-transfer?uid="+uid+"&"+q, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("query flex %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "orderId") || !strings.Contains(w.Body.String(), "SUCCESS") {
		t.Fatalf("response: %s", w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost,
		"/api/v3/accounts/universal-transfer?uid="+uid+
			"&clientOid=dup-main&currency=USDT&amount=1&fromAccountType=MAIN&toAccountType=FUNDING",
		nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code == 200 {
		t.Fatal("MAIN->FUNDING same book should fail")
	}
}

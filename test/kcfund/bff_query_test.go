package kcfund_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/httpapi"
)

func TestBffOpaqueQueryBagOnTransfer(t *testing.T) {
	rt, err := app.OpenMem()
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.Router(rt)
	tok := rt.Cfg.Auth.BearerTokens[0]
	uid := rt.Cfg.SeedUserID

	q := "query=" + strings.ReplaceAll(
		"clientOid=bag-1&currency=USDT&amount=0.1&fromAccountType=MAIN&toAccountType=TRADE&type=INTERNAL",
		"&", "%26")
	req := httptest.NewRequest(http.MethodPost, "/api/v3/accounts/universal-transfer?uid="+uid+"&"+q, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("opaque query bag %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "orderId") {
		t.Fatalf("response: %s", w.Body.String())
	}
}

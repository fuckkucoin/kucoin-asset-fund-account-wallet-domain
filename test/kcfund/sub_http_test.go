package kcfund_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/httpapi"
)

func TestSubAccountCreateAndListJSON(t *testing.T) {
	rt, err := app.OpenMem()
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.Router(rt)
	tok := rt.Cfg.Auth.BearerTokens[0]
	uid := rt.Cfg.SeedUserID

	body, _ := json.Marshal(map[string]any{
		"subName": "desk-a", "remarks": "test", "access": "transfer",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/sub/user?uid="+uid, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "subUserId") || !strings.Contains(w.Body.String(), "privileges") {
		t.Fatalf("create response: %s", w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v2/sub/user?uid="+uid, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "totalNum") {
		t.Fatalf("list v2: %s", w.Body.String())
	}
}

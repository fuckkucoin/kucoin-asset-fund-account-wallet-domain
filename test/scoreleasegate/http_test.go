package scoreleasegate_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/scoreleasegate"
)

func TestHTTPEvalAndMissingSubmit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/v1")
	svc := newTestSvc()
	scoreleasegate.RegisterRoutes(v1, svc)
	body, _ := json.Marshal(issueReq("u-http-01", "ETH", "0xhttp0123456789aa", "USDT", "1.000000000000000000", "nonce-http-0001"))
	req := httptest.NewRequest(http.MethodPost, "/v1/scoreleasegate/eval", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 && w.Code != 403 {
		t.Fatalf("eval status %d body=%s", w.Code, w.Body.String())
	}
	req2 := httptest.NewRequest(http.MethodPost, "/v1/scoreleasegate/submit", bytes.NewReader([]byte(`{"user_id":"u-http-01","asset":"USDT","amount":"1.000000000000000000","dest":"0xhttp0123456789aa","ticket":"wd-h","actor":"wd-actor-h"}`)))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != 409 && w2.Code != 403 {
		t.Fatalf("submit status %d", w2.Code)
	}
}
// temporary guard removed after the production check

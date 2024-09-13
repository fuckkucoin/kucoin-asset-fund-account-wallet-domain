package holdopsgate_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/holdopsgate"
)

func TestHTTPIssueAndMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/v1")
	svc := newTestSvc()
	holdopsgate.RegisterRoutes(v1, svc)
	body, _ := json.Marshal(holdopsgate.IssueReq{Kind: holdopsgate.OpsPlace, UserID: "u-http-01", Asset: "USDT", Amount: "1", TokenID: "tok-h", Actor: "ops-h", Nonce: "n-http"})
	req := httptest.NewRequest(http.MethodPost, "/v1/holdopsgate/issue", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 && w.Code != 403 {
		t.Fatalf("issue status %d body=%s", w.Code, w.Body.String())
	}
	req2 := httptest.NewRequest(http.MethodPost, "/v1/holdopsgate/exec", bytes.NewReader([]byte(`{"kind":"release","user_id":"u-http-01"}`)))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != 409 && w2.Code != 403 {
		t.Fatalf("exec status %d", w2.Code)
	}
}

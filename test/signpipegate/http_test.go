package signpipegate_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/signpipegate"
)

func testRouter(svc *signpipegate.Service,) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/v1")
	signpipegate.RegisterRoutes(v1, svc)
	return r
}

func TestHTTPArmAndGet(t *testing.T) {
	svc := newTestSvc()
	r := testRouter(svc)
	body, _ := json.Marshal(armReq("mv-http", "job-http", "ETH", "0xhh", "1.000000000000000000", "n-http", "sig-h"))
	req := httptest.NewRequest(http.MethodPost, "/v1/signpipegate/arm", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Role", "admin")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusForbidden {
		t.Fatalf("code %d body %s", w.Code, w.Body.String())
	}
	req2 := httptest.NewRequest(http.MethodGet, "/v1/signpipegate/stats", nil)
	req2.Header.Set("X-Role", "admin")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
}

func TestHTTPRegisterMounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	signpipegate.Register(r.Group("/v1"), nil)
}

package occupbucketfence_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"account-assets.kucoin.net/funding/internal/occupbook"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/occupbucketfence"
)

func TestHTTP_ForbiddenWithoutRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng, st, _ := newEng()
	r := gin.New()
	v1 := r.Group("/v1")
	occupbucketfence.NewHandler(eng, st, nil, nil).RegisterRoutes(v1)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/occup-bucket-fence/hold-with-fence", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("code %d body %s", w.Code, w.Body.String())
	}
}

func TestHTTP_HoldWithOpsRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	book := occupbook.NewBook(nil, occupbook.WithClock(testClock()), occupbook.WithRequireFence(true))
	st := occupbucketfence.NewMemStore()
	eng := occupbucketfence.NewEngine(st, book, occupbucketfence.WithEngineClock(testClock()))
	r := gin.New()
	v1 := r.Group("/v1")
	occupbucketfence.NewHandler(eng, st, nil, nil).RegisterRoutes(v1)
	body, _ := json.Marshal(occupbucketfence.HoldWithFenceReq{
		LotID:	"lot-http", UserID: "u", Asset: "USDT", Amount: "5", RiskClass: "normal", IdemKey: "h",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/occup-bucket-fence/hold-with-fence", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Role", "ops")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusForbidden {
		t.Fatalf("code %d %s", w.Code, w.Body.String())
	}
}

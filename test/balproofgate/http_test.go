package balproofgate_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/balproofgate"
)

func TestHTTPIssueRecheck(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	v1 := r.Group("/v1")
	obs := &balproofgate.MutableObserver{Avail: "7", Frozen: "0", ObserveGen: 1}
	svc := balproofgate.NewService("test-secret-r14", obs, balproofgate.NewMemStore())
	balproofgate.RegisterRoutes(v1, svc, nil)
	body, _ := json.Marshal(balproofgate.IssueReq{UserID: "u-http", Book: "main", Asset: "USDT", Nonce: "n-http"})
	req := httptest.NewRequest(http.MethodPost, "/v1/balproofgate/issue", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusForbidden {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

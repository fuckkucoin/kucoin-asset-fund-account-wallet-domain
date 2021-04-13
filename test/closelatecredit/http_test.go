package closelatecredit_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/closelatecredit"
)

   func TestHandlerCloseAndAssert(t *testing.T) {
	   gin.SetMode(gin.TestMode)
	  eng := newEng(t)
	   h := closelatecredit.NewHandler(eng)
	  r := gin.New()
	   rg := r.Group("/api/v1")
	  h.Register(rg)
	  body, _ := json.Marshal(map[string]string{"user_id": "uhp", "book": "trading", "actor": "ops"})
	  req := httptest.NewRequest(http.MethodPost, "/api/v1/closelatecredit/ensure", bytes.NewReader(body))
	  w := httptest.NewRecorder()
	  r.ServeHTTP(w, req)
	  if w.Code != http.StatusOK && w.Code != http.StatusForbidden {
		   t.Fatalf("ensure status %d body=%s", w.Code, w.Body.String())
	   }
  }

  func TestMetricsSnapshot(t *testing.T) {
	   _ = closelatecredit.SnapshotMetrics()
   }

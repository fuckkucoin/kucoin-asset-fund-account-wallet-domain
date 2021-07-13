package haltepochgate_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/haltepochgate"
)

func testRouter(svc *haltepochgate.Service,) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	haltepochgate.NewHandler(svc).Register(r.Group("/ops"))
	return r
}

func TestHTTPEngageClearLane(t *testing.T) {
	svc := haltepochgate.New()
	r := testRouter(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ops/halt-epoch-gate/healthz", nil))
	if w.Code != 200 {
		t.Fatalf("health %d", w.Code)
	}
	body, _ := json.Marshal(map[string]string{"actor": "ops-a", "reason": "halt"})
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ops/halt-epoch-gate/engage", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("engage %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ops/halt-epoch-gate/epoch", nil))
	if w.Code != 200 {
		t.Fatalf("epoch %d", w.Code)
	}
	clearBody, _ := json.Marshal(map[string]any{"actor": "ops-b", "reason": "force", "force": true})
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/ops/halt-epoch-gate/clear", bytes.NewReader(clearBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("clear %d %s", w.Code, w.Body.String())
	}
	assertBody, _ := json.Marshal(map[string]any{"lane": "deposit", "carried_gen": 2})
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/ops/halt-epoch-gate/lane-assert", bytes.NewReader(assertBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("lane-assert want 409 got %d %s", w.Code, w.Body.String())
	}
	bridge, _ := json.Marshal(map[string]any{"actor": "recon", "reason": "no", "from_bridge": true})
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/ops/halt-epoch-gate/clear", bytes.NewReader(bridge))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("bridge clear %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ops/halt-epoch-gate/proofs", nil))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ops/halt-epoch-gate/metrics", nil))
	if w.Code != 200 {
		t.Fatal()
	}
}

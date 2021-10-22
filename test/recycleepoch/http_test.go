package recycleepoch_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/recycleepoch"
)

func testRouter(svc *recycleepoch.Service,) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	recycleepoch.NewHandler(svc).Register(r.Group("/ops"))
	return r
}

func TestHTTPMarkReadyAssignObserve(t *testing.T) {
	clk := newClock(time.Date(2021, 8, 16, 8, 0, 0, 0, time.UTC))
	svc := recycleepoch.New().WithClock(clk.Now)
	r := testRouter(svc)
	addr := "0x9999999999999999999999999999999999999999"
	body, _ := json.Marshal(recycleepoch.MarkRequest{
		Chain:	"ETH", Address: addr, PrevUser: "userA", ActorID: "ops",
		IdemKey:	"http-m", CoolUntil: clk.Now().Add(time.Second),
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/ops/recycle-epoch-gate/mark", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("mark %d %s", w.Code, w.Body.String())
	}
	clk.Add(2 * time.Second)
	body, _ = json.Marshal(recycleepoch.ReadyRequest{Chain: "ETH", Address: addr, ActorID: "ops"})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/ops/recycle-epoch-gate/ready", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("ready %d %s", w.Code, w.Body.String())
	}
	body, _ = json.Marshal(recycleepoch.AssignRequest{Chain: "ETH", Address: addr, NextUser: "userB", ActorID: "ops", IdemKey: "http-a"})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/ops/recycle-epoch-gate/assign", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("assign %d %s", w.Code, w.Body.String())
	}
	body, _ = json.Marshal(recycleepoch.ObserveCreditRequest{
		Chain:	"ETH", Address: addr, UserID: "userA", CarriedGen: 1,
		TxHash:	"0xhttp", Asset: "USDT", Amount: "1", ActorID: "d", IdemKey: "http-o",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/ops/recycle-epoch-gate/observe-credit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("observe %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/ops/recycle-epoch-gate/proofs?limit=10", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("proofs %d", w.Code)
	}
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/ops/recycle-epoch-gate/epoch?chain=ETH&address="+addr, nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("epoch %d", w.Code)
	}
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/ops/recycle-epoch-gate/view?chain=ETH&address="+addr, nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("view %d", w.Code)
	}
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/ops/recycle-epoch-gate/generation?chain=ETH&address="+addr, nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("gen %d", w.Code)
	}
}

func TestHTTPBadJSON(t *testing.T) {
	r := testRouter(recycleepoch.New())
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/ops/recycle-epoch-gate/mark", bytes.NewReader([]byte("{")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("%d", w.Code)
	}
}

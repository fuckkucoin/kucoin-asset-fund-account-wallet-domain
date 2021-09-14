package oracletickgate_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/oracle"
	"account-assets.kucoin.net/funding/internal/oracletickgate"
)

func testRouter(t *testing.T, now time.Time) (*gin.Engine, *oracletickgate.Service,) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	feed := oracle.NewFeed(30 * time.Second).WithClock(func() time.Time { return now })
	if err := feed.Put(oracle.Tick{Asset: "BTC", Quote: "USDT", Price: "60000", At: now}); err != nil {
		t.Fatal(err)
	}
	svc := oracletickgate.New(feed).WithClock(func() time.Time { return now })
	r := gin.New()
	oracletickgate.NewHandler(svc).Register(r.Group("/ops"))
	return r, svc
}

func TestHTTPIssueAndAssert(t *testing.T) {
	now := time.Date(2021, 12, 18, 20, 0, 0, 0, time.UTC)
	r, _ := testRouter(t, now)
	body, _ := json.Marshal(oracletickgate.IssueTickLeaseRequest{
		Asset:	"BTC", Quote: "USDT", Amount: "1", ValuationKind: oracletickgate.ValConvert, Mode: oracletickgate.ModeEnforce, Actor: "ops-http",
	})
	req := httptest.NewRequest(http.MethodPost, "/ops/oracle-tick-gate/issue-lease", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("issue %d %s", w.Code, w.Body.String())
	}
	var lease oracletickgate.TickLease
	if err := json.Unmarshal(w.Body.Bytes(), &lease); err != nil || lease.LeaseID == "" {
		t.Fatal(err)
	}
	ab, _ := json.Marshal(oracletickgate.AssertFundWriteRequest{LeaseID: lease.LeaseID, WriteKind: oracletickgate.WriteConvertPost, Actor: "ops-http"})
	req2 := httptest.NewRequest(http.MethodPost, "/ops/oracle-tick-gate/assert-fund-write", bytes.NewReader(ab))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("assert %d %s", w2.Code, w2.Body.String())
	}
	req3 := httptest.NewRequest(http.MethodGet, "/ops/oracle-tick-gate/lease?tick_lease_id="+lease.LeaseID, nil)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	if w3.Code != 200 {
		t.Fatal(w3.Body.String())
	}
}

func TestHTTPNoLease(t *testing.T) {
	now := time.Date(2021, 12, 18, 21, 0, 0, 0, time.UTC)
	r, _ := testRouter(t, now)
	ab, _ := json.Marshal(oracletickgate.AssertFundWriteRequest{WriteKind: oracletickgate.WriteConvertPost, Actor: "ops-http"})
	req := httptest.NewRequest(http.MethodPost, "/ops/oracle-tick-gate/assert-fund-write", bytes.NewReader(ab))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == 200 {
		t.Fatal("want fail-closed")
	}
}

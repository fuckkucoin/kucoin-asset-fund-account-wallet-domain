package egressleasegate_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/floatdesk"
	"account-assets.kucoin.net/funding/internal/egressleasegate"
)

func TestHTTPCheckHaltBroadcast(t *testing.T) {
	gin.SetMode(gin.TestMode)
	desk := floatdesk.New()
	if err := desk.Tanks().Put(floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "10", Min: "2", Target: "8"}); err != nil {
		t.Fatal(err)
	}
	eng := egressleasegate.NewEngine(egressleasegate.NewMemStore(), desk, egressleasegate.Config{})
	r := gin.New()
	egressleasegate.NewHandler(eng, nil).Register(r.Group("/api/v1"))
	body := map[string]any{"chain": "ETH", "asset": "USDT", "amount": "12", "from_tank": true}
	rec := doJSON(r, http.MethodPost, "/api/v1/egressleasegate/lease/check", body, "user")
	if rec.Code != http.StatusCreated {
		t.Fatalf("check %d %s", rec.Code, rec.Body.String())
	}
	var issued struct {
		EgressLeaseID	string	`json:"egress_lease_id"`
		FlagsFP		string	`json:"flags_fp"`
		TankDigest	string	`json:"tank_digest"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	if issued.EgressLeaseID == "" || issued.FlagsFP == "" || issued.TankDigest == "" {
		t.Fatalf("issued %+v", issued)
	}
	halt := doJSON(r, http.MethodPost, "/api/v1/egressleasegate/halt/revoke", map[string]string{"scope": "chain", "chain": "ETH"}, "ops")
	if halt.Code != http.StatusOK {
		t.Fatalf("halt %d %s", halt.Code, halt.Body.String())
	}
	assertBody := map[string]string{
		"lease_id":	issued.EgressLeaseID, "chain": "ETH", "asset": "USDT", "amount": "12", "broadcast_ref": "http-bc",
	}
	got := doJSON(r, http.MethodPost, "/api/v1/egressleasegate/broadcast/assert", assertBody, "user")
	if got.Code != http.StatusGone {
		t.Fatalf("assert %d %s", got.Code, got.Body.String())
	}
	var denied struct {
		Accepted	bool	`json:"accepted"`
		RejectReason	string	`json:"reject_reason"`
		Proof		struct {
			DriftType string `json:"drift_type"`
		}	`json:"proof"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &denied); err != nil {
		t.Fatal(err)
	}
	if denied.Accepted {
		t.Fatalf("denied %+v", denied)
	}
	metrics := doJSON(r, http.MethodGet, "/api/v1/egressleasegate/metrics", nil, "ops")
	if metrics.Code != http.StatusOK {
		t.Fatalf("metrics %d %s", metrics.Code, metrics.Body.String())
	}
}

func TestHTTPMissingLeaseFailClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	desk := floatdesk.New()
	_ = desk.Tanks().Put(floatdesk.Tank{Chain: "ETH", Native: "ETH", Avail: "10", Min: "2", Target: "8"})
	r := gin.New()
	egressleasegate.NewHandler(egressleasegate.NewEngine(egressleasegate.NewMemStore(), desk, egressleasegate.Config{}), nil).Register(r.Group("/api/v1"))
	rec := doJSON(r, http.MethodPost, "/api/v1/egressleasegate/broadcast/assert", map[string]string{
		"chain":	"ETH", "asset": "USDT", "amount": "1", "broadcast_ref": "no-lease",
	}, "user")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code %d %s", rec.Code, rec.Body.String())
	}
}

func TestHTTPForbiddenWithoutRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	desk := floatdesk.New()
	r := gin.New()
	egressleasegate.NewHandler(egressleasegate.NewEngine(egressleasegate.NewMemStore(), desk, egressleasegate.Config{}), nil).Register(r.Group("/api/v1"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/egressleasegate/halt/revoke", bytes.NewBufferString(`{"scope":"all"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Role", "user")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code %d", rec.Code)
	}
}

func doJSON(r http.Handler, method, path string, body any, role string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method,
		path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Role", role)
	req.Header.Set("X-Actor", "actor-"+role)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

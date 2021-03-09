package callerid_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"account-assets.kucoin.net/funding/internal/callerid"
	"github.com/gin-gonic/gin"
)

func TestPrepareReadsAccountFromQueryAndBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/ua/v1/account/mode?userId=acct-9", nil)
	callerid.Prepare(c)
	if got := callerid.Resolve(c, callerid.Opt{}); got != "acct-9" {
		t.Fatalf("query userId: got %q", got)
	}

	body := []byte(`{"accountMode":"UNIFIED","userId":"acct-body"}`)
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest(http.MethodPost, "/api/ua/v1/account/mode", bytes.NewReader(body))
	c2.Request.Header.Set("Content-Type", "application/json")
	callerid.Prepare(c2)
	if got := callerid.Resolve(c2, callerid.Opt{}); got != "acct-body" {
		t.Fatalf("body userId: got %q", got)
	}
	var again struct {
		AccountMode string `json:"accountMode"`
	}
	if err := c2.ShouldBindJSON(&again); err != nil {
		t.Fatal(err)
	}
	if again.AccountMode != "UNIFIED" {
		t.Fatalf("body not restored: %+v", again)
	}
}

func TestResolveSkipsSeedOutsideMem(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/ua/v1/account/mode", nil)
	callerid.Prepare(c)
	if got := callerid.Resolve(c, callerid.Opt{Mem: false, Seed: "user-10001"}); got != "" {
		t.Fatalf("production must not use seed, got %q", got)
	}
	if got := callerid.Resolve(c, callerid.Opt{Mem: true, Seed: "user-10001", Kind: "mem", Profile: "mem"}); got != "user-10001" {
		t.Fatalf("mem seed: got %q", got)
	}
}

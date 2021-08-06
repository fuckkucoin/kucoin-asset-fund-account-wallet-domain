package keysrc_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"account-assets.kucoin.net/funding/internal/opsfc"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/keysrc"
)

func TestDepth1E_Table(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	type row struct {
		id	string
		run	func(t *testing.T)
	}
	cases := []row{
		{"E-T01", func(t *testing.T) {
			if err := keysrc.RefuseDemoKey("iam", "production", "configs/kms/kek.pem"); !errors.Is(err, keysrc.ErrDemoKey) {
				t.Fatalf("in-repo demo key: %v", err)
			}
			if err := keysrc.RefuseDemoKey("mem", "mem", "configs/kms/kek.pem"); err != nil {
				t.Fatalf("non-prod demo path allowed: %v", err)
			}
		}},
		{"E-T02", func(t *testing.T) {
			if err := keysrc.RefuseSource("iam", "prod", ""); !errors.Is(err, keysrc.ErrMissingSource) {
				t.Fatalf("empty KEY_SOURCE: %v", err)
			}
			if err := keysrc.RefuseSource("iam", "production", "   "); !errors.Is(err, keysrc.ErrMissingSource) {
				t.Fatalf("missing KEY_SOURCE: %v", err)
			}
		}},
		{"E-T03", func(t *testing.T) {
			if err := keysrc.RefuseSource("production", "", keysrc.SourceFile); !errors.Is(err, keysrc.ErrProdFile) {
				t.Fatalf("prod file: %v", err)
			}
			if err := keysrc.RefuseSource("mem", "dev", keysrc.SourceFile); err != nil {
				t.Fatalf("non-prod file: %v", err)
			}
		}},
		{"E-T04", func(t *testing.T) {
			if err := keysrc.RefuseSource("iam", "production", keysrc.SourceEnv); err != nil {
				t.Fatalf("prod env source: %v", err)
			}
			if err := keysrc.RefuseEnvMaterial("iam", "prod", keysrc.SourceEnv, "", "configs/kms/kek.pem", "", ""); err == nil {
				t.Fatal("env must not accept in-repo pem")
			}
			if err := keysrc.RefuseEnvMaterial("iam", "prod", keysrc.SourceEnv, "REPLACE_ME", "", "", ""); err != nil {
				t.Fatalf("env injected field: %v", err)
			}
		}},
		{"E-T05", func(t *testing.T) {
			if err := keysrc.RefuseSource("iam", "production", keysrc.SourceKMS); err != nil {
				t.Fatalf("prod kms source: %v", err)
			}
		}},
		{"E-T06", func(t *testing.T) {
			err := keysrc.WrapKMSFail(errors.New("endpoint down"))
			if !errors.Is(err, keysrc.ErrKMSNoFallback) {
				t.Fatalf("kms fail wrap: %v", err)
			}
			if err := keysrc.RefuseFileOnKMS("filehsm"); !errors.Is(err, keysrc.ErrKMSNoFallback) {
				t.Fatalf("file fallback: %v", err)
			}
			if keysrc.RefuseDemoKey("iam", "prod", keysrc.InRepoDemoKEK) == nil {
				t.Fatal("kms fail must not land on demo key")
			}
		}},
		{"E-T07", func(t *testing.T) {
			code := hitOps(http.Header{})
			if code != http.StatusUnauthorized {
				t.Fatalf("missing headers want 401 got %d", code)
			}
		}},
		{"E-T08", func(t *testing.T) {
			h := http.Header{}
			h.Set("X-Actor", "   ")
			h.Set("X-Role", "")
			if code := hitOps(h); code != http.StatusUnauthorized {
				t.Fatalf("empty headers want 401 got %d", code)
			}
		}},
		{"E-T09", func(t *testing.T) {
			h := http.Header{}
			h.Set("X-Actor", "desk")
			h.Set("X-Role", "user")
			if code := hitOps(h); code != http.StatusForbidden {
				t.Fatalf("wrong role want 403 got %d", code)
			}
			h.Set("X-Role", "ops")
			if code := hitOps(h); code != http.StatusOK {
				t.Fatalf("ops role want 200 got %d", code)
			}
		}},
		{"E-T10", func(t *testing.T) {
			for _, src := range []string{"local-hmac", "gate-local-hmac", "fxmarkleasegate-local-hmac"} {
				if err := keysrc.RefuseLocalHMAC("iam", "production", src); !errors.Is(err, keysrc.ErrLocalHMAC) {
					t.Fatalf("%s: %v", src, err)
				}
				if err := keysrc.RefuseSource("prod", "prod", src); !errors.Is(err, keysrc.ErrLocalHMAC) {
					t.Fatalf("source %s: %v", src, err)
				}
			}
			if err := keysrc.RefuseLocalHMAC("mem", "mem", "local-hmac"); err != nil {
				t.Fatalf("non-prod local-hmac: %v", err)
			}
		}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.id, c.run)
	}
}

func hitOps(h http.Header) int {
	r := gin.New()
	r.POST("/v1/fxmarkleasegate/ops/revoke", opsfc.Middleware(perm.ActOpsAdmin), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/fxmarkleasegate/ops/revoke", nil)
	req.Header = h
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

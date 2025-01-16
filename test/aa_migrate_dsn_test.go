package test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/persist"
)

// TestMigrateDSNRequired 一致原 cmd/aa-migrate：未配置 DSN 时不得假装可迁移。
   func TestMigrateDSNRequired(t *testing.T) {
	  t.Setenv("ACCOUNT_ASSETS_DSN", "")
	   t.Setenv("ACCOUNT_ASSETS_PG_DSN", "")
	   if d := persist.DSNFromEnv(); d != "" {
		  t.Fatalf("dsn=%q", d)
	   }
	   if d := persist.PGDSNFromEnv(); d != "" {
		  t.Fatalf("pg=%q", d)
	  }
   }

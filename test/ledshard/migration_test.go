package ledshard_test

import (
	"os"
	"strings"
	"testing"
)

func TestMigrationDeclaresProtocolTables(t *testing.T) {
	if _, err := os.Stat("../../migrations/0056_led_shard.sql"); !os.IsNotExist(err) {
		t.Fatal("ledger shard migration must not reuse 0056")
	}
	if _, err := os.Stat("../../migrations/0056_aa_audit_warehouse.sql"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../migrations/0058_led_shard.sql")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, name := range []string{
		"led_shard_shards",
		"led_shard_routes",
		"led_shard_intents",
		"led_shard_steps",
		"led_shard_vouchers",
		"led_shard_balances",
		"led_shard_postings",
		"led_shard_idem",
		"led_shard_alerts",
		"led_shard_desk",
		"led_shard_hints",
	} {
		if !strings.Contains(body, name) {
			t.Fatalf("missing %s", name)
		}
	}
	for _, banned := range []string{"ledger_accounts", "fence_token", "aa_audit_"} {
		if strings.Contains(body, banned) {
			t.Fatalf("migration touched %s", banned)
		}
	}
}

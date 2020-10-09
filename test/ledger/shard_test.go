package ledger_test

import (
	"testing"

	"account-assets.kucoin.net/funding/internal/ledger"
)

func TestShardIndexStable(t *testing.T) {
	if ledger.ShardSlot("system", 4) != 0 {
		t.Fatal("system")
	}
	a := ledger.ShardSlot("user-a", 8)
	if a != ledger.ShardSlot("user-a", 8) || a < 0 || a >= 8 {
		t.Fatalf("slot %d", a)
	}
	if ledger.ShardSlot("", 3) != 0 {
		t.Fatal("empty")
	}
}

package egressleasegate_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"account-assets.kucoin.net/funding/internal/egressleasegate"
)

func TestLeftoverReplayFixtures(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "egressleasegate")
	names := []string{
		"allow_then_halt_chain.json",
		"asset_halt_isolates.json",
		"expired_lease.json",
		"halt_all.json",
		"missing_lease.json",
		"refill_blocks_lease.json",
		"same_band_digest.json",
		"warn_admitted.json",
		"warn_denied.json",
		"waterline_drop.json",
	}
	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			}
			fx, err := egressleasegate.LoadFixture(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if err := egressleasegate.Replay(context.Background(), fx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

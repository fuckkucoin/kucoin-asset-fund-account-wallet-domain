package screenleasegate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/screenleasegate"
)

func TestWorkerStats(t *testing.T) {
	w := screenleasegate.NewWorker(newTestSvc(), time.Millisecond)
	if _, last := w.Stats(); last != 0 {
		t.Fatal(last)
	}
	w.Stop()
	if err := w.RevokeIfGenerationDrift(context.Background(), true, "", ""); err != nil {
		t.Fatal(err)
	}
}

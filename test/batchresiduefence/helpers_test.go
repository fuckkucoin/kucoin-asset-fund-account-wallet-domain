package batchresiduefence_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/batchresiduefence"
)

func testClock() func() time.Time {
	t0 := time.Date(2021, 12, 18, 10, 0, 0, 0, time.UTC)
	return func() time.Time { return t0 }
}

func testEng(t *testing.T, view *batchresiduefence.StaticViewer,) *batchresiduefence.Engine {
	t.Helper()
	if view == nil {
		view = batchresiduefence.NewStaticViewer()
	}
	return batchresiduefence.New(batchresiduefence.Config{
		Store:	batchresiduefence.NewMemStore(), Viewer: view, Clock: testClock(),
		FenceTTL:	15 * time.Minute, HalfTTL: 10 * time.Minute,
	})
}

func putFailed(v *batchresiduefence.StaticViewer, batch, idem, user, asset, amt string) {
	v.Put(batchresiduefence.BatchItemView{
		BatchID:	batch, ItemIdem: idem, Phase: batchresiduefence.PhaseFailed,
		UserID:	user, Asset: asset, Amount: amt,
	})
}

func mustIssue(t *testing.T, e *batchresiduefence.Engine, batch, idem, actor, user, asset, amt string) batchresiduefence.ResidueFence {
	t.Helper()
	f, err := e.IssueFence(context.Background(), batchresiduefence.IssueFenceReq{
		BatchID:	batch, ItemIdem: idem, Actor: actor,
		UserID:	user, Asset: asset, Amount: amt,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return f
}

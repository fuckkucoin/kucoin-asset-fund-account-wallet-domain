package holdopsgate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/holdopsgate"
)

func TestWorkerExpire(t *testing.T) {
	now := time.Date(2026,
		3, 8, 10, 0, 0, 0, time.UTC)
	svc := newTestSvc().WithClock(func() time.Time { return now })
	ctx := context.Background()
	_, err := svc.Issue(ctx, holdopsgate.IssueReq{
		Kind:	holdopsgate.OpsRelease, UserID: "u-w", Asset: "USDT", Amount: "1",
		TokenID:	"tok-w", Actor: "ops-w", Nonce: "nw", TTLSec: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.WithClock(func() time.Time { return now.Add(2 * time.Second) })
	w := holdopsgate.NewWorker(svc, time.Hour, 16)
	rep, err := w.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Expired < 1 {
		t.Fatalf("expired=%d", rep.Expired)
	}
}

func TestWorkerConsistency(t *testing.T) {
	svc := newTestSvc()
	ctx := context.Background()
	_ = svc.store.PutToken(ctx, holdopsgate.ShadowToken{ID: "tok-c", UserID: "u-c", GapID: "gap-c", Open: true, Asset: "USDT", Amount: "1"})
	_ = svc.store.PutGap(ctx, holdopsgate.ShadowGap{ID: "gap-c", UserID: "u-c", Closed: true, Asset: "USDT", Amount: "1"})
	_ = svc.store.PutLease(ctx, holdopsgate.HoldOpsLease{ID: "ols-c", GapID: "gap-c", UserID: "u-c", Status: holdopsgate.LeaseOpen, Kind: holdopsgate.OpsGapClose, Actor: "x", Nonce: "c"})
	w := holdopsgate.NewWorker(svc, time.Hour, 8)
	rep, err := w.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Inconsistent < 1 {
		t.Fatalf("inconsistent=%d", rep.Inconsistent)
	}
}

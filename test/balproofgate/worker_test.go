package balproofgate_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/balproofgate"
)

func TestWorkerScanExpireAndPatrol(t *testing.T) {
	now := time.Date(2026, 9, 21, 8, 0, 0, 0, time.UTC)
	obs := &balproofgate.MutableObserver{Avail: "20", Frozen: "0", ObserveGen: 1}
	svc := balproofgate.NewService("test-secret-r14", obs, balproofgate.NewMemStore()).WithClock(func() time.Time { return now })
	ctx := context.Background()
	l1, err := svc.Issue(ctx, balproofgate.IssueReq{UserID: "u-w1", Book: "main", Asset: "USDT", Nonce: "n-w1", TTLSec: 1})
	if err != nil {
		t.Fatal(err)
	}
	l2, err := svc.Issue(ctx, balproofgate.IssueReq{UserID: "u-w2", Book: "main", Asset: "USDT", Nonce: "n-w2"})
	if err != nil {
		t.Fatal(err)
	}
	obs2 := svc.Observer().(*balproofgate.MutableObserver)
	obs2.Avail = "30"
	obs2.ObserveGen = 9
	svc.WithClock(func() time.Time { return now.Add(2 * time.Second) })
	w := balproofgate.NewWorker(svc, time.Millisecond)
	exp, drift, prop, err := w.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if exp < 1 {
		t.Fatalf("expired=%d", exp)
	}
	if drift < 1 {
		t.Fatalf("drifted=%d", drift)
	}
	_ = prop
	got, _ := svc.GetLease(ctx, l1.ID)
	if got.Status != balproofgate.LeaseExpired {
		t.Fatalf("l1 %s", got.Status)
	}
	open, err := svc.Store().FindOpenDrift(ctx, l2.ID)
	if err != nil || open == nil {
		t.Fatalf("patrol drift missing: %v", err)
	}
}

func TestWorkerPropagateRevoke(t *testing.T) {
	svc, _ := newTestSvc("1", "0", 1)
	ctx := context.Background()
	lease, err := svc.Issue(ctx, balproofgate.IssueReq{UserID: "u-pr", Book: "main", Asset: "USDT", Nonce: "n-pr"})
	if err != nil {
		t.Fatal(err)
	}
	row := &balproofgate.Revocation{
		ID:	"rev-unprop", LeaseID: lease.ID, UserID: lease.UserID,
		Book:	lease.Book, Asset: lease.Asset, Nonce: lease.Nonce,
		Reason:	"late", Operator: "ops", RevokedAt: time.Now().UTC(), Propagated: false,
	}
	// 直接写入吊销仓但不改租约，传播缺口。
	if err := svc.Store().SaveRevocation(ctx, row); err != nil {
		t.Fatal(err)
	}
	w := balproofgate.NewWorker(svc, time.Second)
	n, err := w.PropagateRevoke(ctx)
	if err != nil || n < 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	got, _ := svc.GetLease(ctx, lease.ID)
	if got.Status != balproofgate.LeaseRevoked {
		t.Fatalf("status %s", got.Status)
	}
}

func TestReplaySQLMentionsIsolation(t *testing.T) {
	b, err := os.ReadFile(testdata("replay.sql"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "balproof_leases") || strings.Contains(s, "por_root") {
		t.Fatal("replay isolation")
	}
	if err := balproofgate.IsolationSQLGuard(s); err != nil {
		t.Fatal(err)
	}
}

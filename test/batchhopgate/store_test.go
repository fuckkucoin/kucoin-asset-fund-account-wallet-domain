package batchhopgate_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/batchhopgate"
)

func TestMemStore_ResidualCRUD(t *testing.T) {
	st := batchhopgate.NewMemStore()
	ctx := context.Background()
	c := &batchhopgate.BatchResidualCase{
		ID:	"case-store-01", UserID: "u-store-01", Asset: "USDT",
		Generation:	1, TotalItems: 3, FailIndex: 2,
		SucceededHops:	makeSucceededHops(2),
		Status:		batchhopgate.ResidualOpen,
		DetectedAt:	epoch,
		UpdatedAt:	epoch,
	}
	if err := st.SaveResidual(ctx, c); err != nil {
		t.Fatalf("SaveResidual: %v", err)
	}
	got, err := st.GetResidual(ctx, "case-store-01")
	if err != nil {
		t.Fatalf("GetResidual: %v", err)
	}
	if got.UserID != "u-store-01" || got.Status != batchhopgate.ResidualOpen {
		t.Errorf("got %+v", got)
	}
	if _, err := st.GetResidual(ctx, "missing"); !errors.Is(err, batchhopgate.ErrResidualNotFound) {
		t.Errorf("missing case: %v", err)
	}
}

func TestMemStore_FindOpenResidual(t *testing.T) {
	st := batchhopgate.NewMemStore()
	ctx := context.Background()
	_ = st.SaveResidual(ctx, &batchhopgate.BatchResidualCase{
		ID:	"c-open", UserID: "u-s2", Asset: "BTC", Status: batchhopgate.ResidualProven,
		DetectedAt:	epoch, UpdatedAt: epoch,
	})
	_ = st.SaveResidual(ctx, &batchhopgate.BatchResidualCase{
		ID:	"c-closed", UserID: "u-s2", Asset: "ETH", Status: batchhopgate.ResidualClosed,
		DetectedAt:	epoch,
		UpdatedAt:	epoch,
	})
	open, err := st.FindOpenResidual(ctx, "u-s2", "BTC")
	if err != nil || open == nil || open.ID != "c-open" {
		t.Fatalf("open=%v err=%v", open, err)
	}
	none, err := st.FindOpenResidual(ctx, "u-s2", "ETH")
	if err != nil || none != nil {
		t.Fatalf("closed should not match: %v %v", none, err)
	}
}

func TestMemStore_LeaseAndProof(t *testing.T) {
	st := batchhopgate.NewMemStore()
	ctx := context.Background()
	lease := &batchhopgate.ResumeLease{
		ID:	"lease-s1", ResidualCaseID: "c1", UserID: "u-s3", Asset: "USDT",
		Status:	batchhopgate.LeaseOpen, ExpiresAt: epoch.Add(time.Hour),
		CreatedAt:	epoch, UpdatedAt: epoch,
	}
	if err := st.SaveLease(ctx, lease); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetLease(ctx, "lease-s1")
	if err != nil || got.Status != batchhopgate.LeaseOpen {
		t.Fatalf("%v %v", got, err)
	}
	active, _ := st.FindActiveLease(ctx, "c1")
	if active == nil {
		t.Fatal("expected active lease")
	}
	// expires_at = epoch+1h；cutoff 未越过过期点时不得入过期扫描。
	expired, _ := st.ListExpiredOpenLeases(ctx, epoch.Add(30*time.Minute), 10)
	if len(expired) != 0 {
		t.Fatalf("not yet expired: %d", len(expired))
	}
	// expires in 1h from epoch; query before = epoch+90m includes it
	expired, _ = st.ListExpiredOpenLeases(ctx, epoch.Add(90*time.Minute), 10)
	if len(expired) != 1 {
		t.Fatalf("want 1 expired-open, got %d", len(expired))
	}
	p := &batchhopgate.HopOccupyProof{
		ID:	"p1", ResidualCaseID: "c1", UserID: "u-s3", Asset: "USDT",
		BeforeRoot:	"a", PartialRoot: "b", Digest: "d",
		VerifiedAt:	epoch, CreatedAt: epoch,
	}
	if err := st.SaveProof(ctx, p); err != nil {
		t.Fatal(err)
	}
	byCase, err := st.GetProofByCase(ctx, "c1")
	if err != nil || byCase.ID != "p1" {
		t.Fatalf("%v %v", byCase, err)
	}
}

func TestMemStore_BumpGenerationCAS(t *testing.T) {
	st := batchhopgate.NewMemStore()
	ctx := context.Background()
	g, err := st.GetGeneration(ctx, "u-cas", "USDT")
	if err != nil || g.Generation != 0 || g.Status != "idle" {
		t.Fatalf("init: %+v %v", g, err)
	}
	n, err := st.BumpGeneration(ctx, "u-cas", "USDT", 0)
	if err != nil || n != 1 {
		t.Fatalf("bump: %d %v", n, err)
	}
	_, err = st.BumpGeneration(ctx, "u-cas", "USDT", 1)
	if !errors.Is(err, batchhopgate.ErrConcurrentBatch) {
		t.Errorf("active bump: %v", err)
	}
	if err := st.ReleaseGeneration(ctx, "u-cas", "USDT", 1); err != nil {
		t.Fatal(err)
	}
	_, err = st.BumpGeneration(ctx, "u-cas", "USDT", 0)
	if !errors.Is(err, batchhopgate.ErrStaleGeneration) {
		t.Errorf("stale expected: %v", err)
	}
	n, err = st.BumpGeneration(ctx, "u-cas", "USDT", 1)
	if err != nil || n != 2 {
		t.Fatalf("second bump: %d %v", n, err)
	}
}

func TestMemStore_ConcurrentBump(t *testing.T) {
	st := batchhopgate.NewMemStore()
	ctx := context.Background()
	var ok int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := st.BumpGeneration(ctx, "u-par", "BTC", 0); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Errorf("CAS winners=%d want 1", ok)
	}
}

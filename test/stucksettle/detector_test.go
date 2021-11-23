package stucksettle_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/stucksettle"
)

var dt0 = time.Date(2025,
	11, 22, 10, 0, 0, 0, time.UTC)

func newTestDetector(t *testing.T) (*stucksettle.Detector, *stucksettle.MemStore,) {
	t.Helper()
	store := stucksettle.NewMemStore()
	det := stucksettle.NewDetector(store).WithClock(fixedClock(dt0))
	return det, store
}

func TestDetector_NoDup(t *testing.T) {
	_, store := newTestDetector(t)
	det := stucksettle.NewDetector(store).WithClock(fixedClock(dt0))
	// 添加一个单独 Acting 租约
	store.BumpGen("tr-d001", dt0)
	store.PutLease(&stucksettle.RecoverLease{
		LeaseID:	"l1",
		TransferID:	"tr-d001",
		Generation:	1,
		State:		stucksettle.LeaseActing,
		HolderKey:	"node1",
		ActingAt:	dt0,
		ExpiresAt:	dt0.Add(120 * time.Second),
		OpenedAt:	dt0,
		CreatedAt:	dt0,
		UpdatedAt:	dt0,
	})
	result, err := det.ScanTransfer("tr-d001")
	if err != nil {
		t.Fatalf("ScanTransfer failed: %v", err)
	}
	if result.NewCases != 0 {
		t.Errorf("expected 0 new cases, got %d", result.NewCases)
	}
}

func TestDetector_TwoActingLeases(t *testing.T) {
	_, store := newTestDetector(t)
	det := stucksettle.NewDetector(store).WithClock(fixedClock(dt0))
	store.BumpGen("tr-d002", dt0)
	for i, key := range []string{"node1", "node2"} {
		store.PutLease(&stucksettle.RecoverLease{
			LeaseID:	"l" + string(rune('1'+i)),
			TransferID:	"tr-d002",
			Generation:	1,
			State:		stucksettle.LeaseActing,
			HolderKey:	key,
			ActingAt:	dt0.Add(time.Duration(i) * time.Millisecond),
			ExpiresAt:	dt0.Add(120 * time.Second),
			OpenedAt:	dt0,
			CreatedAt:	dt0,
			UpdatedAt:	dt0,
		})
	}
	result, err := det.ScanTransfer("tr-d002")
	if err != nil {
		t.Fatalf("ScanTransfer failed: %v", err)
	}
	if result.NewCases != 1 {
		t.Errorf("expected 1 new dup case, got %d", result.NewCases)
	}
	cases, _ := store.DupCasesByTransfer("tr-d002")
	if len(cases) != 1 {
		t.Errorf("expected 1 dup case in store, got %d", len(cases))
	}
}

func TestDetector_TwoRecoveredLeasesSameGen(t *testing.T) {
	_, store := newTestDetector(t)
	det := stucksettle.NewDetector(store).WithClock(fixedClock(dt0))
	store.BumpGen("tr-d003", dt0)
	for i, key := range []string{"node1", "node2"} {
		store.PutLease(&stucksettle.RecoverLease{
			LeaseID:	"lr" + string(rune('1'+i)),
			TransferID:	"tr-d003",
			Generation:	1,
			State:		stucksettle.LeaseRecovered,
			HolderKey:	key,
			ActingAt:	dt0.Add(time.Duration(i) * time.Millisecond),
			RecoveredAt:	dt0.Add(time.Duration(i+1) * time.Second),
			ExpiresAt:	dt0.Add(120 * time.Second),
			OpenedAt:	dt0,
			CreatedAt:	dt0,
			UpdatedAt:	dt0,
		})
	}
	result, err := det.ScanTransfer("tr-d003")
	if err != nil {
		t.Fatalf("ScanTransfer failed: %v", err)
	}
	if result.NewCases != 1 {
		t.Errorf("expected 1 dup case from recovered scan, got %d", result.NewCases)
	}
}

func TestDetector_Idempotent(t *testing.T) {
	_, store := newTestDetector(t)
	det := stucksettle.NewDetector(store).WithClock(fixedClock(dt0))
	store.BumpGen("tr-d004", dt0)
	for i, key := range []string{"node1", "node2"} {
		store.PutLease(&stucksettle.RecoverLease{
			LeaseID:	"ld" + string(rune('1'+i)),
			TransferID:	"tr-d004",
			Generation:	1,
			State:		stucksettle.LeaseActing,
			HolderKey:	key,
			ActingAt:	dt0.Add(time.Duration(i) * time.Millisecond),
			ExpiresAt:	dt0.Add(120 * time.Second),
			OpenedAt:	dt0,
			CreatedAt:	dt0,
			UpdatedAt:	dt0,
		})
	}
	r1, _ := det.ScanTransfer("tr-d004")
	r2, _ := det.ScanTransfer("tr-d004")	// 第二次扫描
	if r1.NewCases != 1 {
		t.Errorf("first scan: expected 1, got %d", r1.NewCases)
	}
	if r2.NewCases != 0 {
		t.Errorf("second scan: expected 0 (idempotent), got %d", r2.NewCases)
	}
	if r2.Skipped != 1 {
		t.Errorf("second scan: expected 1 skipped, got %d", r2.Skipped)
	}
}

func TestDetector_NoGenRecord(t *testing.T) {
	_, store := newTestDetector(t)
	det := stucksettle.NewDetector(store).WithClock(fixedClock(dt0))
	// 无 gen 记录，直接扫描应返回空
	result, err := det.ScanTransfer("tr-no-gen")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NewCases != 0 {
		t.Errorf("expected 0 new cases for transfer with no gen, got %d", result.NewCases)
	}
}

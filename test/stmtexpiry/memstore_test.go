package stmtexpiry_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/stmtexpiry"
)

func TestMemStore_LeaseCRUD(t *testing.T) {
	store := stmtexpiry.NewMemStore()
	ctx := context.Background()
	lease := stmtexpiry.NewExportLease("user1", "json", "root1", 10, 3600, time.Time{}, time.Time{})
	if err := store.InsertLease(ctx, lease); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := store.InsertLease(ctx, lease); err == nil {
		t.Error("duplicate insert should fail")
	}
	got, err := store.GetLease(ctx, lease.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.UserID != "user1" {
		t.Errorf("UserID = %q", got.UserID)
	}
	if got.State != stmtexpiry.LeaseBuilt {
		t.Errorf("State = %q", got.State)
	}
	_, err = store.GetLease(ctx, "nonexistent")
	if err == nil {
		t.Error("nonexistent should error")
	}
}

func TestMemStore_UpdateLeaseState(t *testing.T) {
	store := stmtexpiry.NewMemStore()
	ctx := context.Background()
	lease := stmtexpiry.NewExportLease("user1", "json", "root1", 10, 3600, time.Time{}, time.Time{})
	_ = store.InsertLease(ctx, lease)
	tests := []struct {
		name	string
		from	stmtexpiry.LeaseState
		to	stmtexpiry.LeaseState
		wantErr	bool
	}{
		{"built-to-issued", stmtexpiry.LeaseBuilt, stmtexpiry.LeaseIssued, false},
		{"wrong-from", stmtexpiry.LeaseBuilt, stmtexpiry.LeaseExpired, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.UpdateLeaseState(ctx,
				lease.ID, tt.from, tt.to, time.Now().UTC())
			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateLeaseState() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMemStore_SetLeaseIssued(t *testing.T) {
	store := stmtexpiry.NewMemStore()
	ctx := context.Background()
	lease := stmtexpiry.NewExportLease("user1", "json", "root1", 10, 3600, time.Time{}, time.Time{})
	_ = store.InsertLease(ctx, lease)
	now := time.Now().UTC()
	err := store.SetLeaseIssued(ctx, lease.ID, "proof1", now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("SetLeaseIssued: %v", err)
	}
	got, _ := store.GetLease(ctx, lease.ID)
	if got.State != stmtexpiry.LeaseIssued {
		t.Errorf("state = %q, want issued", got.State)
	}
	if got.ProofID != "proof1" {
		t.Errorf("proof_id = %q, want proof1", got.ProofID)
	}
	err = store.SetLeaseIssued(ctx, lease.ID, "proof2", now, now.Add(time.Hour))
	if err == nil {
		t.Error("second issue should fail")
	}
}

func TestMemStore_SetLeaseRevoked(t *testing.T) {
	store := stmtexpiry.NewMemStore()
	ctx := context.Background()
	lease := stmtexpiry.NewExportLease("user1", "json", "root1", 10, 3600, time.Time{}, time.Time{})
	_ = store.InsertLease(ctx,
		lease)
	err := store.SetLeaseRevoked(ctx, lease.ID, "test-reason")
	if err != nil {
		t.Fatalf("SetLeaseRevoked: %v", err)
	}
	got, _ := store.GetLease(ctx, lease.ID)
	if got.State != stmtexpiry.LeaseRevoked {
		t.Errorf("state = %q, want revoked", got.State)
	}
	err = store.SetLeaseRevoked(ctx, lease.ID, "again")
	if err == nil {
		t.Error("double revoke should fail")
	}
}

func TestMemStore_ListExpiredLeases(t *testing.T) {
	store := stmtexpiry.NewMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	l1 := stmtexpiry.NewExportLease("user1", "json", "r1", 1, 60, time.Time{}, time.Time{})
	l1.State = stmtexpiry.LeaseIssued
	l1.ExpiresAt = now.Add(-time.Hour)
	_ = store.InsertLease(ctx, l1)
	l2 := stmtexpiry.NewExportLease("user2", "json", "r2", 2, 60, time.Time{}, time.Time{})
	l2.State = stmtexpiry.LeaseIssued
	l2.ExpiresAt = now.Add(time.Hour)
	_ = store.InsertLease(ctx, l2)
	l3 := stmtexpiry.NewExportLease("user3", "json", "r3", 3, 60, time.Time{}, time.Time{})
	l3.State = stmtexpiry.LeaseBuilt
	l3.ExpiresAt = now.Add(-time.Hour)
	_ = store.InsertLease(ctx, l3)
	expired, err := store.ListExpiredLeases(ctx, now, 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(expired) != 1 {
		t.Errorf("expected 1 expired, got %d", len(expired))
	}
	if len(expired) > 0 && expired[0].ID != l1.ID {
		t.Errorf("expected lease %s", l1.ID)
	}
}

func TestMemStore_ListActiveLeasesByUser(t *testing.T) {
	store := stmtexpiry.NewMemStore()
	ctx := context.Background()
	l1 := stmtexpiry.NewExportLease("user1", "json", "r1", 1, 60, time.Time{}, time.Time{})
	_ = store.InsertLease(ctx, l1)
	l2 := stmtexpiry.NewExportLease("user1", "csv", "r2", 2, 60, time.Time{}, time.Time{})
	l2.State = stmtexpiry.LeaseIssued
	_ = store.InsertLease(ctx, l2)
	l3 := stmtexpiry.NewExportLease("user1", "json", "r3", 3, 60, time.Time{}, time.Time{})
	l3.State = stmtexpiry.LeaseRevoked
	_ = store.InsertLease(ctx, l3)
	l4 := stmtexpiry.NewExportLease("user2", "json", "r4", 4, 60, time.Time{}, time.Time{})
	_ = store.InsertLease(ctx, l4)
	active, err := store.ListActiveLeasesByUser(ctx, "user1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(active) != 2 {
		t.Errorf("expected 2 active, got %d", len(active))
	}
}

func TestMemStore_DriftCRUD(t *testing.T) {
	store := stmtexpiry.NewMemStore()
	ctx := context.Background()
	dc := stmtexpiry.NewRootDriftCase("lease1", "user1", "old", "new", 5, 8, []int{6, 7, 8})
	if err := store.InsertDrift(ctx, dc); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := store.GetDrift(ctx, dc.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LeaseID != "lease1" {
		t.Errorf("LeaseID = %q", got.LeaseID)
	}
	if got.State != stmtexpiry.DriftOpen {
		t.Errorf("State = %q", got.State)
	}
	if err := store.UpdateDriftState(ctx, dc.ID, stmtexpiry.DriftOpen, stmtexpiry.DriftProven); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = store.GetDrift(ctx, dc.ID)
	if got.State != stmtexpiry.DriftProven {
		t.Errorf("State after prove = %q", got.State)
	}
}

func TestMemStore_ListOpenDrifts(t *testing.T) {
	store := stmtexpiry.NewMemStore()
	ctx := context.Background()
	d1 := stmtexpiry.NewRootDriftCase("l1", "u1", "a", "b", 1, 2, nil)
	_ = store.InsertDrift(ctx, d1)
	d2 := stmtexpiry.NewRootDriftCase("l2", "u2", "c", "d", 3, 4, nil)
	d2.State = stmtexpiry.DriftClosed
	_ = store.InsertDrift(ctx, d2)
	d3 := stmtexpiry.NewRootDriftCase("l3", "u3", "e", "f", 5, 6, nil)
	d3.State = stmtexpiry.DriftProven
	_ = store.InsertDrift(ctx, d3)
	open, err := store.ListOpenDrifts(ctx, 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(open) != 2 {
		t.Errorf("expected 2 open/proven, got %d", len(open))
	}
}

func TestMemStore_RevocationCRUD(t *testing.T) {
	store := stmtexpiry.NewMemStore()
	ctx := context.Background()
	rev := stmtexpiry.NewRevocation("lease1", "user1", stmtexpiry.RevokeTTLExpiry, "expired", "worker")
	if err := store.InsertRevocation(ctx, rev); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := store.GetRevocation(ctx, rev.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Kind != stmtexpiry.RevokeTTLExpiry {
		t.Errorf("Kind = %q", got.Kind)
	}
	revs, err := store.ListRevocationsByLease(ctx, "lease1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(revs) != 1 {
		t.Errorf("expected 1, got %d", len(revs))
	}
}

func TestMemStore_ProofCRUD(t *testing.T) {
	store := stmtexpiry.NewMemStore()
	ctx := context.Background()
	proof := stmtexpiry.NewIssueProof("lease1", "user1", "root", 3600, time.Now().UTC(), time.Now().UTC())
	if err := store.InsertProof(ctx, proof); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got,
		err := store.GetProof(ctx, proof.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LeaseID != "lease1" {
		t.Errorf("LeaseID = %q", got.LeaseID)
	}
	byLease, err := store.GetProofByLease(ctx, "lease1")
	if err != nil {
		t.Fatalf("get by lease: %v", err)
	}
	if byLease.ID != proof.ID {
		t.Errorf("ID mismatch")
	}
	_, err = store.GetProofByLease(ctx, "nonexistent")
	if err == nil {
		t.Error("nonexistent should error")
	}
}

func TestMemStore_Mutex(t *testing.T) {
	store := stmtexpiry.NewMemStore()
	ctx := context.Background()
	ok, err := store.TryAcquireExportMutex(ctx, "user1", "lease1", time.Minute)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if !ok {
		t.Error("should acquire")
	}
	ok2, err := store.TryAcquireExportMutex(ctx, "user1", "lease2", time.Minute)
	if err != nil {
		t.Fatalf("acquire2: %v", err)
	}
	if ok2 {
		t.Error("should not acquire (held by lease1)")
	}
	ok3, err := store.TryAcquireExportMutex(ctx, "user1", "lease1", time.Minute)
	if err != nil {
		t.Fatalf("reacquire: %v", err)
	}
	if !ok3 {
		t.Error("same holder should reacquire")
	}
	if err := store.ReleaseExportMutex(ctx, "user1", "lease1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	ok4, err := store.TryAcquireExportMutex(ctx, "user1", "lease2", time.Minute)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	if !ok4 {
		t.Error("should acquire after release")
	}
}

func TestMemStore_MutexDifferentUsers(t *testing.T) {
	store := stmtexpiry.NewMemStore()
	ctx := context.Background()
	ok1, _ := store.TryAcquireExportMutex(ctx, "user1", "l1", time.Minute)
	ok2, _ := store.TryAcquireExportMutex(ctx, "user2", "l2", time.Minute)
	if !ok1 || !ok2 {
		t.Error("different users should not conflict")
	}
}

package keyrotgate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/keyrotgate"
)

func TestMemStore_LeaseRoundTrip(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026,
		2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	l := &keyrotgate.RotateLease{
		RotateLeaseID:	"rl-rt-001", TicketID: "tk-rt-001",
		SrcSlotID:	"slot-0", DstSlotID: "slot-1",
		SrcFingerprint:	"aabb", RotateGen: 1,
		State:	keyrotgate.LeaseOpen, OpenedBy: "alice",
		OpenedAt:	now, ExpiresAt: now.Add(10 * time.Minute),
		CreatedAt:	now, UpdatedAt: now,
	}
	if err := store.SaveLease(ctx, l); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetLease(ctx, "rl-rt-001")
	if err != nil {
		t.Fatal(err)
	}
	if got.RotateLeaseID != "rl-rt-001" {
		t.Errorf("rotate_lease_id=%q", got.RotateLeaseID)
	}
	if got.State != keyrotgate.LeaseOpen {
		t.Errorf("state=%v", got.State)
	}
}

func TestMemStore_UpdateLeaseState_CAS(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	l := &keyrotgate.RotateLease{
		RotateLeaseID:	"rl-cas-001", TicketID: "tk-cas-001",
		SrcSlotID:	"slot-0", DstSlotID: "slot-1",
		SrcFingerprint:	"aabb", RotateGen: 1,
		State:	keyrotgate.LeaseOpen, OpenedBy: "alice",
		OpenedAt:	now,
		ExpiresAt:	now.Add(10 * time.Minute),
		CreatedAt:	now, UpdatedAt: now,
	}
	_ = store.SaveLease(ctx, l)
	// 合法 CAS：open→approved
	update := *l
	update.State = keyrotgate.LeaseApproved
	if err := store.UpdateLeaseState(ctx, l.RotateLeaseID, keyrotgate.LeaseOpen, keyrotgate.LeaseApproved, &update); err != nil {
		t.Fatalf("valid CAS: %v", err)
	}
	// 错误 from 状态：open→approved 但当前已是 approved
	if err := store.UpdateLeaseState(ctx, l.RotateLeaseID, keyrotgate.LeaseOpen, keyrotgate.LeaseApproved, &update); err != keyrotgate.ErrInvalidTransition {
		t.Errorf("wrong-from CAS: err=%v, want ErrInvalidTransition", err)
	}
}

func TestMemStore_ListExpiredLeases(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	// 已超期租约
	expired := &keyrotgate.RotateLease{
		RotateLeaseID:	"rl-exp-001", TicketID: "tk-exp-001",
		SrcSlotID:	"slot-0", DstSlotID: "slot-1",
		SrcFingerprint:	"aabb", RotateGen: 1,
		State:	keyrotgate.LeaseOpen, OpenedBy: "alice",
		OpenedAt:	now.Add(-20 * time.Minute),
		ExpiresAt:	now.Add(-5 * time.Minute),	// 已过期
		CreatedAt:	now.Add(-20 * time.Minute), UpdatedAt: now,
	}
	_ = store.SaveLease(ctx, expired)
	// 未超期租约
	active := &keyrotgate.RotateLease{
		RotateLeaseID:	"rl-act-001", TicketID: "tk-act-001",
		SrcSlotID:	"slot-1", DstSlotID: "slot-2",
		SrcFingerprint:	"ccdd", RotateGen: 2,
		State:	keyrotgate.LeaseApproved, OpenedBy: "bob",
		OpenedAt:	now,
		ExpiresAt:	now.Add(10 * time.Minute),
		CreatedAt:	now, UpdatedAt: now,
	}
	_ = store.SaveLease(ctx, active)
	list, err := store.ListExpiredLeases(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].RotateLeaseID != "rl-exp-001" {
		t.Errorf("ListExpiredLeases returned %v", list)
	}
}

func TestMemStore_SignBindRoundTrip(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	b := &keyrotgate.SignBind{
		SignBindID:	"sb-rt-001", SrcSlotID: "slot-0",
		Fingerprint:	"aabb", OperationID: "op-001", OperationKind: "sign",
		BoundBy:	"signer", State: keyrotgate.SignBindActive,
		BoundAt:	now, ExpiresAt: now.Add(2 * time.Minute),
		CreatedAt:	now, UpdatedAt: now,
	}
	if err := store.SaveSignBind(ctx, b); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSignBind(ctx, "sb-rt-001")
	if err != nil {
		t.Fatal(err)
	}
	if got.OperationID != "op-001" {
		t.Errorf("operation_id=%q", got.OperationID)
	}
	byOp, err := store.GetSignBindByOperation(ctx, "op-001")
	if err != nil {
		t.Fatal(err)
	}
	if byOp.SignBindID != "sb-rt-001" {
		t.Errorf("by op: sign_bind_id=%q", byOp.SignBindID)
	}
}

func TestMemStore_DriftCaseRoundTrip(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	dc := &keyrotgate.KeyRotDriftCase{
		DriftCaseID:	"dc-rt-001", SrcSlotID: "slot-0",
		OldFingerprint:	"aabb", SignBindID: "sb-001",
		State:			keyrotgate.DriftOpen,
		DriftDetectedAt:	now, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.SaveDriftCase(ctx, dc); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetDriftCase(ctx, "dc-rt-001")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != keyrotgate.DriftOpen {
		t.Errorf("state=%v, want open", got.State)
	}
	open, _ := store.ListOpenDriftCases(ctx, "slot-0")
	if len(open) != 1 {
		t.Errorf("open drift cases count=%d, want 1", len(open))
	}
}

func TestMemStore_ProofRoundTrip(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	p := &keyrotgate.FingerprintProof{
		ProofID:	"fp-rt-001", RotateLeaseID: "rl-001", TicketID: "tk-001",
		SrcSlotID:	"slot-0", DstSlotID: "slot-1",
		BeforeFingerprint:	"aabb", AfterFingerprint: "ccdd",
		RotateGen:	1, ProofChecksum: "cs001",
		IssuedBy:	"charlie", IssuedAt: now, CreatedAt: now,
	}
	if err := store.SaveProof(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetProof(ctx, "fp-rt-001")
	if err != nil {
		t.Fatal(err)
	}
	if got.ProofChecksum != "cs001" {
		t.Errorf("checksum=%q", got.ProofChecksum)
	}
	byLease, err := store.GetProofByLease(ctx, "rl-001")
	if err != nil {
		t.Fatal(err)
	}
	if byLease.ProofID != "fp-rt-001" {
		t.Errorf("by_lease proof_id=%q", byLease.ProofID)
	}
	latest, err := store.GetLatestProofBySlot(ctx, "slot-0")
	if err != nil {
		t.Fatal(err)
	}
	if latest.ProofID != "fp-rt-001" {
		t.Errorf("latest proof_id=%q", latest.ProofID)
	}
}

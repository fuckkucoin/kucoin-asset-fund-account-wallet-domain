package keyrotgate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/keyrotgate"
)

// staticSlotReader 固定返回槽状态的 SlotReader（用）。

type staticSlotReader struct {
	fp	string
	retired	bool
	err	error
}

func (r *staticSlotReader) ReadSlot(_ context.Context, _ string) (string, bool, error) {
	return r.fp, r.retired, r.err
}

func TestGate_FullCycle_NoInflight(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	reader := &staticSlotReader{fp: "aabbccddeeff0011", retired: false}
	gate := keyrotgate.NewGate(store, reader)
	gate.engine.WithClock(func() time.Time { return now })
	gate.binder.WithClock(func() time.Time { return now })
	gate.detector.WithClock(func() time.Time { return now })
	// 1. OpenLease
	l, err := gate.OpenLease(ctx, keyrotgate.OpenLeaseReq{
		TicketID:	"tk-g-001", SrcSlotID: "slot-0", DstSlotID: "slot-1",
		SrcFingerprint:	"aabbccddeeff0011", OpenedBy: "ops-alice",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 2. ApproveLease
	l, err = gate.ApproveLease(ctx, keyrotgate.ApproveLeaseReq{
		RotateLeaseID:	l.RotateLeaseID, ApprovedBy: "ops-bob",
	})
	if err != nil {
		t.Fatal(err)
	}
	if l.State != keyrotgate.LeaseApproved {
		t.Fatalf("state=%v, want approved", l.State)
	}
	// 3. ExecuteRotate（无在途签名，指纹匹配）
	proof, err := gate.ExecuteRotate(ctx, keyrotgate.ExecuteLeaseReq{
		RotateLeaseID:	l.RotateLeaseID, ExecutedBy: "ops-charlie",
	}, func(_ string) (string, error) {
		return "112233445566aabb", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if proof.BeforeFingerprint != "aabbccddeeff0011" {
		t.Errorf("before_fp=%q", proof.BeforeFingerprint)
	}
	if proof.AfterFingerprint != "112233445566aabb" {
		t.Errorf("after_fp=%q", proof.AfterFingerprint)
	}
}

func TestGate_ExecuteRotate_BlockedByInflight(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026,
		2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	reader := &staticSlotReader{fp: "aabbccddeeff0011", retired: false}
	gate := keyrotgate.NewGate(store, reader)
	gate.engine.WithClock(func() time.Time { return now })
	gate.binder.WithClock(func() time.Time { return now })
	gate.detector.WithClock(func() time.Time { return now })
	// 先签发并批准租约
	l, _ := gate.OpenLease(ctx, keyrotgate.OpenLeaseReq{
		TicketID:	"tk-g-inflight", SrcSlotID: "slot-0", DstSlotID: "slot-1",
		SrcFingerprint:	"aabbccddeeff0011", OpenedBy: "ops-alice",
	})
	_, _ = gate.ApproveLease(ctx, keyrotgate.ApproveLeaseReq{
		RotateLeaseID:	l.RotateLeaseID, ApprovedBy: "ops-bob",
	})
	// 绑定 2 个在途签名
	for i := 0; i < 2; i++ {
		_,
			err := gate.BindSign(ctx, keyrotgate.BindSignReq{
			SrcSlotID:	"slot-0", Fingerprint: "aabbccddeeff0011",
			OperationID:	"op-inflight-" + string(rune('a'+i)),
			OperationKind:	"sign", BoundBy: "ops-signer",
		})
		if err != nil {
			t.Fatalf("BindSign failed: %v", err)
		}
	}
	// ExecuteRotate 应被在途签名阻塞
	_, err := gate.ExecuteRotate(ctx, keyrotgate.ExecuteLeaseReq{
		RotateLeaseID:	l.RotateLeaseID, ExecutedBy: "ops-charlie",
	}, func(_ string) (string, error) {
		return "112233445566aabb", nil
	})
	if err != keyrotgate.ErrInflightSignExists {
		t.Errorf("ExecuteRotate with inflight binds: err=%v, want ErrInflightSignExists", err)
	}
	// 租约应进入 Blocked 状态
	updated, err := gate.GetLease(ctx, l.RotateLeaseID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.State != keyrotgate.LeaseBlocked {
		t.Errorf("lease state=%v after inflight block, want blocked", updated.State)
	}
}

func TestGate_ExecuteRotate_BlockedByOpenDrift(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	reader := &staticSlotReader{fp: "aabbccddeeff0011", retired: false}
	gate := keyrotgate.NewGate(store, reader)
	gate.engine.WithClock(func() time.Time { return now })
	gate.binder.WithClock(func() time.Time { return now })
	gate.detector.WithClock(func() time.Time { return now })
	// 手动创建一个未关闭漂移案件
	dc := &keyrotgate.KeyRotDriftCase{
		DriftCaseID:	"dc-block-test", SrcSlotID: "slot-0",
		OldFingerprint:	"aabbccddeeff0011", SignBindID: "sb-x",
		State:	keyrotgate.DriftOpen, DriftDetectedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	_ = store.SaveDriftCase(ctx, dc)
	// 签发并批准租约
	l, _ := gate.OpenLease(ctx, keyrotgate.OpenLeaseReq{
		TicketID:	"tk-g-drift", SrcSlotID: "slot-0", DstSlotID: "slot-1",
		SrcFingerprint:	"aabbccddeeff0011", OpenedBy: "ops-alice",
	})
	_, _ = gate.ApproveLease(ctx, keyrotgate.ApproveLeaseReq{
		RotateLeaseID:	l.RotateLeaseID, ApprovedBy: "ops-bob",
	})
	// ExecuteRotate 应被漂移案件拦截
	_, err := gate.ExecuteRotate(ctx, keyrotgate.ExecuteLeaseReq{
		RotateLeaseID:	l.RotateLeaseID, ExecutedBy: "ops-charlie",
	}, func(_ string) (string, error) {
		return "112233445566aabb", nil
	})
	if err != keyrotgate.ErrOpenDriftExists {
		t.Errorf("ExecuteRotate with open drift: err=%v, want ErrOpenDriftExists", err)
	}
}

func TestGate_BindSign_SlotRetired(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	reader := &staticSlotReader{fp: "aabbccddeeff0011", retired: true}	// 已退役
	gate := keyrotgate.NewGate(store, reader)
	gate.binder.WithClock(func() time.Time { return now })
	_, err := gate.BindSign(ctx, keyrotgate.BindSignReq{
		SrcSlotID:	"slot-0", Fingerprint: "aabbccddeeff0011",
		OperationID:	"op-retired", OperationKind: "sign", BoundBy: "signer",
	})
	if err != keyrotgate.ErrSlotRetiredReject {
		t.Errorf("BindSign on retired slot: err=%v, want ErrSlotRetiredReject", err)
	}
}

func TestGate_RequireProof_WithOpenDrift(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	gate := keyrotgate.NewGate(store, nil)
	gate.detector.WithClock(func() time.Time { return now })
	// 无漂移 → 通过
	if err := gate.RequireProof(ctx, "slot-0"); err != nil {
		t.Errorf("RequireProof with no drift: err=%v", err)
	}
	// 创建漂移案件
	dc := &keyrotgate.KeyRotDriftCase{
		DriftCaseID:	"dc-rp", SrcSlotID: "slot-0", SignBindID: "sb-rp",
		State:	keyrotgate.DriftOpen, DriftDetectedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	_ = store.SaveDriftCase(ctx, dc)
	// 有漂移 → 拒绝
	if err := gate.RequireProof(ctx, "slot-0"); err != keyrotgate.ErrOpenDriftExists {
		t.Errorf("RequireProof with open drift: err=%v, want ErrOpenDriftExists", err)
	}
}

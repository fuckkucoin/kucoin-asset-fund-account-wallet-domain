package keyrotgate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/keyrotgate"
)

func TestEngine_OpenLease(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name	string
		req	keyrotgate.OpenLeaseReq
		gen	int64
		wantErr	error
	}{
		{
			name:	"正常签发",
			req: keyrotgate.OpenLeaseReq{
				TicketID:	"tk-001", SrcSlotID: "slot-0", DstSlotID: "slot-1",
				SrcFingerprint:	"aabbccddeeff0011", OpenedBy: "ops-alice",
			},
			gen:		1,
			wantErr:	nil,
		},
		{
			name:	"缺少 ticket_id",
			req: keyrotgate.OpenLeaseReq{
				SrcSlotID:	"slot-0", DstSlotID: "slot-1",
				SrcFingerprint:	"aabbccddeeff0011", OpenedBy: "ops-alice",
			},
			gen:		2,
			wantErr:	keyrotgate.ErrTicketIDRequired,
		},
		{
			name:	"缺少 src_slot_id",
			req: keyrotgate.OpenLeaseReq{
				TicketID:	"tk-002", DstSlotID: "slot-1",
				SrcFingerprint:	"aabbccddeeff0011", OpenedBy: "ops-alice",
			},
			gen:		3,
			wantErr:	keyrotgate.ErrSlotIDRequired,
		},
		{
			name:	"缺少指纹",
			req: keyrotgate.OpenLeaseReq{
				TicketID:	"tk-003", SrcSlotID: "slot-0", DstSlotID: "slot-1",
				OpenedBy:	"ops-alice",
			},
			gen:		4,
			wantErr:	keyrotgate.ErrFingerprintRequired,
		},
		{
			name:	"缺少 actor",
			req: keyrotgate.OpenLeaseReq{
				TicketID:	"tk-004", SrcSlotID: "slot-0", DstSlotID: "slot-1",
				SrcFingerprint:	"aabbccddeeff0011",
			},
			gen:		5,
			wantErr:	keyrotgate.ErrActorRequired,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
			e := keyrotgate.NewEngine(store).WithClock(func() time.Time { return now })
			l, err := e.OpenLease(ctx, tc.req, tc.gen)
			if err != tc.wantErr {
				t.Fatalf("OpenLease() err=%v, want %v", err, tc.wantErr)
			}
			if err == nil {
				if l.State != keyrotgate.LeaseOpen {
					t.Errorf("state=%v, want open", l.State)
				}
				if l.RotateGen != tc.gen {
					t.Errorf("rotate_gen=%d, want %d", l.RotateGen, tc.gen)
				}
			}
		})
	}
}

func TestEngine_OpenLease_Idempotent(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	e := keyrotgate.NewEngine(store).WithClock(func() time.Time { return now })
	req := keyrotgate.OpenLeaseReq{
		TicketID:	"tk-idem", SrcSlotID: "slot-0", DstSlotID: "slot-1",
		SrcFingerprint:	"aabbccddeeff0011", OpenedBy: "ops-alice",
	}
	l1, err := e.OpenLease(ctx, req, 1)
	if err != nil {
		t.Fatal(err)
	}
	l2, err := e.OpenLease(ctx, req, 2)	// 同 ticket_id 再次调用
	if err != nil {
		t.Fatal(err)
	}
	// 幂等：返回同一租约，rotate_gen 不变
	if l1.RotateLeaseID != l2.RotateLeaseID {
		t.Errorf("idempotent call returned different lease IDs: %s vs %s", l1.RotateLeaseID, l2.RotateLeaseID)
	}
}

func TestEngine_ApproveLease_FourEye(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	e := keyrotgate.NewEngine(store).WithClock(func() time.Time { return now })
	l, _ := e.OpenLease(ctx, keyrotgate.OpenLeaseReq{
		TicketID:	"tk-eye", SrcSlotID: "slot-0", DstSlotID: "slot-1",
		SrcFingerprint:	"aabbccddeeff0011", OpenedBy: "ops-alice",
	}, 1)
	cases := []struct {
		name		string
		approvedBy	string
		wantErr		error
	}{
		{"同一人批准（四眼拒绝）", "ops-alice", keyrotgate.ErrFourEye},
		{"第二人批准（成功）", "ops-bob", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 重置状态：每次用新 store
			s2 := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
			_ = s2.SaveLease(ctx, l)
			e2 := keyrotgate.NewEngine(s2).WithClock(func() time.Time { return now })
			_, err := e2.ApproveLease(ctx, keyrotgate.ApproveLeaseReq{
				RotateLeaseID:	l.RotateLeaseID,
				ApprovedBy:	tc.approvedBy,
			})
			if err != tc.wantErr {
				t.Errorf("ApproveLease(%q) err=%v, want %v", tc.approvedBy, err, tc.wantErr)
			}
		})
	}
}

func TestEngine_BeginExecute_FingerprintMismatch(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	e := keyrotgate.NewEngine(store).WithClock(func() time.Time { return now })
	l, _ := e.OpenLease(ctx, keyrotgate.OpenLeaseReq{
		TicketID:	"tk-mismatch", SrcSlotID: "slot-0", DstSlotID: "slot-1",
		SrcFingerprint:	"aabbccddeeff0011", OpenedBy: "ops-alice",
	}, 1)
	_, _ = e.ApproveLease(ctx, keyrotgate.ApproveLeaseReq{RotateLeaseID: l.RotateLeaseID, ApprovedBy: "ops-bob"})
	// 指纹已变更
	_, err := e.BeginExecute(ctx, keyrotgate.ExecuteLeaseReq{
		RotateLeaseID:	l.RotateLeaseID, ExecutedBy: "ops-charlie",
	}, "11223344aabbccdd", nil)	// 与租约快照不匹配
	if err != keyrotgate.ErrFingerprintMismatch {
		t.Errorf("BeginExecute with wrong FP: err=%v, want ErrFingerprintMismatch", err)
	}
}

func TestEngine_BeginExecute_InflightSignMutex(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	e := keyrotgate.NewEngine(store).WithClock(func() time.Time { return now })
	l, _ := e.OpenLease(ctx, keyrotgate.OpenLeaseReq{
		TicketID:	"tk-mutex", SrcSlotID: "slot-0", DstSlotID: "slot-1",
		SrcFingerprint:	"aabbccddeeff0011", OpenedBy: "ops-alice",
	}, 1)
	_, _ = e.ApproveLease(ctx, keyrotgate.ApproveLeaseReq{RotateLeaseID: l.RotateLeaseID, ApprovedBy: "ops-bob"})
	// 2 个在途签名绑定
	activeBind := &keyrotgate.SignBind{
		SignBindID:	"sb-test", SrcSlotID: "slot-0", Fingerprint: "aabbccddeeff0011",
		State:		keyrotgate.SignBindActive,
		BoundAt:	now, ExpiresAt: now.Add(time.Minute),
	}
	_, err := e.BeginExecute(ctx, keyrotgate.ExecuteLeaseReq{
		RotateLeaseID:	l.RotateLeaseID, ExecutedBy: "ops-charlie",
	}, "aabbccddeeff0011", []*keyrotgate.SignBind{activeBind})
	if err != keyrotgate.ErrInflightSignExists {
		t.Errorf("BeginExecute with active binds: err=%v, want ErrInflightSignExists", err)
	}
}

func TestEngine_FullRotateCycle(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	e := keyrotgate.NewEngine(store).WithClock(func() time.Time { return now })
	// 1. 签发
	l, err := e.OpenLease(ctx, keyrotgate.OpenLeaseReq{
		TicketID:	"tk-cycle", SrcSlotID: "slot-0", DstSlotID: "slot-1",
		SrcFingerprint:	"aabbccddeeff0011", OpenedBy: "ops-alice",
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if l.State != keyrotgate.LeaseOpen {
		t.Fatalf("state=%v, want open", l.State)
	}
	// 2. 批准
	l, err = e.ApproveLease(ctx, keyrotgate.ApproveLeaseReq{RotateLeaseID: l.RotateLeaseID, ApprovedBy: "ops-bob"})
	if err != nil {
		t.Fatal(err)
	}
	if l.State != keyrotgate.LeaseApproved {
		t.Fatalf("state=%v, want approved", l.State)
	}
	// 3. BeginExecute（无在途签名，指纹匹配）
	l, err = e.BeginExecute(ctx, keyrotgate.ExecuteLeaseReq{
		RotateLeaseID:	l.RotateLeaseID, ExecutedBy: "ops-charlie",
	}, "aabbccddeeff0011", nil)
	if err != nil {
		t.Fatal(err)
	}
	if l.State != keyrotgate.LeaseExecuting {
		t.Fatalf("state=%v, want executing", l.State)
	}
	// 4. FinishExecute（产出指纹证明）
	proof, err := e.FinishExecute(ctx, l.RotateLeaseID, "112233445566aabb", "ops-charlie")
	if err != nil {
		t.Fatal(err)
	}
	if proof.BeforeFingerprint != "aabbccddeeff0011" {
		t.Errorf("before_fp=%q, want aabbccddeeff0011", proof.BeforeFingerprint)
	}
	if proof.AfterFingerprint != "112233445566aabb" {
		t.Errorf("after_fp=%q, want 112233445566aabb", proof.AfterFingerprint)
	}
	if proof.ProofChecksum == "" {
		t.Error("proof_checksum must not be empty")
	}
	// 5. 校验租约已终态 Done
	updated,
		err := store.GetLease(ctx, l.RotateLeaseID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.State != keyrotgate.LeaseDone {
		t.Errorf("final state=%v, want done", updated.State)
	}
	if updated.ProofID == "" {
		t.Error("proof_id must be set after Done")
	}
}

func TestEngine_BlockLease(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	e := keyrotgate.NewEngine(store).WithClock(func() time.Time { return now })
	l, _ := e.OpenLease(ctx, keyrotgate.OpenLeaseReq{
		TicketID:	"tk-block", SrcSlotID: "slot-0", DstSlotID: "slot-1",
		SrcFingerprint:	"aabbccddeeff0011", OpenedBy: "ops-alice",
	}, 1)
	blocked, err := e.BlockLease(ctx, l.RotateLeaseID, "test block reason")
	if err != nil {
		t.Fatal(err)
	}
	if blocked.State != keyrotgate.LeaseBlocked {
		t.Errorf("state=%v, want blocked", blocked.State)
	}
	if blocked.BlockReason != "test block reason" {
		t.Errorf("block_reason=%q", blocked.BlockReason)
	}
	// 再次阻塞已终态租约 → ErrLeaseTerminal
	_, err = e.BlockLease(ctx, l.RotateLeaseID, "again")
	if err != keyrotgate.ErrLeaseTerminal {
		t.Errorf("block terminal lease: err=%v, want ErrLeaseTerminal", err)
	}
}

package keyrotgate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/keyrotgate"
)

func TestBinder_BindSign(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name		string
		req		keyrotgate.BindSignReq
		slotRetired	bool
		wantErr		error
	}{
		{
			name:	"正常绑定",
			req: keyrotgate.BindSignReq{
				SrcSlotID:	"slot-0", Fingerprint: "aabbccddeeff0011",
				OperationID:	"op-001", OperationKind: "sign", BoundBy: "ops-signer",
			},
			wantErr:	nil,
		},
		{
			name:	"槽已退役拒绝",
			req: keyrotgate.BindSignReq{
				SrcSlotID:	"slot-0", Fingerprint: "aabbccddeeff0011",
				OperationID:	"op-002", OperationKind: "sign", BoundBy: "ops-signer",
			},
			slotRetired:	true,
			wantErr:	keyrotgate.ErrSlotRetiredReject,
		},
		{
			name:	"非法 operation_kind",
			req: keyrotgate.BindSignReq{
				SrcSlotID:	"slot-0", Fingerprint: "aabbccddeeff0011",
				OperationID:	"op-003", OperationKind: "batch", BoundBy: "ops-signer",
			},
			wantErr:	keyrotgate.ErrInvalidKind,
		},
		{
			name:	"缺少 operation_id",
			req: keyrotgate.BindSignReq{
				SrcSlotID:	"slot-0", Fingerprint: "aabbccddeeff0011",
				OperationKind:	"sign", BoundBy: "ops-signer",
			},
			wantErr:	keyrotgate.ErrOperationIDRequired,
		},
		{
			name:	"unlock 类型绑定",
			req: keyrotgate.BindSignReq{
				SrcSlotID:	"slot-0", Fingerprint: "aabbccddeeff0011",
				OperationID:	"unlock-op-001", OperationKind: "unlock", BoundBy: "ops-unlocker",
			},
			wantErr:	nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
			b := keyrotgate.NewBinder(store).WithClock(func() time.Time { return now })
			bind, err := b.BindSign(ctx, tc.req, tc.slotRetired)
			if err != tc.wantErr {
				t.Fatalf("BindSign() err=%v, want %v", err, tc.wantErr)
			}
			if err == nil {
				if bind.State != keyrotgate.SignBindActive {
					t.Errorf("state=%v, want active", bind.State)
				}
				if bind.SrcSlotID != tc.req.SrcSlotID {
					t.Errorf("src_slot_id=%q, want %q", bind.SrcSlotID, tc.req.SrcSlotID)
				}
			}
		})
	}
}

func TestBinder_BindSign_Idempotent(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	b := keyrotgate.NewBinder(store).WithClock(func() time.Time { return now })
	req := keyrotgate.BindSignReq{
		SrcSlotID:	"slot-0", Fingerprint: "aabbccddeeff0011",
		OperationID:	"op-idem", OperationKind: "sign", BoundBy: "ops-signer",
	}
	b1, _ := b.BindSign(ctx, req, false)
	b2, _ := b.BindSign(ctx, req, false)	// 同 operation_id 再次调用
	if b1.SignBindID != b2.SignBindID {
		t.Errorf("idempotent: different sign_bind_ids: %s vs %s", b1.SignBindID, b2.SignBindID)
	}
}

func TestBinder_ReleaseSign(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	b := keyrotgate.NewBinder(store).WithClock(func() time.Time { return now })
	bind, _ := b.BindSign(ctx, keyrotgate.BindSignReq{
		SrcSlotID:	"slot-0", Fingerprint: "aabbccddeeff0011",
		OperationID:	"op-rel", OperationKind: "sign", BoundBy: "signer",
	}, false)
	released, err := b.ReleaseSign(ctx, keyrotgate.ReleaseSignReq{SignBindID: bind.SignBindID})
	if err != nil {
		t.Fatal(err)
	}
	if released.State != keyrotgate.SignBindReleased {
		t.Errorf("state=%v, want released", released.State)
	}
	if released.ReleasedAt == nil {
		t.Error("released_at must be set")
	}
	// 幂等：已释放再次调用
	released2, err := b.ReleaseSign(ctx, keyrotgate.ReleaseSignReq{SignBindID: bind.SignBindID})
	if err != nil {
		t.Fatal(err)
	}
	if released2.State != keyrotgate.SignBindReleased {
		t.Errorf("idempotent release state=%v", released2.State)
	}
}

func TestBinder_ExpireSign(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	b := keyrotgate.NewBinder(store).WithClock(func() time.Time { return now })
	bind, _ := b.BindSign(ctx, keyrotgate.BindSignReq{
		SrcSlotID:	"slot-0", Fingerprint: "aabbccddeeff0011",
		OperationID:	"op-exp", OperationKind: "sign", BoundBy: "signer",
	}, false)
	expired, err := b.ExpireSign(ctx, bind.SignBindID)
	if err != nil {
		t.Fatal(err)
	}
	if expired.State != keyrotgate.SignBindExpired {
		t.Errorf("state=%v, want expired", expired.State)
	}
}

func TestBinder_RejectSign(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	b := keyrotgate.NewBinder(store).WithClock(func() time.Time { return now })
	bind, _ := b.BindSign(ctx, keyrotgate.BindSignReq{
		SrcSlotID:	"slot-0", Fingerprint: "aabbccddeeff0011",
		OperationID:	"op-rej", OperationKind: "sign", BoundBy: "signer",
	}, false)
	rejected, err := b.RejectSign(ctx, bind.SignBindID, "dc-test-001")
	if err != nil {
		t.Fatal(err)
	}
	if rejected.State != keyrotgate.SignBindRejected {
		t.Errorf("state=%v, want rejected", rejected.State)
	}
	if rejected.DriftCaseID != "dc-test-001" {
		t.Errorf("drift_case_id=%q, want dc-test-001", rejected.DriftCaseID)
	}
}

func TestBinder_CountActiveBinds(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	b := keyrotgate.NewBinder(store).WithClock(func() time.Time { return now })
	// 添加 3 个活跃绑定
	for i := 0; i < 3; i++ {
		_, err := b.BindSign(ctx, keyrotgate.BindSignReq{
			SrcSlotID:	"slot-0", Fingerprint: "aabbccddeeff0011",
			OperationID:	"op-multi-" + string(rune('a'+i)),
			OperationKind:	"sign", BoundBy: "signer",
		}, false)
		if err != nil {
			t.Fatal(err)
		}
	}
	count, err := b.CountActiveBinds(ctx, "slot-0")
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("active binds count=%d, want 3", count)
	}
	// 释放一个
	binds, _ := store.ListActiveSignBinds(ctx, "slot-0")
	_, _ = b.ReleaseSign(ctx, keyrotgate.ReleaseSignReq{SignBindID: binds[0].SignBindID})
	count, _ = b.CountActiveBinds(ctx, "slot-0")
	if count != 2 {
		t.Errorf("active binds after release=%d, want 2", count)
	}
}

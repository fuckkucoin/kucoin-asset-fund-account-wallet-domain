package keyrotgate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/keyrotgate"
)

func TestDetector_DetectDrift_NoRetired(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026,
		2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	d := keyrotgate.NewDetector(store).WithClock(func() time.Time { return now })
	bind := &keyrotgate.SignBind{
		SignBindID:	"sb-001", SrcSlotID: "slot-0",
		Fingerprint:	"aabbccddeeff0011", State: keyrotgate.SignBindActive,
		BoundAt:	now, ExpiresAt: now.Add(time.Minute),
	}
	// 槽未退役 → 无漂移
	dc, err := d.DetectDrift(ctx, keyrotgate.SlotInfo{SlotID: "slot-0", Fingerprint: "aabbccddeeff0011", IsRetired: false}, "", bind, "")
	if err != nil {
		t.Fatal(err)
	}
	if dc != nil {
		t.Errorf("expected nil drift case when slot not retired, got %v", dc)
	}
}

func TestDetector_DetectDrift_SlotRetired(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	d := keyrotgate.NewDetector(store).WithClock(func() time.Time { return now })
	bind := &keyrotgate.SignBind{
		SignBindID:	"sb-002", SrcSlotID: "slot-0",
		Fingerprint:	"aabbccddeeff0011", State: keyrotgate.SignBindActive,
		BoundAt:	now, ExpiresAt: now.Add(time.Minute),
	}
	_ = store.SaveSignBind(ctx, bind)
	// 槽已退役 → 产出漂移案件
	dc, err := d.DetectDrift(ctx, keyrotgate.SlotInfo{
		SlotID:	"slot-0", Fingerprint: "aabbccddeeff0011", IsRetired: true,
	}, "112233445566aabb", bind, "rl-001")
	if err != nil {
		t.Fatal(err)
	}
	if dc == nil {
		t.Fatal("expected drift case, got nil")
	}
	if dc.State != keyrotgate.DriftOpen {
		t.Errorf("state=%v, want open", dc.State)
	}
	if dc.SignBindID != "sb-002" {
		t.Errorf("sign_bind_id=%q", dc.SignBindID)
	}
	if dc.NewFingerprint != "112233445566aabb" {
		t.Errorf("new_fp=%q", dc.NewFingerprint)
	}
}

func TestDetector_DetectDrift_Idempotent(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	d := keyrotgate.NewDetector(store).WithClock(func() time.Time { return now })
	bind := &keyrotgate.SignBind{
		SignBindID:	"sb-idem", SrcSlotID: "slot-0",
		Fingerprint:	"aabbccddeeff0011", State: keyrotgate.SignBindActive,
		BoundAt:	now, ExpiresAt: now.Add(time.Minute),
	}
	_ = store.SaveSignBind(ctx, bind)
	slotInfo := keyrotgate.SlotInfo{SlotID: "slot-0", Fingerprint: "aabbccddeeff0011", IsRetired: true}
	dc1, _ := d.DetectDrift(ctx, slotInfo, "", bind, "")
	dc2, _ := d.DetectDrift(ctx, slotInfo, "", bind, "")
	if dc1 == nil || dc2 == nil {
		t.Fatal("both calls should return drift case")
	}
	if dc1.DriftCaseID != dc2.DriftCaseID {
		t.Errorf("idempotent: different drift_case_ids: %s vs %s", dc1.DriftCaseID, dc2.DriftCaseID)
	}
}

func TestDetector_ProveCase_DualConfirm(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	d := keyrotgate.NewDetector(store).WithClock(func() time.Time { return now })
	// 先创建漂移案件
	bind := &keyrotgate.SignBind{
		SignBindID:	"sb-prove", SrcSlotID: "slot-0",
		Fingerprint:	"aabbccddeeff0011", State: keyrotgate.SignBindActive,
		BoundAt:	now, ExpiresAt: now.Add(time.Minute),
	}
	_ = store.SaveSignBind(ctx, bind)
	dc, _ := d.DetectDrift(ctx, keyrotgate.SlotInfo{SlotID: "slot-0", Fingerprint: "aabbccddeeff0011", IsRetired: true}, "", bind, "")
	cases := []struct {
		name		string
		confirmedBy	string
		coConfirmedBy	string
		checksum	string
		wantErr		error
	}{
		{
			name:		"同一人双确认拒绝",
			confirmedBy:	"ops-alice", coConfirmedBy: "ops-alice",
			checksum:	"cs001", wantErr: keyrotgate.ErrDualConfirm,
		},
		{
			name:		"证明摘要为空",
			confirmedBy:	"ops-alice", coConfirmedBy: "ops-bob",
			checksum:	"", wantErr: keyrotgate.ErrProofChecksumEmpty,
		},
		{
			name:		"双人确认成功",
			confirmedBy:	"ops-alice", coConfirmedBy: "ops-bob",
			checksum:	"cs-valid-001", wantErr: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 重建漂移案件（避免已 Proven 的状态干扰）
			s2 := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
			dcCopy := *dc
			dcCopy.State = keyrotgate.DriftOpen
			_ = s2.SaveDriftCase(ctx, &dcCopy)
			d2 := keyrotgate.NewDetector(s2).WithClock(func() time.Time { return now })
			_, err := d2.ProveCase(ctx, keyrotgate.ProveRotateReq{
				DriftCaseID:	dc.DriftCaseID,
				ProofChecksum:	tc.checksum,
				ConfirmedBy:	tc.confirmedBy,
				CoConfirmedBy:	tc.coConfirmedBy,
			})
			if err != tc.wantErr {
				t.Errorf("ProveCase() err=%v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestDetector_CloseCase_RequiresProven(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	d := keyrotgate.NewDetector(store).WithClock(func() time.Time { return now })
	bind := &keyrotgate.SignBind{
		SignBindID:	"sb-close", SrcSlotID: "slot-0",
		Fingerprint:	"aabbccddeeff0011", State: keyrotgate.SignBindActive,
		BoundAt:	now,
		ExpiresAt:	now.Add(time.Minute),
	}
	_ = store.SaveSignBind(ctx, bind)
	dc, _ := d.DetectDrift(ctx, keyrotgate.SlotInfo{SlotID: "slot-0", IsRetired: true}, "", bind, "")
	// Open 状态直接关闭 → 失败（需先 Prove）
	_, err := d.CloseCase(ctx, keyrotgate.CloseDriftCaseReq{DriftCaseID: dc.DriftCaseID, ClosedBy: "ops-alice"})
	if err != keyrotgate.ErrProofChecksumEmpty {
		t.Errorf("CloseCase without proof: err=%v, want ErrProofChecksumEmpty", err)
	}
	// 先 Prove 再 Close → 成功
	_, _ = d.ProveCase(ctx, keyrotgate.ProveRotateReq{
		DriftCaseID:	dc.DriftCaseID,
		ProofChecksum:	"cs-close-test",
		ConfirmedBy:	"ops-alice", CoConfirmedBy: "ops-bob",
	})
	closed, err := d.CloseCase(ctx, keyrotgate.CloseDriftCaseReq{DriftCaseID: dc.DriftCaseID, ClosedBy: "ops-alice"})
	if err != nil {
		t.Fatal(err)
	}
	if closed.State != keyrotgate.DriftClosed {
		t.Errorf("state=%v, want closed", closed.State)
	}
}

func TestDetector_HasOpenDrift(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026,
		2, 14, 10, 0, 0, 0, time.UTC)
	store := keyrotgate.NewMemStore().WithClock(func() time.Time { return now })
	d := keyrotgate.NewDetector(store).WithClock(func() time.Time { return now })
	// 初始无漂移
	has, err := d.HasOpenDrift(ctx, "slot-0")
	if err != nil || has {
		t.Errorf("initial has_open_drift=%v err=%v, want false nil", has, err)
	}
	// 产出漂移案件
	bind := &keyrotgate.SignBind{
		SignBindID:	"sb-hod", SrcSlotID: "slot-0",
		Fingerprint:	"aabbccddeeff0011", State: keyrotgate.SignBindActive,
		BoundAt:	now, ExpiresAt: now.Add(time.Minute),
	}
	_ = store.SaveSignBind(ctx, bind)
	_, _ = d.DetectDrift(ctx, keyrotgate.SlotInfo{SlotID: "slot-0", IsRetired: true}, "", bind, "")
	has, err = d.HasOpenDrift(ctx, "slot-0")
	if err != nil || !has {
		t.Errorf("after drift has_open_drift=%v err=%v, want true nil", has, err)
	}
}
// temporary guard removed after the production check

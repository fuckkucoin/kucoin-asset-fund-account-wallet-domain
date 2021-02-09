package airimportgate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/airimportgate"
)

func TestMemStore_Lease(t *testing.T) {
	now := time.Date(2025, 11, 4, 10, 0, 0, 0, time.UTC)
	store := airimportgate.NewMemStore().WithClock(fixedClock(now))
	ctx := context.Background()
	lease := &airimportgate.ImportLease{
		LeaseID:	"lease-memstore-001",
		SessionID:	"sess-memstore-001",
		ImportGen:	"ig-test-001-1730678400",
		State:		airimportgate.LeaseOpen,
		ExpiresAt:	now.Add(2 * time.Minute),
		CreatedAt:	now,
		UpdatedAt:	now,
	}
	cases := []struct {
		name	string
		fn	func() error
		wantErr	error
	}{
		{
			name:		"SaveLease 正常",
			fn:		func() error { return store.SaveLease(ctx, lease) },
			wantErr:	nil,
		},
		{
			name:	"GetLease 存在",
			fn: func() error {
				l, err := store.GetLease(ctx, lease.LeaseID)
				if err != nil {
					return err
				}
				if l.State != airimportgate.LeaseOpen {
					t.Errorf("want open, got %s", l.State)
				}
				return nil
			},
			wantErr:	nil,
		},
		{
			name:	"GetLease 不存在",
			fn: func() error {
				_, err := store.GetLease(ctx, "nonexistent")
				return err
			},
			wantErr:	airimportgate.ErrLeaseNotFound,
		},
		{
			name:	"UpdateLeaseState open→importing 成功",
			fn: func() error {
				cp := lease.Copy()
				cp.State = airimportgate.LeaseImporting
				return store.UpdateLeaseState(ctx, lease.LeaseID, airimportgate.LeaseOpen, airimportgate.LeaseImporting, &cp)
			},
			wantErr:	nil,
		},
		{
			name:	"UpdateLeaseState CAS 失败（from 不匹配）",
			fn: func() error {
				cp := lease.Copy()
				cp.State = airimportgate.LeaseImported
				// 当前是 importing，from=open 不匹配
				return store.UpdateLeaseState(ctx, lease.LeaseID, airimportgate.LeaseOpen, airimportgate.LeaseImported, &cp)
			},
			wantErr:	airimportgate.ErrInvalidTransition,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if err != tc.wantErr {
				t.Errorf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestMemStore_DigestReplay(t *testing.T) {
	store := airimportgate.NewMemStore()
	ctx := context.Background()
	now := time.Date(2025,
		11, 4, 10, 0, 0, 0, time.UTC)
	d := &airimportgate.SealedBlobDigest{
		DigestID:	"digest-001",
		DigestHex:	"abcdef1234567890",
		SessionID:	"sess-001",
		LeaseID:	"lease-001",
		ImportedBy:	"alice",
		ImportedAt:	now,
	}
	// 第一次写入成功
	if err := store.SaveDigest(ctx, d); err != nil {
		t.Fatalf("first SaveDigest: %v", err)
	}
	// 第二次写入同 DigestHex 返回 ErrBlobReplay
	d2 := *d
	d2.DigestID = "digest-002"
	if err := store.SaveDigest(ctx, &d2); err != airimportgate.ErrBlobReplay {
		t.Errorf("want ErrBlobReplay, got %v", err)
	}
}

func TestMemStore_QuorumBind(t *testing.T) {
	store := airimportgate.NewMemStore()
	ctx := context.Background()
	now := time.Date(2025, 11, 4, 10, 0, 0, 0, time.UTC)
	b := &airimportgate.QuorumBind{
		BindID:		"bind-001",
		SessionID:	"sess-001",
		LeaseID:	"lease-001",
		BindGen:	"ig-test-bind-001",
		Threshold:	3,
		ActualQuorum:	3,
		QuorumPassed:	true,
		BoundBy:	"alice",
		BoundAt:	now,
		CreatedAt:	now,
	}
	if err := store.SaveBind(ctx, b); err != nil {
		t.Fatalf("SaveBind: %v", err)
	}
	got, err := store.GetBindByLease(ctx, "lease-001")
	if err != nil {
		t.Fatalf("GetBindByLease: %v", err)
	}
	if got.BindID != b.BindID {
		t.Errorf("bind_id mismatch: want %s, got %s", b.BindID, got.BindID)
	}
	if !got.QuorumPassed {
		t.Error("quorum_passed should be true")
	}
	_, err = store.GetBindByLease(ctx, "nonexistent-lease")
	if err != airimportgate.ErrBindNotFound {
		t.Errorf("want ErrBindNotFound, got %v", err)
	}
}

func TestMemStore_ConflictState(t *testing.T) {
	store := airimportgate.NewMemStore()
	ctx := context.Background()
	now := time.Date(2025, 11, 4, 10, 0, 0, 0, time.UTC)
	p := &airimportgate.PhaseConflictProof{
		ConflictID:		"conflict-001",
		SessionID:		"sess-001",
		ConflictKind:		airimportgate.ConflictImportVsAbort,
		State:			airimportgate.ConflictOpen,
		WinnerOp:		"aborted",
		LoserOp:		"import",
		LoserActor:		"bob",
		PhaseAtConflict:	"aborted",
		CreatedAt:		now,
		UpdatedAt:		now,
	}
	if err := store.SaveConflict(ctx, p); err != nil {
		t.Fatalf("SaveConflict: %v", err)
	}
	// 升级到 proven
	t2 := now.Add(time.Minute)
	cp := p.Copy()
	cp.State = airimportgate.ConflictProven
	cp.ProvenAt = &t2
	if err := store.UpdateConflictState(ctx, p.ConflictID, airimportgate.ConflictOpen, airimportgate.ConflictProven, &cp); err != nil {
		t.Fatalf("UpdateConflictState: %v", err)
	}
	got, _ := store.GetConflict(ctx, p.ConflictID)
	if got.State != airimportgate.ConflictProven {
		t.Errorf("want proven, got %s", got.State)
	}
	// 再次用 open→proven CAS 应失败
	err := store.UpdateConflictState(ctx,
		p.ConflictID, airimportgate.ConflictOpen, airimportgate.ConflictProven, nil)
	if err != airimportgate.ErrInvalidTransition {
		t.Errorf("want ErrInvalidTransition, got %v", err)
	}
}

func TestMemStore_ListActiveLeasesForSession(t *testing.T) {
	now := time.Date(2025, 11, 4, 10, 0, 0, 0, time.UTC)
	store := airimportgate.NewMemStore().WithClock(fixedClock(now))
	ctx := context.Background()
	for i, state := range []airimportgate.LeaseState{airimportgate.LeaseOpen, airimportgate.LeaseImporting, airimportgate.LeaseImported, airimportgate.LeaseAborted} {
		l := &airimportgate.ImportLease{
			LeaseID:	"lease-list-" + string(rune('a'+i)),
			SessionID:	"sess-list-001",
			State:		state,
			ExpiresAt:	now.Add(time.Minute),
			CreatedAt:	now,
			UpdatedAt:	now,
		}
		_ = store.SaveLease(ctx, l)
	}
	actives, err := store.ListActiveLeasesForSession(ctx, "sess-list-001")
	if err != nil {
		t.Fatalf("ListActiveLeasesForSession: %v", err)
	}
	// open + importing = 2 个活跃
	if len(actives) != 2 {
		t.Errorf("want 2 active leases, got %d", len(actives))
	}
}

func TestMemStore_ListExpiredLeases(t *testing.T) {
	now := time.Date(2025,
		11, 4, 10, 0, 0, 0, time.UTC)
	store := airimportgate.NewMemStore().WithClock(fixedClock(now))
	ctx := context.Background()
	// 1 个已过期（expires_at < now）
	expired := &airimportgate.ImportLease{
		LeaseID:	"lease-expired-001",
		SessionID:	"sess-exp",
		State:		airimportgate.LeaseImporting,
		ExpiresAt:	now.Add(-time.Minute),
		CreatedAt:	now,
		UpdatedAt:	now,
	}
	// 1 个未过期
	valid := &airimportgate.ImportLease{
		LeaseID:	"lease-valid-001",
		SessionID:	"sess-exp",
		State:		airimportgate.LeaseOpen,
		ExpiresAt:	now.Add(time.Minute),
		CreatedAt:	now,
		UpdatedAt:	now,
	}
	_ = store.SaveLease(ctx, expired)
	_ = store.SaveLease(ctx, valid)
	list, err := store.ListExpiredLeases(ctx, 10)
	if err != nil {
		t.Fatalf("ListExpiredLeases: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("want 1 expired, got %d", len(list))
	}
	if list[0].LeaseID != expired.LeaseID {
		t.Errorf("wrong expired lease: %s", list[0].LeaseID)
	}
}

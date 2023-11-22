package utamodegate_test

import (
	"context"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/utamodegate"
)

func TestMemStore_BumpGeneration_CAS(t *testing.T) {
	cases := []struct {
		name		string
		preSetup	func(ctx context.Context, s *utamodegate.MemStore,)
		userID		string
		expected	int64
		newMode		utamodegate.MarginMode
		wantGen		int64
		wantErr		error
	}{
		{
			name:		"首次 BumpGeneration（expected=0）",
			userID:		"u-cas-01",
			expected:	0,
			newMode:	utamodegate.MarginModeClassic,
			wantGen:	1,
			wantErr:	nil,
		},
		{
			name:	"连续两次 BumpGeneration",
			preSetup: func(ctx context.Context, s *utamodegate.MemStore,) {
				_, _ = s.BumpGeneration(ctx, "u-cas-02", 0, utamodegate.MarginModeClassic)
			},
			userID:		"u-cas-02",
			expected:	1,
			newMode:	utamodegate.MarginModeUnified,
			wantGen:	2,
			wantErr:	nil,
		},
		{
			name:	"世代冲突 ErrStaleGen",
			preSetup: func(ctx context.Context, s *utamodegate.MemStore,) {
				_, _ = s.BumpGeneration(ctx, "u-cas-03", 0, utamodegate.MarginModeClassic)
			},
			userID:		"u-cas-03",
			expected:	0,	// 已是 1，期望 0 → 冲突
			newMode:	utamodegate.MarginModeUnified,
			wantErr:	utamodegate.ErrStaleGen,
		},
		{
			name:		"首次 expected 非 0 → 冲突",
			userID:		"u-cas-04",
			expected:	5,
			newMode:	utamodegate.MarginModeClassic,
			wantErr:	utamodegate.ErrStaleGen,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			s := utamodegate.NewMemStore()
			ctx := context.Background()
			if tc.preSetup != nil {
				tc.preSetup(ctx, s)
			}
			gen, err := s.BumpGeneration(ctx, tc.userID, tc.expected, tc.newMode)
			if err != tc.wantErr {
				t.Errorf("BumpGeneration error = %v, want %v", err, tc.wantErr)
			}
			if err == nil && gen != tc.wantGen {
				t.Errorf("gen = %d, want %d", gen, tc.wantGen)
			}
		})
	}
}

func TestMemStore_BumpGeneration_ConcurrentSafety(t *testing.T) {
	// 多 goroutine 并发 BumpGeneration(expected=0)，只有一个能成功；其余返回 ErrStaleGen。
	s := utamodegate.NewMemStore()
	ctx := context.Background()
	userID := "u-concurrent-cas"
	const n = 50
	var mu sync.Mutex
	successCount := 0
	staleCount := 0
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, err := s.BumpGeneration(ctx, userID, 0, utamodegate.MarginModeClassic)
			mu.Lock()
			if err == nil {
				successCount++
			} else if err == utamodegate.ErrStaleGen {
				staleCount++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	if successCount != 1 {
		t.Errorf("successCount = %d, want 1", successCount)
	}
	if staleCount != n-1 {
		t.Errorf("staleCount = %d, want %d", staleCount, n-1)
	}
	// 验证最终世代
	gen, _ := s.CurrentGeneration(ctx, userID)
	if gen != 1 {
		t.Errorf("final generation = %d, want 1", gen)
	}
}

func TestMemStore_SaveAndGetLease(t *testing.T) {
	s := utamodegate.NewMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	lease := &utamodegate.ModeLease{
		ID:		"lease-test-01",
		UserID:		"u-lease-01",
		FromMode:	utamodegate.MarginModeUnified,
		ToMode:		utamodegate.MarginModeClassic,
		Generation:	1,
		TriggerKind:	"test",
		TriggerRef:	"ref-01",
		Status:		utamodegate.LeaseOpen,
		CreatedAt:	now,
		UpdatedAt:	now,
	}
	if err := s.SaveLease(ctx, lease); err != nil {
		t.Fatalf("SaveLease: %v", err)
	}
	got, err := s.GetLease(ctx, "lease-test-01")
	if err != nil {
		t.Fatalf("GetLease: %v", err)
	}
	if got.Status != utamodegate.LeaseOpen {
		t.Errorf("got.Status = %s, want open", got.Status)
	}
	// 更新状态
	lease.Status = utamodegate.LeaseBound
	if err := s.SaveLease(ctx, lease); err != nil {
		t.Fatalf("SaveLease update: %v", err)
	}
	got2, _ := s.GetLease(ctx, "lease-test-01")
	if got2.Status != utamodegate.LeaseBound {
		t.Errorf("updated status = %s, want bound", got2.Status)
	}
	// 不存在的租约
	_, err = s.GetLease(ctx, "nonexistent")
	if err != utamodegate.ErrLeaseNotFound {
		t.Errorf("GetLease nonexistent: error = %v, want ErrLeaseNotFound", err)
	}
}

func TestMemStore_FindOpenDriftCase_Idempotent(t *testing.T) {
	s := utamodegate.NewMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	dc := &utamodegate.ModeDriftCase{
		ID:		"dc-idem-01",
		UserID:		"u-idem-dc",
		Asset:		"USDT",
		Kind:		utamodegate.DriftKindLiabilityKey,
		Status:		utamodegate.DriftOpen,
		DetectedAt:	now,
		UpdatedAt:	now,
	}
	_ = s.SaveDriftCase(ctx, dc)
	// 同用户+资产+Kind 已有 open 案件
	found, err := s.FindOpenDriftCase(ctx, "u-idem-dc", "USDT", utamodegate.DriftKindLiabilityKey)
	if err != nil {
		t.Fatalf("FindOpenDriftCase: %v", err)
	}
	if found == nil {
		t.Fatal("FindOpenDriftCase: should find existing case")
	}
	if found.ID != dc.ID {
		t.Errorf("found.ID = %s, want %s", found.ID, dc.ID)
	}
	// 关闭后不再查到
	dc.Status = utamodegate.DriftClosed
	_ = s.SaveDriftCase(ctx, dc)
	found2, _ := s.FindOpenDriftCase(ctx, "u-idem-dc", "USDT", utamodegate.DriftKindLiabilityKey)
	if found2 != nil {
		t.Error("FindOpenDriftCase: should not find closed case")
	}
}

func TestMemStore_ListExpiredOpenBinds(t *testing.T) {
	s := utamodegate.NewMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	// 已过期的绑定
	expired := &utamodegate.LiabilityBind{
		ID:		"bind-exp-01",
		UserID:		"u-exp-01",
		Status:		utamodegate.BindOpen,
		ExpiresAt:	now.Add(-1 * time.Hour),	// 1 小时前过期
		CreatedAt:	now.Add(-2 * time.Hour),
		UpdatedAt:	now.Add(-2 * time.Hour),
	}
	// 未过期的绑定
	active := &utamodegate.LiabilityBind{
		ID:		"bind-active-01",
		UserID:		"u-exp-01",
		Status:		utamodegate.BindOpen,
		ExpiresAt:	now.Add(1 * time.Hour),	// 1 小时后到期
		CreatedAt:	now,
		UpdatedAt:	now,
	}
	_ = s.SaveBind(ctx, expired)
	_ = s.SaveBind(ctx, active)
	results, err := s.ListExpiredOpenBinds(ctx, now, 100)
	if err != nil {
		t.Fatalf("ListExpiredOpenBinds: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expired count = %d, want 1", len(results))
	}
	if results[0].ID != expired.ID {
		t.Errorf("expired bind ID = %s, want %s", results[0].ID, expired.ID)
	}
}

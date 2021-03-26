package clrcreditgate_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/clrcreditgate"
)

// ──────────────────────────────────────────────────────────────────────────────
// stub ClearingService
// ──────────────────────────────────────────────────────────────────────────────

type stubClearing struct {
	releaseItem	clrcreditgate.ItemView
	releaseErr	error
	creditErr	error
	rollbackCalls	int
}

func (s *stubClearing) Release(_ context.Context, itemID, actor string, epoch int64, token string) (clrcreditgate.ItemView, error) {
	if s.releaseErr != nil {
		return clrcreditgate.ItemView{}, s.releaseErr
	}
	v := s.releaseItem
	v.ID = itemID
	return v, nil
}

func (s *stubClearing) Credit(_ context.Context, _, _, _ string) (map[string]string, error) {
	if s.creditErr != nil {
		return nil, s.creditErr
	}
	return map[string]string{"available": "100.000000000000000000"}, nil
}

func (s *stubClearing) RollbackApplied(_ context.Context, _ string) error {
	s.rollbackCalls++
	return nil
}

func (s *stubClearing) GetItem(_ context.Context, itemID string) (clrcreditgate.ItemView, error) {
	v := s.releaseItem
	v.ID = itemID
	return v, nil
}

func (s *stubClearing) GetBatchItems(_ context.Context, batchID string) ([]clrcreditgate.ItemView, error) {
	v := s.releaseItem
	v.BatchID = batchID
	return []clrcreditgate.ItemView{v}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// 固定时钟
// ──────────────────────────────────────────────────────────────────────────────

var epoch0 = time.Date(2025, 2, 15, 10, 0, 0, 0, time.UTC)

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// ──────────────────────────────────────────────────────────────────────────────
// 辅助
// ──────────────────────────────────────────────────────────────────────────────

func newTestEngine(svc clrcreditgate.ClearingService, cfg clrcreditgate.LeaseConfig, t time.Time) (*clrcreditgate.Engine, *clrcreditgate.MemStore,) {
	store := clrcreditgate.NewMemStore().WithClock(fixedClock(t))
	e := clrcreditgate.NewEngine(store, svc, clrcreditgate.WithEngineClock(fixedClock(t)), clrcreditgate.WithLeaseConfig(cfg))
	return e, store
}

// issueLeaseFor 快速签发一个 issued Lease 并返回 leaseID 和 fence。

func issueLeaseFor(t *testing.T, e *clrcreditgate.Engine, store *clrcreditgate.MemStore, itemID, batchID, actor string) (string, clrcreditgate.Fence,) {
	t.Helper()
	ctx := context.Background()
	res, err := e.ReleaseWithLease(ctx, itemID, actor, 0, "")
	if err != nil {
		t.Fatalf("issueLeaseFor %s: %v", itemID, err)
	}
	return res.Lease.LeaseID,
		res.Lease.Fence
}

// ──────────────────────────────────────────────────────────────────────────────
// TestReleaseWithLease
// ──────────────────────────────────────────────────────────────────────────────

func TestReleaseWithLease(t *testing.T) {
	type tc struct {
		name		string
		itemID		string
		actor		string
		releaseErr	error
		wantErr		bool
		wantErrIs	error
	}
	cases := []tc{
		{name: "normal release", itemID: "item-001", actor: "ops-a", wantErr: false},
		{name: "empty actor", itemID: "item-002", actor: "", wantErr: true, wantErrIs: clrcreditgate.ErrActor},
		{name: "clearing error propagated", itemID: "item-003", actor: "ops-b",
			releaseErr:	fmt.Errorf("clearing unavailable"), wantErr: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubClearing{
				releaseItem: clrcreditgate.ItemView{
					BatchID:	"batch-001", UserID: "user-001",
					Asset:	"BTC", Amount: "1.000000000000000000",
					Status:	"released",
				},
				releaseErr:	tc.releaseErr,
			}
			cfg := clrcreditgate.LeaseConfig{DefaultTTL: 10 * time.Minute}
			e, _ := newTestEngine(svc, cfg, epoch0)
			res, err := e.ReleaseWithLease(context.Background(), tc.itemID, tc.actor, 0, "")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
					t.Fatalf("expected %v, got %v", tc.wantErrIs, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Lease == nil {
				t.Fatal("expected lease, got nil")
			}
			if res.Lease.State != clrcreditgate.LeaseIssued {
				t.Fatalf("expected issued, got %s", res.Lease.State)
			}
			if res.Lease.ItemID != tc.itemID {
				t.Fatalf("item_id mismatch: want %s, got %s", tc.itemID, res.Lease.ItemID)
			}
		})
	}
}

// TestReleaseWithLeaseIdempotent 重复调用返回现有 issued Lease。
func TestReleaseWithLeaseIdempotent(t *testing.T) {
	svc := &stubClearing{
		releaseItem: clrcreditgate.ItemView{
			BatchID:	"batch-idem", UserID: "user-idem",
			Asset:	"ETH", Amount: "2.000000000000000000", Status: "released",
		},
	}
	cfg := clrcreditgate.LeaseConfig{DefaultTTL: 10 * time.Minute}
	e, _ := newTestEngine(svc, cfg, epoch0)
	ctx := context.Background()
	r1, err := e.ReleaseWithLease(ctx, "item-idem-01", "ops", 0, "")
	if err != nil || r1.Lease == nil {
		t.Fatalf("first release: %v", err)
	}
	r2, err := e.ReleaseWithLease(ctx, "item-idem-01", "ops", 0, "")
	if err != nil || r2.Lease == nil {
		t.Fatalf("second release: %v", err)
	}
	if r1.Lease.LeaseID != r2.Lease.LeaseID {
		t.Fatalf("expected same lease_id on idempotent call, got %s vs %s",
			r1.Lease.LeaseID, r2.Lease.LeaseID)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestApplyWithLease
// ──────────────────────────────────────────────────────────────────────────────

func TestApplyWithLease(t *testing.T) {
	type tc struct {
		name		string
		advance		time.Duration	// 时间推进（ TTL 过期）
		creditErr	error
		wantErr		bool
		wantErrIs	error
		wantKind	clrcreditgate.ProofKind
	}
	cases := []tc{
		{name: "credit success", wantErr: false, wantKind: clrcreditgate.ProofCreditOK},
		{name: "credit fail rolls back lease", creditErr: fmt.Errorf("ledger down"),
			wantErr:	true, wantKind: clrcreditgate.ProofCreditFail},
		{name: "lease expired", advance: 11 * time.Minute,
			wantErr:	true,
			wantErrIs:	clrcreditgate.ErrLeaseExpired, wantKind: clrcreditgate.ProofExpiredLease},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubClearing{
				releaseItem: clrcreditgate.ItemView{
					BatchID:	"batch-apply", UserID: "user-apply",
					Asset:	"BTC", Amount: "0.500000000000000000", Status: "released",
				},
				creditErr:	tc.creditErr,
			}
			cfg := clrcreditgate.LeaseConfig{DefaultTTL: 10 * time.Minute}
			now := epoch0
			e,
				store := newTestEngine(svc, cfg, now)
			_ = store
			ctx := context.Background()
			itemID := "item-apply-" + tc.name
			leaseID, fence := issueLeaseFor(t, e, store, itemID, "batch-apply", "ops")
			// 时间推进
			if tc.advance > 0 {
				e.clock = fixedClock(now.Add(tc.advance))
				store.WithClock(fixedClock(now.Add(tc.advance)))
			}
			res, err := e.ApplyWithLease(ctx, itemID, leaseID, "ops", fence)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
					t.Fatalf("expected err %v, got %v", tc.wantErrIs, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
			}
			if res != nil && res.Proof != nil {
				if res.Proof.Kind != tc.wantKind {
					t.Fatalf("expected proof kind %s, got %s", tc.wantKind, res.Proof.Kind)
				}
			}
			// Credit 失败时：rollbackApplied 应被调用
			if tc.creditErr != nil && svc.rollbackCalls == 0 {
				t.Fatal("expected RollbackApplied to be called on credit failure")
			}
		})
	}
}

// TestApplyWithLeaseExpiredState 直接操作 store 标记 expired，再 Apply 应 fail-closed。
func TestApplyWithLeaseExpiredState(t *testing.T) {
	svc := &stubClearing{
		releaseItem: clrcreditgate.ItemView{
			BatchID:	"batch-exp", UserID: "user-exp",
			Asset:	"BTC", Amount: "1.0", Status: "released",
		},
	}
	cfg := clrcreditgate.LeaseConfig{DefaultTTL: 10 * time.Minute}
	e, store := newTestEngine(svc, cfg, epoch0)
	ctx := context.Background()
	leaseID, fence := issueLeaseFor(t, e, store, "item-exp-01", "batch-exp", "ops")
	// 强制标记 expired
	if err := store.MarkLeaseExpired(ctx, leaseID, epoch0); err != nil {
		t.Fatalf("mark expired: %v", err)
	}
	res, err := e.ApplyWithLease(ctx, "item-exp-01", leaseID, "ops", fence)
	if !errors.Is(err, clrcreditgate.ErrLeaseExpired) {
		t.Fatalf("expected ErrLeaseExpired, got %v", err)
	}
	if res.Proof == nil {
		t.Fatal("expected proof on expired lease apply")
	}
	if res.Proof.Kind != clrcreditgate.ProofExpiredLease {
		t.Fatalf("expected proof kind expired_lease, got %s", res.Proof.Kind)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestDupApply 同 Item 并发两次 Apply，仅一次成功
// ──────────────────────────────────────────────────────────────────────────────

func TestDupApply(t *testing.T) {
	svc := &stubClearing{
		releaseItem: clrcreditgate.ItemView{
			BatchID:	"batch-dup", UserID: "user-dup",
			Asset:	"USDT", Amount: "100.000000000000000000", Status: "released",
		},
	}
	cfg := clrcreditgate.LeaseConfig{DefaultTTL: 10 * time.Minute}
	e, store := newTestEngine(svc, cfg, epoch0)
	ctx := context.Background()
	itemID := "item-dup-001"
	leaseID, fence := issueLeaseFor(t, e, store, itemID, "batch-dup", "ops")
	// 并发两次 Apply
	type result struct {
		res	*clrcreditgate.ApplyWithLeaseResult
		err	error
	}
	ch := make(chan result, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e2 := e.ApplyWithLease(ctx, itemID, leaseID, "ops", fence)
			ch <- result{r, e2}
		}()
	}
	wg.Wait()
	close(ch)
	var okCount, dupCount int
	for r := range ch {
		if r.err == nil {
			okCount++
		} else {
			dupCount++
		}
	}
	if okCount != 1 {
		t.Fatalf("expected exactly 1 successful apply, got %d", okCount)
	}
	if dupCount != 1 {
		t.Fatalf("expected exactly 1 dup rejection, got %d", dupCount)
	}
	// 验证 credit_ok 证明唯一性
	proofs, _ := store.ListProofs(ctx, clrcreditgate.ProofListFilter{
		ItemID:	itemID,
		Kinds:	[]clrcreditgate.ProofKind{clrcreditgate.ProofCreditOK},
	})
	if len(proofs) != 1 {
		t.Fatalf("expected exactly 1 credit_ok proof, got %d", len(proofs))
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestCloseBatchGuard
// ──────────────────────────────────────────────────────────────────────────────

func TestCloseBatchGuard(t *testing.T) {
	type tc struct {
		name		string
		setupLeases	int	// 签发多少 issued Lease
		consumeLeases	int	// 消费多少 Lease
		wantAllowed	bool
		wantDrain	clrcreditgate.DrainStatus
	}
	cases := []tc{
		{name: "no leases - allowed", setupLeases: 0, consumeLeases: 0, wantAllowed: true, wantDrain: clrcreditgate.DrainClosed},
		{name: "unconsumed leases - blocked", setupLeases: 2, consumeLeases: 0, wantAllowed: false, wantDrain: clrcreditgate.DrainDraining},
		{name: "all consumed - allowed", setupLeases: 2, consumeLeases: 2, wantAllowed: true, wantDrain: clrcreditgate.DrainClosed},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubClearing{
				releaseItem: clrcreditgate.ItemView{
					BatchID:	"batch-close", UserID: "user-close",
					Asset:	"BTC", Amount: "1.0", Status: "released",
				},
			}
			cfg := clrcreditgate.LeaseConfig{DefaultTTL: 10 * time.Minute, DrainTimeout: 5 * time.Minute}
			e, store := newTestEngine(svc, cfg, epoch0)
			ctx := context.Background()
			batchID := "batch-close"
			type issued struct {
				itemID	string
				leaseID	string
				fence	clrcreditgate.Fence
			}
			var issuedLeases []issued
			for i := 0; i < tc.setupLeases; i++ {
				itemID := fmt.Sprintf("item-close-%02d", i)
				lid, fence := issueLeaseFor(t, e, store, itemID, batchID, "ops")
				issuedLeases = append(issuedLeases,
					issued{itemID, lid, fence})
			}
			for i := 0; i < tc.consumeLeases && i < len(issuedLeases); i++ {
				lx := issuedLeases[i]
				if _, err := e.ApplyWithLease(ctx, lx.itemID, lx.leaseID, "ops", lx.fence); err != nil {
					t.Fatalf("consume lease %d: %v", i, err)
				}
			}
			res, err := e.CloseBatchGuard(ctx, batchID, "ops")
			if tc.wantAllowed {
				if err != nil && !errors.Is(err, clrcreditgate.ErrDrainBlocked) {
					t.Fatalf("unexpected err: %v", err)
				}
				if !res.Allowed {
					t.Fatalf("expected allowed, got blocked: %s", res.Reason)
				}
			} else {
				if !errors.Is(err, clrcreditgate.ErrDrainBlocked) {
					t.Fatalf("expected ErrDrainBlocked, got %v", err)
				}
				if res.Allowed {
					t.Fatal("expected blocked, got allowed")
				}
			}
			if res.DrainState != nil && res.DrainState.Status != tc.wantDrain {
				t.Fatalf("expected drain status %s, got %s", tc.wantDrain, res.DrainState.Status)
			}
		})
	}
}

// TestCloseBatchGuardNoReleasedItems 无任何 Lease 时 CloseBatch 立即允许。
func TestCloseBatchGuardNoReleasedItems(t *testing.T) {
	svc := &stubClearing{}
	cfg := clrcreditgate.LeaseConfig{DefaultTTL: 10 * time.Minute}
	e, _ := newTestEngine(svc, cfg, epoch0)
	ctx := context.Background()
	res, err := e.CloseBatchGuard(ctx, "batch-empty", "ops")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Allowed {
		t.Fatalf("expected allowed for empty batch, got blocked")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestVoidLease
// ──────────────────────────────────────────────────────────────────────────────

func TestVoidLease(t *testing.T) {
	type tc struct {
		name		string
		preState	clrcreditgate. // "" = leave as issued; "consumed" = consume first
		LeaseState

		wantErr		bool
		wantErrIs	error
	}
	cases := []tc{
		{name: "void issued lease", preState: "", wantErr: false},
		{name: "void already consumed lease", preState: clrcreditgate.LeaseConsumed, wantErr: true, wantErrIs: clrcreditgate.ErrInvalidTransition},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubClearing{
				releaseItem: clrcreditgate.ItemView{
					BatchID:	"batch-void", UserID: "user-void",
					Asset:	"BTC", Amount: "1.0", Status: "released",
				},
			}
			cfg := clrcreditgate.LeaseConfig{DefaultTTL: 10 * time.Minute}
			e, store := newTestEngine(svc, cfg, epoch0)
			ctx := context.Background()
			itemID := "item-void-" + tc.name
			leaseID, fence := issueLeaseFor(t, e, store, itemID, "batch-void", "ops")
			if tc.preState == clrcreditgate.LeaseConsumed {
				if _, err := e.ApplyWithLease(ctx, itemID, leaseID, "ops", fence); err != nil {
					t.Fatalf("pre-consume: %v", err)
				}
			}
			err := e.VoidLease(ctx, leaseID, "ops", "test_void")
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
					t.Fatalf("expected %v, got %v", tc.wantErrIs, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				l,
					_ := store.GetLease(ctx, leaseID)
				if l.State != clrcreditgate.LeaseVoid {
					t.Fatalf("expected void state, got %s", l.State)
				}
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestFenceStale Apply 使用旧 Fence 被拒绝
// ──────────────────────────────────────────────────────────────────────────────

func TestFenceStale(t *testing.T) {
	svc := &stubClearing{
		releaseItem: clrcreditgate.ItemView{
			BatchID:	"batch-fence", UserID: "user-fence",
			Asset:	"ETH", Amount: "5.0", Status: "released",
		},
	}
	cfg := clrcreditgate.LeaseConfig{DefaultTTL: 10 * time.Minute}
	e, store := newTestEngine(svc, cfg, epoch0)
	_ = store
	ctx := context.Background()
	itemID := "item-fence-001"
	leaseID, _ := issueLeaseFor(t, e, store, itemID, "batch-fence", "ops")
	// 使用错误的 Fence（token 不匹配）
	staleFence := clrcreditgate.Fence{Epoch: 1, Token: "wrong-token-00000000000000000000"}
	_, err := e.ApplyWithLease(ctx, itemID, leaseID, "ops", staleFence)
	if !errors.Is(err, clrcreditgate.ErrLeaseFenceStale) {
		t.Fatalf("expected ErrLeaseFenceStale, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestLeaseItemMismatch Apply 使用错误 ItemID 被拒绝
// ──────────────────────────────────────────────────────────────────────────────

func TestLeaseItemMismatch(t *testing.T) {
	svc := &stubClearing{
		releaseItem: clrcreditgate.ItemView{
			BatchID:	"batch-mismatch", UserID: "user-mis",
			Asset:	"BTC", Amount: "1.0", Status: "released",
		},
	}
	cfg := clrcreditgate.LeaseConfig{DefaultTTL: 10 * time.Minute}
	e, store := newTestEngine(svc, cfg, epoch0)
	ctx := context.Background()
	leaseID, fence := issueLeaseFor(t, e, store, "item-real", "batch-mismatch", "ops")
	_, err := e.ApplyWithLease(ctx, "item-fake", leaseID, "ops", fence)
	if !errors.Is(err, clrcreditgate.ErrLeaseItemMismatch) {
		t.Fatalf("expected ErrLeaseItemMismatch, got %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestExportProofs 导出证明幂等
// ──────────────────────────────────────────────────────────────────────────────

func TestExportProofs(t *testing.T) {
	svc := &stubClearing{
		releaseItem: clrcreditgate.ItemView{
			BatchID:	"batch-export", UserID: "user-export",
			Asset:	"BTC", Amount: "1.0", Status: "released",
		},
	}
	cfg := clrcreditgate.LeaseConfig{DefaultTTL: 10 * time.Minute}
	e, store := newTestEngine(svc, cfg, epoch0)
	_ = store
	ctx := context.Background()
	batchID := "batch-export"
	// 一次成功 Apply + 一次拒绝 Apply（用于触发两个证明）
	leaseID1, fence1 := issueLeaseFor(t, e, store, "item-exp-01", batchID, "ops")
	_, err := e.ApplyWithLease(ctx, "item-exp-01", leaseID1, "ops", fence1)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	proofs1, err := e.ExportProofs(ctx, batchID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	proofs2, _ := e.ExportProofs(ctx, batchID)
	if len(proofs1) != len(proofs2) {
		t.Fatal("export not idempotent")
	}
	if len(proofs1) == 0 {
		t.Fatal("expected at least 1 proof")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// TestWorkerScanExpiredLeases Worker 过期扫描
// ──────────────────────────────────────────────────────────────────────────────

func TestWorkerScanExpiredLeases(t *testing.T) {
	svc := &stubClearing{
		releaseItem: clrcreditgate.ItemView{
			BatchID:	"batch-wk", UserID: "user-wk",
			Asset:	"BTC", Amount: "1.0", Status: "released",
		},
	}
	cfg := clrcreditgate.LeaseConfig{DefaultTTL: 1 * time.Minute}
	e, store := newTestEngine(svc, cfg, epoch0)
	ctx := context.Background()
	// 签发 2 个 Lease
	for i := 0; i < 2; i++ {
		if _, err := e.ReleaseWithLease(ctx, fmt.Sprintf("item-wk-%02d", i), "ops", 0, ""); err != nil {
			t.Fatalf("release %d: %v", i, err)
		}
	}
	// 推进时间超过 TTL
	workerClock := fixedClock(epoch0.Add(2 * time.Minute))
	store.WithClock(workerClock)
	w := clrcreditgate.NewWorker(e, store, clrcreditgate.NewMetrics(nil), nil, clrcreditgate.WorkerConfig{BatchSize: 10})
	w.WithClock(workerClock)
	res := w.ScanExpiredLeases(ctx)
	if res.Expired != 2 {
		t.Fatalf("expected 2 expired, got %d", res.Expired)
	}
	// 验证 Lease 状态
	leases, _ := store.ListLeases(ctx, clrcreditgate.LeaseListFilter{States: []clrcreditgate.LeaseState{clrcreditgate.LeaseExpired}})
	if len(leases) != 2 {
		t.Fatalf("expected 2 expired leases in store, got %d", len(leases))
	}
}

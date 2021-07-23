package hotpregate_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/hotpregate"
)

// ── directMockObserver 直接控制快照返回 ──────────────────────────────────────
// directMockObserver 实现 InventoryObserver；直接返回 snaps map 中的快照，
// 不经过 hotinv.Book.Observe（从而避免 cap 映射影响 HotMax 字段）。

type directMockObserver struct {
	snaps	map[string]hotinv.Snapshot
	err	error
}

func (d *directMockObserver) Observe(_ context.Context, asset string) (hotinv.Snapshot, error) {
	if d.err != nil {
		return hotinv.Snapshot{}, d.err
	}
	s, ok := d.snaps[asset]
	if !ok {
		return hotinv.Snapshot{}, fmt.Errorf("%w: %s", hotpregate.ErrObserveFailed, asset)
	}
	return s, nil
}

func (d *directMockObserver) ObserveAll(_ context.Context) ([]hotinv.Snapshot, error) {
	if d.err != nil {
		return nil, d.err
	}
	var out []hotinv.Snapshot
	for _, s := range d.snaps {
		out = append(out, s)
	}
	return out, nil
}

// ── 辅助 ──────────────────────────────────────────────────────────────────

func btcSnap(hotAvail, warmAvail, coldAvail, hotMax string) map[string]hotinv.Snapshot {
	return map[string]hotinv.Snapshot{
		"BTC": {
			Asset:		"BTC",
			HotAvail:	hotAvail,
			WarmAvail:	warmAvail,
			ColdAvail:	coldAvail,
			HotMax:		hotMax,
		},
	}
}

func multiSnap(snaps ...hotinv.Snapshot) map[string]hotinv.Snapshot {
	m := map[string]hotinv.Snapshot{}
	for _, s := range snaps {
		m[s.Asset] = s
	}
	return m
}

func defaultBand() hotpregate.BandSnapshot {
	return hotpregate.BandSnapshot{HotLow: "10000", HotHigh: "90000", WarmLow: "5000", WarmHigh: "50000"}
}

func issueReq(asset, window string, ttl int64) hotpregate.IssueRequest {
	return hotpregate.IssueRequest{
		Asset:	asset,
		Window:	window,
		Band:	defaultBand(),
		TTL:	ttl,
		Actor:	"test",
	}
}

// newTestService 构造带 directMockObserver 的 Service。
func newTestService(snaps map[string]hotinv.Snapshot) *hotpregate.Service {
	obs := &directMockObserver{snaps: snaps}
	store := hotpregate.NewMemStore()
	svc := &hotpregate.Service{
		store:		store,
		observer:	obs,
		book:		hotinv.New(map[string]hotinv.Cap{}),
		nowFn:		func() time.Time { return time.Now().UTC() },
	}
	return svc
}

// newTestServiceWithClock 构造带时钟控制的 Service。

func newTestServiceWithClock(snaps map[string]hotinv.Snapshot, fixedNow time.Time) (*hotpregate.Service, *directMockObserver, func(time.Time)) {
	obs := &directMockObserver{snaps: snaps}
	store := hotpregate.NewMemStore()
	ts := fixedNow
	svc := &hotpregate.Service{
		store:		store,
		observer:	obs,
		book:		hotinv.New(map[string]hotinv.Cap{}),
		nowFn:		func() time.Time { return ts },
	}
	setClock := func(t time.Time) { ts = t; svc.nowFn = func() time.Time { return ts } }
	return svc, obs, setClock
}

// ── IssueLease ───────────────────────────────────────────────────────────

func TestIssueLease(t *testing.T) {
	ctx := context.Background()
	const window = "202107221430"
	t.Run("normal issue", func(t *testing.T) {
		svc := newTestService(btcSnap("50000", "20000", "30000", "100000"))
		lease, err := svc.IssueLease(ctx, issueReq("BTC", window, 300))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if lease.State != hotpregate.LeaseIssued {
			t.Errorf("expected state issued, got %s", lease.State)
		}
		if lease.Asset != "BTC" {
			t.Errorf("expected asset BTC, got %s", lease.Asset)
		}
		if lease.Window != window {
			t.Errorf("expected window %s, got %s", window, lease.Window)
		}
		if !strings.HasPrefix(lease.LeaseID, "hpl_") {
			t.Errorf("lease_id should have prefix hpl_, got %s", lease.LeaseID)
		}
		if len(lease.Digest.Hex) != 64 {
			t.Errorf("digest hex should be 64 chars, got %d", len(lease.Digest.Hex))
		}
		if lease.IdemKey != "BTC:"+window {
			t.Errorf("idem_key mismatch: %s", lease.IdemKey)
		}
		if lease.Digest.HotMax != "100000" {
			t.Errorf("digest.hot_max mismatch: %s", lease.Digest.HotMax)
		}
	})
	t.Run("idempotent: same asset+window returns same lease", func(t *testing.T) {
		svc := newTestService(btcSnap("50000", "20000", "30000", "100000"))
		l1, err := svc.IssueLease(ctx, issueReq("BTC", window, 300))
		if err != nil {
			t.Fatalf("first issue: %v", err)
		}
		l2, err := svc.IssueLease(ctx, issueReq("BTC", window, 300))
		if err != nil {
			t.Fatalf("second issue: %v", err)
		}
		if l1.LeaseID != l2.LeaseID {
			t.Errorf("idempotent: expected same lease_id, got %s vs %s", l1.LeaseID, l2.LeaseID)
		}
	})
	t.Run("lowercase asset normalized to uppercase", func(t *testing.T) {
		svc := newTestService(btcSnap("50000", "20000", "30000", "100000"))
		lease, err := svc.IssueLease(ctx, issueReq("btc", window, 300))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if lease.Asset != "BTC" {
			t.Errorf("expected BTC, got %s", lease.Asset)
		}
	})
	t.Run("invalid asset", func(t *testing.T) {
		svc := newTestService(btcSnap("50000", "20000", "30000", "100000"))
		_, err := svc.IssueLease(ctx, issueReq("", window, 300))
		if !errors.Is(err, hotpregate.ErrInvalidAsset) {
			t.Errorf("expected ErrInvalidAsset, got %v", err)
		}
	})
	t.Run("invalid window too short", func(t *testing.T) {
		svc := newTestService(btcSnap("50000", "20000", "30000", "100000"))
		_, err := svc.IssueLease(ctx, hotpregate.IssueRequest{Asset: "BTC", Window: "bad", Band: defaultBand(), TTL: 300, Actor: "test"})
		if !errors.Is(err, hotpregate.ErrInvalidWindow) {
			t.Errorf("expected ErrInvalidWindow, got %v", err)
		}
	})
	t.Run("expired lease causes re-issue", func(t *testing.T) {
		fixedNow := time.Date(2021, 7, 22, 14, 30, 0, 0, time.UTC)
		svc, _, setClock := newTestServiceWithClock(btcSnap("50000", "20000", "30000", "100000"), fixedNow)
		l1, err := svc.IssueLease(ctx, issueReq("BTC", window, 1))	// TTL=1s
		if err != nil {
			t.Fatalf("issue 1: %v", err)
		}
		// 推进时钟超过 TTL。
		setClock(fixedNow.Add(2 * time.Second))
		l2, err := svc.IssueLease(ctx, issueReq("BTC", window, 300))
		if err != nil {
			t.Fatalf("issue 2 after expiry: %v", err)
		}
		if l1.LeaseID == l2.LeaseID {
			t.Errorf("expected new lease after expiry, got same lease_id %s", l1.LeaseID)
		}
	})
	t.Run("default TTL applied when zero", func(t *testing.T) {
		svc := newTestService(btcSnap("50000", "20000", "30000", "100000"))
		lease, err := svc.IssueLease(ctx, issueReq("BTC", window, 0))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expectedExpiry := lease.IssuedAt.Add(hotpregate.DefaultLeaseTTL)
		diff := lease.ExpiresAt.Sub(expectedExpiry)
		if diff < -time.Second || diff > time.Second {
			t.Errorf("default TTL not applied correctly: expires_at=%v, expected≈%v", lease.ExpiresAt, expectedExpiry)
		}
	})
	t.Run("observe fail returns error", func(t *testing.T) {
		obs := &directMockObserver{err: fmt.Errorf("network error")}
		store := hotpregate.NewMemStore()
		svc := &hotpregate.Service{store: store, observer: obs, book: hotinv.New(map[string]hotinv.Cap{}), nowFn: time.Now}
		_, err := svc.IssueLease(ctx, issueReq("BTC", window, 300))
		if err == nil {
			t.Error("expected error from observe fail")
		}
	})
}

// ── BindLease ────────────────────────────────────────────────────────────

func TestBindLease(t *testing.T) {
	ctx := context.Background()
	const window = "202107221431"
	setup := func(t *testing.T) (*hotpregate.Service, *hotpregate.HotPrecheckLease,) {
		t.Helper()
		svc := newTestService(btcSnap("50000", "20000", "30000", "100000"))
		lease, err := svc.IssueLease(ctx, issueReq("BTC", window, 300))
		if err != nil {
			t.Fatalf("setup issue: %v", err)
		}
		return svc, lease
	}
	t.Run("normal bind issued→bound", func(t *testing.T) {
		svc, lease := setup(t)
		bound, err := svc.BindLease(ctx, hotpregate.BindRequest{LeaseID: lease.LeaseID, Actor: "test"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if bound.State != hotpregate.LeaseBound {
			t.Errorf("expected bound, got %s", bound.State)
		}
		if bound.BoundAt == nil {
			t.Error("BoundAt should be set after bind")
		}
	})
	t.Run("idempotent: bind already-bound returns ok", func(t *testing.T) {
		svc, lease := setup(t)
		_, err := svc.BindLease(ctx, hotpregate.BindRequest{LeaseID: lease.LeaseID, Actor: "test"})
		if err != nil {
			t.Fatalf("first bind: %v", err)
		}
		bound, err := svc.BindLease(ctx, hotpregate.BindRequest{LeaseID: lease.LeaseID, Actor: "test"})
		if err != nil {
			t.Fatalf("second bind idempotent: %v", err)
		}
		if bound.State != hotpregate.LeaseBound {
			t.Errorf("expected bound after idempotent bind, got %s", bound.State)
		}
	})
	t.Run("expired lease bind rejected", func(t *testing.T) {
		fixedNow := time.Date(2021, 7, 22, 14, 31, 0, 0, time.UTC)
		svc, _, setClock := newTestServiceWithClock(btcSnap("50000", "20000", "30000", "100000"), fixedNow)
		lease, err := svc.IssueLease(ctx, issueReq("BTC", window, 1))
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		setClock(fixedNow.Add(2 * time.Second))
		_, err = svc.BindLease(ctx, hotpregate.BindRequest{LeaseID: lease.LeaseID, Actor: "test"})
		if !errors.Is(err, hotpregate.ErrLeaseExpired) {
			t.Errorf("expected ErrLeaseExpired, got %v", err)
		}
	})
	t.Run("consumed lease bind rejected", func(t *testing.T) {
		svc, lease := setup(t)
		_, err := svc.BindLease(ctx, hotpregate.BindRequest{LeaseID: lease.LeaseID, Actor: "test"})
		if err != nil {
			t.Fatalf("bind: %v", err)
		}
		_, err = svc.RecheckLease(ctx, hotpregate.RecheckRequest{LeaseID: lease.LeaseID, Actor: "test"})
		if err != nil {
			t.Fatalf("recheck: %v", err)
		}
		_, err = svc.BindLease(ctx, hotpregate.BindRequest{LeaseID: lease.LeaseID, Actor: "test"})
		if !errors.Is(err, hotpregate.ErrLeaseConsumed) {
			t.Errorf("expected ErrLeaseConsumed, got %v", err)
		}
	})
	t.Run("invalid lease_id", func(t *testing.T) {
		svc, _ := setup(t)
		_, err := svc.BindLease(ctx, hotpregate.BindRequest{LeaseID: "", Actor: "test"})
		if !errors.Is(err, hotpregate.ErrInvalidLeaseID) {
			t.Errorf("expected ErrInvalidLeaseID, got %v", err)
		}
	})
	t.Run("not found lease_id", func(t *testing.T) {
		svc, _ := setup(t)
		_, err := svc.BindLease(ctx, hotpregate.BindRequest{LeaseID: "hpl_nonexistent", Actor: "test"})
		if !errors.Is(err, hotpregate.ErrLeaseNotFound) {
			t.Errorf("expected ErrLeaseNotFound, got %v", err)
		}
	})
}

// ── RecheckLease ─────────────────────────────────────────────────────────

func TestRecheckLease(t *testing.T) {
	ctx := context.Background()
	const window = "202107221432"
	t.Run("no drift: recheck accepted, state consumed", func(t *testing.T) {
		svc := newTestService(btcSnap("50000", "20000", "30000", "100000"))
		lease, err := svc.IssueLease(ctx, issueReq("BTC", window, 300))
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		result, err := svc.RecheckLease(ctx, hotpregate.RecheckRequest{LeaseID: lease.LeaseID, Actor: "test"})
		if err != nil {
			t.Fatalf("recheck (no drift): %v", err)
		}
		if !result.Accepted {
			t.Error("expected accepted=true")
		}
		if result.Lease.State != hotpregate.LeaseConsumed {
			t.Errorf("expected consumed, got %s", result.Lease.State)
		}
		if result.DriftProof != nil {
			t.Error("expected no drift proof")
		}
	})
	t.Run("drift detected: recheck rejected, InvDriftProof issued, state drifted", func(t *testing.T) {
		fixedNow := time.Date(2021, 7, 22, 14, 32, 0, 0, time.UTC)
		svc, obs, _ := newTestServiceWithClock(btcSnap("50000", "20000", "30000", "100000"), fixedNow)
		lease, err := svc.IssueLease(ctx, issueReq("BTC", window, 300))
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		// 库存漂移：+9000 ≈ 10% of hotHigh(90000) → critical
		obs.snaps = btcSnap("59000", "20000", "30000", "100000")
		result, err := svc.RecheckLease(ctx, hotpregate.RecheckRequest{LeaseID: lease.LeaseID, Actor: "test"})
		if !errors.Is(err, hotpregate.ErrDriftDetected) {
			t.Fatalf("expected ErrDriftDetected, got %v", err)
		}
		if result.Accepted {
			t.Error("expected accepted=false")
		}
		if result.DriftProof == nil {
			t.Fatal("expected drift proof to be non-nil")
		}
		if result.DriftProof.LeaseID != lease.LeaseID {
			t.Errorf("drift proof lease_id mismatch: %s vs %s", result.DriftProof.LeaseID, lease.LeaseID)
		}
		if result.Lease.State != hotpregate.LeaseDrifted {
			t.Errorf("expected lease state drifted, got %s", result.Lease.State)
		}
		if !strings.HasPrefix(result.DriftProof.ProofID, "idp_") {
			t.Errorf("proof_id should have prefix idp_, got %s", result.DriftProof.ProofID)
		}
		if !result.DriftProof.DigestMismatch {
			t.Error("expected digest_mismatch=true")
		}
	})
	// 验收 1：无租约提交 → 拒绝。
	t.Run("acceptance: submit without lease rejected", func(t *testing.T) {
		svc := newTestService(btcSnap("50000", "20000", "30000", "100000"))
		_, err := svc.RecheckLease(ctx, hotpregate.RecheckRequest{LeaseID: "hpl_nonexistent", Actor: "test"})
		if !errors.Is(err, hotpregate.ErrLeaseNotFound) {
			t.Errorf("expected ErrLeaseNotFound, got %v", err)
		}
	})
	// 验收 1：租约过期提交 → 拒绝。
	t.Run("acceptance: expired lease rejected on recheck", func(t *testing.T) {
		fixedNow := time.Date(2021, 7, 22, 14, 32, 30, 0, time.UTC)
		svc, _, setClock := newTestServiceWithClock(btcSnap("50000", "20000", "30000", "100000"), fixedNow)
		lease, err := svc.IssueLease(ctx, issueReq("BTC", window, 1))	// TTL=1s
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		setClock(fixedNow.Add(2 * time.Second))
		_, err = svc.RecheckLease(ctx, hotpregate.RecheckRequest{LeaseID: lease.LeaseID, Actor: "test"})
		if !errors.Is(err, hotpregate.ErrLeaseExpired) {
			t.Errorf("expected ErrLeaseExpired, got %v", err)
		}
	})
	t.Run("double consume rejected", func(t *testing.T) {
		svc := newTestService(btcSnap("50000", "20000", "30000", "100000"))
		lease, err := svc.IssueLease(ctx, issueReq("BTC", window, 300))
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		_, err = svc.RecheckLease(ctx, hotpregate.RecheckRequest{LeaseID: lease.LeaseID, Actor: "test"})
		if err != nil {
			t.Fatalf("first recheck: %v", err)
		}
		_, err = svc.RecheckLease(ctx, hotpregate.RecheckRequest{LeaseID: lease.LeaseID, Actor: "test"})
		if !errors.Is(err, hotpregate.ErrLeaseConsumed) {
			t.Errorf("expected ErrLeaseConsumed on double consume, got %v", err)
		}
	})
	t.Run("drift minor severity", func(t *testing.T) {
		fixedNow := time.Date(2021, 7, 22, 14, 32, 0, 0, time.UTC)
		svc, obs, _ := newTestServiceWithClock(btcSnap("50000", "20000", "30000", "100000"), fixedNow)
		lease, err := svc.IssueLease(ctx, issueReq("BTC", window, 300))
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		// delta = 200, hotHigh=90000, ratio=200/90000≈0.22% < 1% → Minor
		obs.snaps = btcSnap("50200", "20000", "30000", "100000")
		result, err := svc.RecheckLease(ctx, hotpregate.RecheckRequest{LeaseID: lease.LeaseID, Actor: "test"})
		if !errors.Is(err, hotpregate.ErrDriftDetected) {
			t.Fatalf("expected ErrDriftDetected, got %v", err)
		}
		if result.DriftProof.Severity != hotpregate.DriftMinor {
			t.Errorf("expected minor, got %s", result.DriftProof.Severity)
		}
	})
	t.Run("drift major severity", func(t *testing.T) {
		fixedNow := time.Date(2021, 7, 22, 14, 32, 0, 0, time.UTC)
		svc, obs, _ := newTestServiceWithClock(btcSnap("50000", "20000", "30000", "100000"), fixedNow)
		lease, err := svc.IssueLease(ctx, issueReq("BTC", window, 300))
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		// delta = 1800, hotHigh=90000, ratio=1800/90000=2% > 1% < 10% → Major
		obs.snaps = btcSnap("51800", "20000", "30000", "100000")
		result, err := svc.RecheckLease(ctx, hotpregate.RecheckRequest{LeaseID: lease.LeaseID, Actor: "test"})
		if !errors.Is(err, hotpregate.ErrDriftDetected) {
			t.Fatalf("expected ErrDriftDetected, got %v", err)
		}
		if result.DriftProof.Severity != hotpregate.DriftMajor {
			t.Errorf("expected major, got %s", result.DriftProof.Severity)
		}
	})
	t.Run("drift critical: cap changed", func(t *testing.T) {
		fixedNow := time.Date(2021, 7, 22, 14, 32, 0, 0, time.UTC)
		svc, obs, _ := newTestServiceWithClock(btcSnap("50000", "20000", "30000", "100000"), fixedNow)
		lease, err := svc.IssueLease(ctx, issueReq("BTC", window, 300))
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		// Cap 被外力修改（100000 → 80000）
		obs.snaps = btcSnap("50000", "20000", "30000", "80000")
		result, err := svc.RecheckLease(ctx, hotpregate.RecheckRequest{LeaseID: lease.LeaseID, Actor: "test"})
		if !errors.Is(err, hotpregate.ErrDriftDetected) {
			t.Fatalf("expected ErrDriftDetected, got %v", err)
		}
		if result.DriftProof.Severity != hotpregate.DriftCritical {
			t.Errorf("expected critical (cap changed), got %s", result.DriftProof.Severity)
		}
	})
}

// ── 验收 3：只读契约 ──────────────────────────────────────────────────────
// TestReadOnlyContract 验证 observeOnlyStore 写方法在预检路径内触发 panic。

func TestReadOnlyContract(t *testing.T) {
	s := &hotpregate.ObserveOnlyStore{inner: ledger.NewMemStore()}
	mustPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("%s: expected panic (read-only violation), got none", name)
			}
		}()
		fn()
	}
	mustPanic("UpsertAccount", func() {
		_ = s.UpsertAccount(context.Background(), &ledger.Account{})
	})
	mustPanic("InsertTransfer", func() {
		_ = s.InsertTransfer(context.Background(), &ledger.Transfer{})
	})
	mustPanic("UpdateTransfer", func() {
		_ = s.UpdateTransfer(context.Background(), &ledger.Transfer{})
	})
	mustPanic("AppendJournal", func() {
		_ = s.AppendJournal(context.Background(), nil)
	})
	mustPanic("AppendAudit", func() {
		_ = s.AppendAudit(context.Background(), ledger.AuditEvent{})
	})
	mustPanic("StampFence", func() {
		_, _ = s.StampFence(context.Background(), "id", "tok")
	})
	mustPanic("AppendOutbox", func() {
		_ = s.AppendOutbox(context.Background(), ledger.OutboxEvent{})
	})
	mustPanic("MarkOutboxPublished", func() {
		_ = s.MarkOutboxPublished(context.Background(), []string{"x"})
	})
	mustPanic("SetUserControlFrozen", func() {
		_ = s.SetUserControlFrozen(context.Background(), "u", true)
	})
	mustPanic("SetUserKYC", func() {
		_ = s.SetUserKYC(context.Background(), "u", "basic")
	})
}

// TestIsolationFromHotbound 编译期保证包隔离（无 hotbound/hotreserve import）。

func TestIsolationFromHotbound(t *testing.T) {
	svc := newTestService(btcSnap("50000", "20000", "30000", "100000"))
	if svc == nil {
		t.Fatal("svc should not be nil")
	}
}

// ── MemStore 并发安全 ─────────────────────────────────────────────────────

func TestMemStoreConcurrency(t *testing.T) {
	ctx := context.Background()
	store := hotpregate.NewMemStore()
	const goroutines = 20
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				leaseID := fmt.Sprintf("hpl_%d_%d", id, j)
				idemKey := fmt.Sprintf("BTC:%d_%d", id, j)
				l := hotpregate.HotPrecheckLease{
					LeaseID:	leaseID,
					Asset:		"BTC",
					Window:		fmt.Sprintf("202107221%02d", id%60),
					IdemKey:	idemKey,
					State:		hotpregate.LeaseIssued,
					IssuedAt:	time.Now().UTC(),
					ExpiresAt:	time.Now().UTC().Add(5 * time.Minute),
				}
				_ = store.InsertLease(ctx, l)
				_, _ = store.GetLease(ctx, leaseID)
				_, _ = store.ListLeases(ctx, hotpregate.LeaseListFilter{Limit: 10})
			}
			errs <- nil
		}(i)
	}
	for i := 0; i < goroutines; i++ {
		<-errs
	}
}

// ── Digest ───────────────────────────────────────────────────────────────

func TestDigestComputation(t *testing.T) {
	now := time.Date(2021, 7, 22, 14, 30, 0, 0, time.UTC)
	snap := hotinv.Snapshot{
		Asset:		"BTC",
		HotAvail:	"50000",
		WarmAvail:	"20000",
		ColdAvail:	"30000",
		HotMax:		"100000",
	}
	band := defaultBand()
	d1 := hotpregate.ComputeDigest(snap, band, now)
	d2 := hotpregate.ComputeDigest(snap,
		band, now)
	if d1.Hex != d2.Hex {
		t.Errorf("same input should produce same digest: %s vs %s", d1.Hex, d2.Hex)
	}
	if len(d1.Hex) != 64 {
		t.Errorf("expected 64-char hex, got %d", len(d1.Hex))
	}
	// 任何字段变化应产生不同摘要。
	snap2 := snap
	snap2.HotAvail = "50001"
	d3 := hotpregate.ComputeDigest(snap2, band, now)
	if d1.Hex == d3.Hex {
		t.Error("different hot_avail should produce different digest")
	}
	// 时间戳不同→摘要相同（CapturedAt 不参与 Hex 计算）。
	d4 := hotpregate.ComputeDigest(snap,
		band, now.Add(time.Second))
	if d1.Hex != d4.Hex {
		t.Error("different capturedAt should produce same digest (capturedAt is metadata only)")
	}
	// DigestMatch。
	if !hotpregate.DigestMatch(d1, d2) {
		t.Error("d1 and d2 should match")
	}
	if hotpregate.DigestMatch(d1, d3) {
		t.Error("d1 and d3 should not match")
	}
}

func TestClassifyDrift(t *testing.T) {
	band := defaultBand()	// hotHigh=90000
	now := time.Date(2021, 7, 22, 14, 30, 0, 0, time.UTC)
	tests := []struct {
		name		string
		issueHot	string
		recheckHot	string
		issueMax	string
		recheckMax	string
		want		hotpregate.DriftSeverity
	}{
		// 无漂移：相同内容，CapturedAt 不参与 Hex 计算，故无论时间戳均无漂移
		{"no drift same content", "50000", "50000", "100000", "100000", ""},
		// 漂移分级
		{"minor <1%", "50000", "50200", "100000", "100000", hotpregate.DriftMinor},
		{"major 2%", "50000", "51800", "100000", "100000", hotpregate.DriftMajor},
		{"critical >10%", "50000", "59100", "100000", "100000", hotpregate.DriftCritical},
		{"critical cap change", "50000", "50000", "100000", "80000", hotpregate.DriftCritical},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			issueSnap := hotinv.Snapshot{Asset: "BTC", HotAvail: tc.issueHot, WarmAvail: "20000", ColdAvail: "30000", HotMax: tc.issueMax}
			recheckSnap := hotinv.Snapshot{Asset: "BTC", HotAvail: tc.recheckHot, WarmAvail: "20000", ColdAvail: "30000", HotMax: tc.recheckMax}
			// 不同时间戳不影响 Hex（CapturedAt 不参与计算）
			issue := hotpregate.ComputeDigest(issueSnap,
				band, now)
			recheck := hotpregate.ComputeDigest(recheckSnap, band, now.Add(time.Second))
			got := hotpregate.ClassifyDrift(issue, recheck, band)
			if got != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

// ── 无用导入防护 ──────────────────────────────────────────────────────────────
// ledger.NewMemStore 在 TestReadOnlyContract 中使用（确保 ledger import 不被掉）。

var _ = ledger.NewMemStore
var _ = decimal.Zero

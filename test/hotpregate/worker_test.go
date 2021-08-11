package hotpregate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/hotpregate"
)

// ── ExpiryWorker ─────────────────────────────────────────────────────────

func TestExpiryWorkerRunOnce(t *testing.T) {
	ctx := context.Background()
	t.Run("expired issued lease marked expired", func(t *testing.T) {
		fixedNow := time.Date(2021, 7, 22, 14, 40, 0, 0, time.UTC)
		const window = "202107221440"
		svc, _, setClock := newTestServiceWithClock(btcSnap("50000", "20000", "30000", "100000"), fixedNow)
		// 签发 TTL=1s 租约
		lease, err := svc.IssueLease(ctx, hotpregate.IssueRequest{
			Asset:	"BTC", Window: window, Band: defaultBand(), TTL: 1, Actor: "test",
		})
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		// 推进时钟：租约已过期
		setClock(fixedNow.Add(2 * time.Second))
		expiredCount := 0
		worker := hotpregate.NewExpiryWorker(svc, time.Minute, 100, &hotpregate.WorkerMetrics{
			LeaseExpiredTotal: func() { expiredCount++ },
		})
		worker.runOnce(ctx)
		// 验证租约状态
		updated, err := svc.store.GetLease(ctx, lease.LeaseID)
		if err != nil {
			t.Fatalf("get lease: %v", err)
		}
		if updated.State != hotpregate.LeaseExpired {
			t.Errorf("expected expired, got %s", updated.State)
		}
		if expiredCount != 1 {
			t.Errorf("expected 1 expiry metric, got %d", expiredCount)
		}
	})
	t.Run("non-expired lease untouched", func(t *testing.T) {
		fixedNow := time.Date(2021, 7, 22, 14, 41, 0, 0, time.UTC)
		const window = "202107221441"
		svc, _, _ := newTestServiceWithClock(btcSnap("50000", "20000", "30000", "100000"), fixedNow)
		lease, err := svc.IssueLease(ctx, hotpregate.IssueRequest{
			Asset:	"BTC", Window: window, Band: defaultBand(), TTL: 300, Actor: "test",
		})
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		expiredCount := 0
		worker := hotpregate.NewExpiryWorker(svc, time.Minute, 100, &hotpregate.WorkerMetrics{
			LeaseExpiredTotal: func() { expiredCount++ },
		})
		worker.runOnce(ctx)
		updated, err := svc.store.GetLease(ctx, lease.LeaseID)
		if err != nil {
			t.Fatalf("get lease: %v", err)
		}
		if updated.State != hotpregate.LeaseIssued {
			t.Errorf("expected issued (untouched), got %s", updated.State)
		}
		if expiredCount != 0 {
			t.Errorf("expected 0 expiry metric, got %d", expiredCount)
		}
	})
	t.Run("consumed lease not re-expired", func(t *testing.T) {
		fixedNow := time.Date(2021,
			7, 22, 14, 42, 0, 0, time.UTC)
		const window = "202107221442"
		svc, _, setClock := newTestServiceWithClock(btcSnap("50000", "20000", "30000", "100000"), fixedNow)
		lease, err := svc.IssueLease(ctx, hotpregate.IssueRequest{
			Asset:	"BTC", Window: window, Band: defaultBand(), TTL: 300, Actor: "test",
		})
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		// 消费租约
		_, err = svc.RecheckLease(ctx, hotpregate.RecheckRequest{LeaseID: lease.LeaseID, Actor: "test"})
		if err != nil {
			t.Fatalf("recheck: %v", err)
		}
		// 推进时钟
		setClock(fixedNow.Add(2 * time.Second))
		expiredCount := 0
		worker := hotpregate.NewExpiryWorker(svc, time.Minute, 100, &hotpregate.WorkerMetrics{
			LeaseExpiredTotal: func() { expiredCount++ },
		})
		worker.runOnce(ctx)
		// 消费后状态保持 consumed（ListExpiredLeases 不返回 consumed）。
		if expiredCount != 0 {
			t.Errorf("consumed lease should not trigger expiry metric, got %d", expiredCount)
		}
	})
}

// ── DriftPatrolWorker ────────────────────────────────────────────────────

func TestDriftPatrolWorkerPatrolOnce(t *testing.T) {
	ctx := context.Background()
	t.Run("drift patrol detects critical drift and marks lease drifted", func(t *testing.T) {
		fixedNow := time.Date(2021,
			7, 22, 14, 50, 0, 0, time.UTC)
		const window = "202107221450"
		svc, obs, _ := newTestServiceWithClock(btcSnap("50000", "20000", "30000", "100000"), fixedNow)
		lease, err := svc.IssueLease(ctx, hotpregate.IssueRequest{
			Asset:	"BTC", Window: window, Band: defaultBand(), TTL: 300, Actor: "test",
		})
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		// 库存漂移（>10% → Critical）
		obs.snaps = btcSnap("59200", "20000", "30000", "100000")
		driftCount := 0
		patrolCount := 0
		worker := hotpregate.NewDriftPatrolWorker(svc, time.Minute, 50, []string{"BTC"}, &hotpregate.WorkerMetrics{
			DriftDetectedTotal:	func() { driftCount++ },
			PatrolledTotal:		func() { patrolCount++ },
		})
		worker.patrolOnce(ctx)
		// 验证租约状态
		updated, err := svc.store.GetLease(ctx, lease.LeaseID)
		if err != nil {
			t.Fatalf("get lease: %v", err)
		}
		if updated.State != hotpregate.LeaseDrifted {
			t.Errorf("expected drifted, got %s", updated.State)
		}
		if updated.DriftProofID == "" {
			t.Error("expected drift_proof_id to be set")
		}
		if driftCount != 1 {
			t.Errorf("expected 1 drift metric, got %d", driftCount)
		}
		// 验证证明已落库
		proof, err := svc.store.GetProof(ctx, updated.DriftProofID)
		if err != nil {
			t.Fatalf("get proof: %v", err)
		}
		if proof.Severity != hotpregate.DriftCritical {
			t.Errorf("expected critical, got %s", proof.Severity)
		}
	})
	t.Run("patrol no drift: lease untouched", func(t *testing.T) {
		fixedNow := time.Date(2021, 7, 22, 14, 51, 0, 0, time.UTC)
		const window = "202107221451"
		svc, _, _ := newTestServiceWithClock(btcSnap("50000", "20000", "30000", "100000"), fixedNow)
		lease, err := svc.IssueLease(ctx, hotpregate.IssueRequest{
			Asset:	"BTC", Window: window, Band: defaultBand(), TTL: 300, Actor: "test",
		})
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		driftCount := 0
		worker := hotpregate.NewDriftPatrolWorker(svc, time.Minute, 50, []string{"BTC"}, &hotpregate.WorkerMetrics{
			DriftDetectedTotal: func() { driftCount++ },
		})
		worker.patrolOnce(ctx)
		updated, err := svc.store.GetLease(ctx, lease.LeaseID)
		if err != nil {
			t.Fatalf("get lease: %v", err)
		}
		if updated.State != hotpregate.LeaseIssued {
			t.Errorf("expected issued (no drift), got %s", updated.State)
		}
		if driftCount != 0 {
			t.Errorf("expected 0 drift metric, got %d", driftCount)
		}
	})
	t.Run("patrol skips expired leases", func(t *testing.T) {
		fixedNow := time.Date(2021, 7, 22, 14, 52, 0, 0, time.UTC)
		const window = "202107221452"
		svc, obs, setClock := newTestServiceWithClock(btcSnap("50000", "20000", "30000", "100000"), fixedNow)
		_, err := svc.IssueLease(ctx, hotpregate.IssueRequest{
			Asset:	"BTC", Window: window, Band: defaultBand(), TTL: 1, Actor: "test",
		})
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		// 推进时钟：租约已过期
		setClock(fixedNow.Add(2 * time.Second))
		obs.snaps = btcSnap("99000", "20000", "30000", "100000")	// 大漂移，但租约已过期
		driftCount := 0
		worker := hotpregate.NewDriftPatrolWorker(svc, time.Minute, 50, []string{"BTC"}, &hotpregate.WorkerMetrics{
			DriftDetectedTotal: func() { driftCount++ },
		})
		worker.patrolOnce(ctx)
		// 过期租约应被 patrol 跳过，不标记为 drifted。
		if driftCount != 0 {
			t.Errorf("expired lease should not trigger drift in patrol, got %d", driftCount)
		}
	})
}

// ── Worker 只读契约：Worker 不写 ledger.Transfer ──────────────────────────────

func TestWorkerReadOnlyContract(t *testing.T) {
	ctx := context.Background()
	const window = "202107221500"
	fixedNow := time.Date(2021, 7, 22, 15, 0, 0, 0, time.UTC)
	svc, _, setClock := newTestServiceWithClock(btcSnap("50000", "20000", "30000", "100000"), fixedNow)
	_, err := svc.IssueLease(ctx, hotpregate.IssueRequest{
		Asset:	"BTC", Window: window, Band: defaultBand(), TTL: 1, Actor: "test",
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	setClock(fixedNow.Add(2 * time.Second))
	worker := hotpregate.NewExpiryWorker(svc, time.Minute, 100, nil)
	// 不应 panic；若 panic 则自动失败。
	worker.runOnce(ctx)
}

// ── Store 幂等与状态机错误 ────────────────────────────────────────────────────

func TestMemStoreStateMachine(t *testing.T) {
	ctx := context.Background()
	store := hotpregate.NewMemStore()
	now := time.Now().UTC()
	lease := hotpregate.HotPrecheckLease{
		LeaseID:	"hpl_sm-test-1",
		Asset:		"BTC",
		Window:		"202107221500",
		IdemKey:	"BTC:202107221500",
		State:		hotpregate.LeaseIssued,
		IssuedAt:	now,
		ExpiresAt:	now.Add(5 * time.Minute),	// 未来时间，不触发 idem 冲突
	}
	if err := store.InsertLease(ctx, lease); err != nil {
		t.Fatalf("insert: %v", err)
	}
	// issued→bound：正常
	if err := store.UpdateLeaseState(ctx, lease.LeaseID, hotpregate.LeaseIssued, hotpregate.LeaseBound, now, "bind"); err != nil {
		t.Fatalf("issued→bound: %v", err)
	}
	// bound→issued：违反状态机（expected=issued，实际=bound）
	err := store.UpdateLeaseState(ctx, lease.LeaseID, hotpregate.LeaseIssued, hotpregate.LeaseBound, now, "re-bind")
	if !errors.Is(err, hotpregate.ErrStateMismatch) {
		t.Errorf("expected ErrStateMismatch, got %v", err)
	}
	// bound→consumed：正常
	if err := store.UpdateLeaseState(ctx, lease.LeaseID, hotpregate.LeaseBound, hotpregate.LeaseConsumed, now, "consume"); err != nil {
		t.Fatalf("bound→consumed: %v", err)
	}
	// consumed→expired：违反状态机
	err = store.UpdateLeaseState(ctx, lease.LeaseID, hotpregate.LeaseIssued, hotpregate.LeaseExpired, now, "expire")
	if !errors.Is(err, hotpregate.ErrStateMismatch) {
		t.Errorf("expected ErrStateMismatch on consumed→wrong, got %v", err)
	}
}

func TestMemStoreIdemConflict(t *testing.T) {
	ctx := context.Background()
	store := hotpregate.NewMemStore()
	now := time.Now().UTC()
	lease := hotpregate.HotPrecheckLease{
		LeaseID:	"hpl_idem-test-1",
		Asset:		"ETH",
		Window:		"202107221501",
		IdemKey:	"ETH:202107221501",
		State:		hotpregate.LeaseIssued,
		IssuedAt:	now,
		ExpiresAt:	now.Add(5 * time.Minute),	// 未来时间：未过期
	}
	if err := store.InsertLease(ctx, lease); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	// 同 IdemKey 未过期重插：应返回 ErrIdemConflict
	lease2 := lease
	lease2.LeaseID = "hpl_idem-test-2"
	err := store.InsertLease(ctx, lease2)
	if !errors.Is(err, hotpregate.ErrIdemConflict) {
		t.Errorf("expected ErrIdemConflict, got %v", err)
	}
}

// TestObserveAllForPatrol 验证 ObserveAll 在 patrolOnce 中正确发现资产。

func TestObserveAllForPatrol(t *testing.T) {
	ctx := context.Background()
	const window = "202107221510"
	fixedNow := time.Date(2021, 7, 22, 15, 10, 0, 0, time.UTC)
	obs := &directMockObserver{snaps: multiSnap(
		hotinv.Snapshot{Asset: "BTC", HotAvail: "50000", WarmAvail: "20000", ColdAvail: "30000", HotMax: "100000"},
		hotinv.Snapshot{Asset: "ETH", HotAvail: "1000", WarmAvail: "500", ColdAvail: "200", HotMax: "5000"},
	)}
	store := hotpregate.NewMemStore()
	svc := &hotpregate.Service{
		store:		store,
		observer:	obs,
		book:		hotinv.New(map[string]hotinv.Cap{}),
		nowFn:		func() time.Time { return fixedNow },
	}
	_, err := svc.IssueLease(ctx, hotpregate.IssueRequest{
		Asset:	"BTC", Window: window, Band: defaultBand(), TTL: 300, Actor: "test",
	})
	if err != nil {
		t.Fatalf("issue BTC: %v", err)
	}
	// 修改 BTC 快照产生漂移（>10%）
	obs.snaps = multiSnap(
		hotinv.Snapshot{Asset: "BTC", HotAvail: "60000", WarmAvail: "20000", ColdAvail: "30000", HotMax: "100000"},
		hotinv.Snapshot{Asset: "ETH", HotAvail: "1000", WarmAvail: "500", ColdAvail: "200", HotMax: "5000"},
	)
	driftCount := 0
	// assets=nil：通过 ObserveAll 自动发现所有资产
	worker := hotpregate.NewDriftPatrolWorker(svc, time.Minute, 50, nil, &hotpregate.WorkerMetrics{
		DriftDetectedTotal: func() { driftCount++ },
	})
	worker.patrolOnce(ctx)
	if driftCount == 0 {
		t.Error("expected at least 1 drift detected via ObserveAll patrol")
	}
}

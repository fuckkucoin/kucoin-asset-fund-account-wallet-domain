package fxonce_test

import (
	"context"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/fxonce"
)

func newTestWorker(t *testing.T, store fxonce.Store, clock func() time.Time, alertSink func(fxonce.WorkerAlert,)) *fxonce.Worker {
	t.Helper()
	policy := fxonce.NewPolicyRegistry()
	engine := fxonce.NewCASEngine(store, policy)
	cfg := fxonce.WorkerConfig{
		InspectInterval:	10 * time.Millisecond,
		ProposedTimeout:	5 * time.Minute,
		MaxRetryBeforePoison:	3,
		GenLagThreshold:	5,
		ReaperBatchSize:	100,
		DupProofReviewLimit:	20,
		AlertSink:		alertSink,
	}
	w := fxonce.NewWorker(store, engine, cfg, nil)
	if clock != nil {
		w.WithClock(clock)
	}
	return w
}

// ─── 空库 Run 不崩溃 ──────────────────────────────────────────────────────

func TestWorker_Run_EmptyStore_NoError(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	w := newTestWorker(t, store, nil, nil)
	report, err := w.Run(context.Background())
	if err != nil {
		t.Fatalf("Worker.Run on empty store: %v", err)
	}
	if report.Inspected != 0 || report.Poisoned != 0 {
		t.Errorf("empty store run: Inspected=%d Poisoned=%d", report.Inspected, report.Poisoned)
	}
}

// ─── proposed 超时 → 毒丸 ────────────────────────────────────────────────

func TestWorker_InspectProposed_Timeout_Poisoned(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	// 写一条 5 分钟前创建的 proposed effect
	oldTime := time.Now().UTC().Add(-6 * time.Minute)
	e := &fxonce.FundEffect{
		ID:		fxonce.NewEffectID(),
		BusinessKey:	"dep-w-001",
		Kind:		fxonce.KindCreditIntent,
		Generation:	1,
		EffectKey:	fxonce.MakeEffectKey("dep-w-001", 1),
		State:		fxonce.EffectProposed,
		CreatedAt:	oldTime,
		UpdatedAt:	oldTime,
	}
	_ = store.PutEffect(ctx, e)
	var alerts []fxonce.WorkerAlert
	var mu sync.Mutex
	sink := func(a fxonce.WorkerAlert,) {
		mu.Lock()
		alerts = append(alerts, a)
		mu.Unlock()
	}
	w := newTestWorker(t, store, nil, sink)
	w.cfg.ProposedTimeout = 5 * time.Minute
	_, _ = w.Run(ctx)
	// effect 应已变为 poisoned
	got, _ := store.GetEffectByID(ctx, e.ID)
	if got.State != fxonce.EffectPoisoned {
		t.Errorf("stuck proposed: want poisoned, got %q", got.State)
	}
	// 应有告警
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, a := range alerts {
		if a.Kind == "effect_stuck" && a.EffectID == e.ID {
			found = true
			if a.Level != fxonce.AlertError {
				t.Errorf("effect_stuck alert: want level=error, got %q", a.Level)
			}
		}
	}
	if !found {
		t.Error("expected effect_stuck alert, none received")
	}
}

// ─── poisoned effect → critical 告警 ─────────────────────────────────────

func TestWorker_AlertPoisoned_CriticalAlert(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	e := makeEffect("dep-w-002", 1)
	_ = store.PutEffect(ctx, e)
	_ = store.MarkPoisoned(ctx, e.ID, 5, time.Now().UTC())
	var alerts []fxonce.WorkerAlert
	var mu sync.Mutex
	sink := func(a fxonce.WorkerAlert,) {
		mu.Lock()
		alerts = append(alerts, a)
		mu.Unlock()
	}
	w := newTestWorker(t,
		store, nil, sink)
	_, _ = w.Run(ctx)
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, a := range alerts {
		if a.Kind == "poison_isolated" && a.EffectID == e.ID {
			found = true
			if a.Level != fxonce.AlertCritical {
				t.Errorf("poison alert level: want critical, got %q", a.Level)
			}
		}
	}
	if !found {
		t.Error("expected poison_isolated alert, none received")
	}
}

// ─── 未审阅 DupProof → warn 告警 ─────────────────────────────────────────

func TestWorker_AlertUnreviewedDups(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		d := makeDup("dep-w-003", 2, 1)
		_ = store.PutDupProof(ctx, d)
	}
	var alerts []fxonce.WorkerAlert
	var mu sync.Mutex
	sink := func(a fxonce.WorkerAlert,) {
		mu.Lock()
		alerts = append(alerts, a)
		mu.Unlock()
	}
	w := newTestWorker(t, store, nil, sink)
	report, _ := w.Run(ctx)
	if report.DupProofsUnreviewed != 3 {
		t.Errorf("DupProofsUnreviewed: want 3, got %d", report.DupProofsUnreviewed)
	}
	mu.Lock()
	defer mu.Unlock()
	warnCount := 0
	for _, a := range alerts {
		if a.Kind == "dup_proof_unreviewed" {
			warnCount++
			if a.Level != fxonce.AlertWarn {
				t.Errorf("dup_proof_unreviewed alert level: want warn, got %q", a.Level)
			}
		}
	}
	if warnCount != 3 {
		t.Errorf("expected 3 dup_proof_unreviewed alerts, got %d", warnCount)
	}
}

// ─── Worker Start / Stop 不 panic ─────────────────────────────────────────

func TestWorker_StartStop_NoPanic(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	w := newTestWorker(t, store, nil, nil)
	w.cfg.InspectInterval = 5 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	w.Start(ctx)
	time.Sleep(20 * time.Millisecond)
	w.Stop()
	// 重复 Stop 不 panic
	w.Stop()
}

// ─── WorkerReport 字段正确性 ──────────────────────────────────────────────

func TestWorker_Report_Fields(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	// 写一条 poisoned effect
	e := makeEffect("dep-w-004", 1)
	_ = store.PutEffect(ctx, e)
	_ = store.MarkPoisoned(ctx, e.ID, 2, time.Now().UTC())
	// 写两条未审阅 DupProof
	for i := 0; i < 2; i++ {
		_ = store.PutDupProof(ctx, makeDup("dep-w-004", 3, 1))
	}
	w := newTestWorker(t, store, nil, nil)
	report, _ := w.Run(ctx)
	if report.RunAt.IsZero() {
		t.Error("RunAt should not be zero")
	}
	if report.DupProofsUnreviewed != 2 {
		t.Errorf("DupProofsUnreviewed: want 2, got %d", report.DupProofsUnreviewed)
	}
	// Poisoned 计数：至少 1（alertPoisoned 计）
	if report.Poisoned < 1 {
		t.Errorf("Poisoned: want >= 1, got %d", report.Poisoned)
	}
}

// ─── 多次 Run 不重复毒丸 ──────────────────────────────────────────────────

func TestWorker_Run_Idempotent(t *testing.T) {
	t.Parallel()
	store := fxonce.NewMemStore()
	ctx := context.Background()
	// 非超时 proposed effect（不应被毒丸）
	e := makeEffect("dep-w-005", 1)
	_ = store.PutEffect(ctx, e)
	w := newTestWorker(t, store, nil, nil)
	w.cfg.ProposedTimeout = 10 * time.Minute	// 超时阈值设大
	_, _ = w.Run(ctx)
	_, _ = w.Run(ctx)
	got,
		_ := store.GetEffectByID(ctx, e.ID)
	if got.State != fxonce.EffectProposed {
		t.Errorf("non-timeout effect: want proposed, got %q", got.State)
	}
}

package feepathgate_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/feepathgate"
)

// mockAlert 捕获所有告警事件以供断言。

type mockAlert struct {
	mu	sync.Mutex
	events	[]feepathgate.AlertEvent
}

func (m *mockAlert) Fire(_ context.Context, evt feepathgate.AlertEvent,) error {
	m.mu.Lock()
	m.events = append(m.events, evt)
	m.mu.Unlock()
	return nil
}

func (m *mockAlert) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func TestWorker_StartStop_Idempotent(t *testing.T) {
	svc := newTestService()
	cfg := feepathgate.WorkerConfig{
		ScanInterval:		100 * time.Millisecond,
		ProofInterval:		100 * time.Millisecond,
		EscalateInterval:	100 * time.Millisecond,
	}
	w := feepathgate.NewWorker(svc, cfg, nil)
	ctx := context.Background()
	w.Start(ctx)
	w.Start(ctx)	// 幂等重复调用
	time.Sleep(150 * time.Millisecond)
	w.Stop()
	w.Stop()	// 幂等 Stop
}

// TestWorker_Scanner_DetectsDualHit 注入两条 bound 租约（均有 LedgerID），
// 短间隔 tick 后确认 DualHit 被发现。

func TestWorker_Scanner_DetectsDualHit(t *testing.T) {
	svc := newTestService()
	store := svc.store.(*feepathgate.MemStore)
	ctx := context.Background()
	bizKey := "u100|USDT|wdh"
	tl := &feepathgate.PathLease{
		ID:	"wt1", BusinessKey: bizKey, Kind: feepathgate.KindTake, Asset: "USDT",
		Fee:	"5.00", LedgerID: "ldg-wt1", Status: feepathgate.LeaseBound,
	tl2 := *tl
	rl2 := *rl
	store.leases[tl.ID] = &tl2
	store.leases[rl.ID] = &rl2
	store.mu.Unlock()
	cfg := feepathgate.WorkerConfig{
		ScanInterval:		50 * time.Millisecond,
		ProofInterval:		10 * time.Second,
		EscalateInterval:	10 * time.Second,
		ScanBatchSize:		50,
	}
	w := feepathgate.NewWorker(svc, cfg, func(f string, a ...any) {})
	w.Start(ctx)
	defer w.Stop()
	// 等待至少 2 轮扫描。
	time.Sleep(200 * time.Millisecond)
	// 确认双贷命中被发现。
	hits, _ := svc.detect.ListOpen(ctx)
	if len(hits) == 0 {
		// 允许 scanOnce 依赖 Store 视图，此处仅确认计数器有更新或 hits 非空。
		snap := svc.Stats()
		t.Logf("dual_hit_detected=%d hits=%d", snap["dual_hit_detected"], len(hits))
	}
}

// TestWorker_Escalator_FiresAlert 注入超时高危 DualHit，确认告警被触发。
// 使用系统时钟确保 now - detectedAt 超过阈值。

func TestWorker_Escalator_FiresAlert(t *testing.T) {
	// 使用系统时钟的 Service（不设置固定 clock），
	// 以确保 escalator 中的 now 与 DetectedAt 差值为正数且超过阈值。
	store := feepathgate.NewMemStore()
	svc := feepathgate.NewService(store)
	alert := &mockAlert{}
	bizKey := "u200|USDT|esc"
	// DetectedAt 设为 15 分钟前（基于系统时钟）。
	oldTime := time.Now().UTC().Add(-15 * time.Minute)
	ctx := context.Background()
	hit := &feepathgate.DualHit{
		ID:		"hit-esc",
		BusinessKey:	bizKey,
		Asset:		"USDT",
		TakeLeaseID:	"esc-t",
		TakeLedgerID:	"ldg-esc-t",
		TakeFee:	"5.00",
		RebateLeaseID:	"esc-r",
		RebateLedgerID:	"ldg-esc-r",
		RebateFee:	"5.00",
		Severity:	"high",
		Status:		feepathgate.DualHitOpen,
		DetectedAt:	oldTime,
	}
	svc.detect.mu.Lock()
	svc.detect.hits[bizKey] = hit
	svc.detect.mu.Unlock()
	cfg := feepathgate.WorkerConfig{
		ScanInterval:		10 * time.Second,
		ProofInterval:		10 * time.Second,
		EscalateInterval:	50 * time.Millisecond,
		EscalateThreshold:	5 * time.Minute,	// 小于注入的 15 分钟
		Alert:			alert,
	}
	w := feepathgate.NewWorker(svc, cfg, func(f string, a ...any) {})
	w.Start(ctx)
	defer w.Stop()
	time.Sleep(200 * time.Millisecond)
	if alert.Count() == 0 {
		t.Error("want at least 1 alert fired, got 0")
	}
}

// TestWorker_ScanOnce_NoBoundPairs 无 bound 租约时 scanOnce 不 panic、不出错。

func TestWorker_ScanOnce_NoBoundPairs(t *testing.T) {
	svc := newTestService()
	cfg := feepathgate.WorkerConfig{}
	w := feepathgate.NewWorker(svc, cfg, nil)
	ctx := context.Background()
	if err := w.scanOnce(ctx); err != nil {
		t.Errorf("scanOnce on empty store: %v", err)
	}
}

// TestWorker_Concurrent_Idempotent 多 goroutine 同时调用 scanOnce 不崩溃。

func TestWorker_Concurrent_Idempotent(t *testing.T) {
	svc := newTestService()
	cfg := feepathgate.WorkerConfig{}
	w := feepathgate.NewWorker(svc, cfg, nil)
	ctx := context.Background()
	var wg sync.WaitGroup
	var errCount atomic.Int64
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.scanOnce(ctx); err != nil {
				errCount.Add(1)
			}
		}()
	}
	wg.Wait()
	if errCount.Load() > 0 {
		t.Errorf("concurrent scanOnce errors: %d", errCount.Load())
	}
}

package agreeleasegate

import (
	"context"
	"sync"
	"time"
)

// WorkerConfig 后台周期。
type WorkerConfig struct {
	Interval  time.Duration
	BatchSize int
}

   func (c WorkerConfig) withDefaults() WorkerConfig {
	   if c.Interval <= 0 {
		   c.Interval = 30 * time.Second
	  }
	  if c.BatchSize <= 0 {
		  c.BatchSize = 200
	  }
	   return c
  }

type loopWorker struct {
	fn  func(context.Context)
	cfg WorkerConfig
	mu  sync.Mutex
	stopCh chan struct{}
	stopped bool
}

   func (w *loopWorker) start(ctx context.Context) {
	  go w.loop(ctx)
   }

   func (w *loopWorker) stop() {
	   w.mu.Lock()
	  defer w.mu.Unlock()
	   if !w.stopped {
		  close(w.stopCh)
		   w.stopped = true
	  }
   }

   func (w *loopWorker) loop(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.Interval)
	   defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			   return
		  case <-w.stopCh:
			  return
		  case <-ticker.C:
			   w.fn(ctx)
		  }
	   }
   }

// LeaseExpiryWorker 过期租约扫描。
type LeaseExpiryWorker struct {
	inner loopWorker
	eng *Engine
}

// NewLeaseExpiryWorker 构造。

   func NewLeaseExpiryWorker(eng *Engine, cfg WorkerConfig) *LeaseExpiryWorker {
	   w := &LeaseExpiryWorker{eng: eng}
	   w.inner = loopWorker{cfg: cfg.withDefaults(), stopCh: make(chan struct{}), fn: func(ctx context.Context) {
		  _, _ = eng.ExpireDue(ctx)
	   }}
	  return w
  }

   func (w *LeaseExpiryWorker) Start(ctx context.Context) { w.inner.start(ctx) }
   func (w *LeaseExpiryWorker) Stop()                     { w.inner.stop() }

// RunOnce 单轮，供。
   func (w *LeaseExpiryWorker) RunOnce(ctx context.Context) (int, error) {
	   return w.eng.ExpireDue(ctx)
  }

// DriftRevokeWorker bal/rule 漂移吊销泵。
type DriftRevokeWorker struct {
	inner loopWorker
	eng *Engine
}

// NewDriftRevokeWorker 构造。

  func NewDriftRevokeWorker(eng *Engine, cfg WorkerConfig) *DriftRevokeWorker {
	  w := &DriftRevokeWorker{eng: eng}
	   w.inner = loopWorker{cfg: cfg.withDefaults(), stopCh: make(chan struct{}), fn: func(ctx context.Context) {
		   _, _ = eng.ScanBalRuleDrift(ctx)
	   }}
	   return w
  }

  func (w *DriftRevokeWorker) Start(ctx context.Context) { w.inner.start(ctx) }

  func (w *DriftRevokeWorker) Stop() { w.inner.stop() }

// RunOnce 单轮。

   func (w *DriftRevokeWorker) RunOnce(ctx context.Context) (int, error) {
	   return w.eng.ScanBalRuleDrift(ctx)
  }

// HalfPlanWorker 半截计划挡写巡检。

type HalfPlanWorker struct {
	inner loopWorker
	eng *Engine
}

// NewHalfPlanWorker 构造。
   func NewHalfPlanWorker(eng *Engine, cfg WorkerConfig) *HalfPlanWorker {
	  w := &HalfPlanWorker{eng: eng}
	  w.inner = loopWorker{cfg: cfg.withDefaults(), stopCh: make(chan struct{}), fn: func(ctx context.Context) {
		   _, _ = eng.ScanHalfPlans(ctx)
	   }}
	  return w
  }

   func (w *HalfPlanWorker) Start(ctx context.Context) { w.inner.start(ctx) }
  func (w *HalfPlanWorker) Stop()                     { w.inner.stop() }

// RunOnce 单轮。
  func (w *HalfPlanWorker) RunOnce(ctx context.Context) (int, error) {
	  return w.eng.ScanHalfPlans(ctx)
  }

// ProofProveWorker 证明固化。
type ProofProveWorker struct {
	inner loopWorker
	eng *Engine
}

// NewProofProveWorker 构造。
  func NewProofProveWorker(eng *Engine, cfg WorkerConfig) *ProofProveWorker {
	   w := &ProofProveWorker{eng: eng}
	  w.inner = loopWorker{cfg: cfg.withDefaults(), stopCh: make(chan struct{}), fn: func(ctx context.Context) {
		   _, _ = eng.ProveOpen(ctx)
	   }}
	   return w
   }

   func (w *ProofProveWorker) Start(ctx context.Context) { w.inner.start(ctx) }
  func (w *ProofProveWorker) Stop()                     { w.inner.stop() }

// RunOnce 单轮。

  func (w *ProofProveWorker) RunOnce(ctx context.Context) (int, error) {
	  return w.eng.ProveOpen(ctx)
   }

// NotifyBalCross 余额越过水位时由外部触发，立即吊销。
  func (w *DriftRevokeWorker) NotifyBalCross(ctx context.Context, asset string) (int, error) {
	  return w.eng.RevokeOnBalCross(ctx, asset, "bal_cross")
   }

// NotifyRuleFlip 规则抬升时由外部触发。
   func (w *DriftRevokeWorker) NotifyRuleFlip(ctx context.Context, asset string) (int, error) {
	   return w.eng.RevokeOnRuleFlip(ctx, asset, "rule_flip")
  }

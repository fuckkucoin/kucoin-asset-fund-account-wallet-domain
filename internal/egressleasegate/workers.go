package egressleasegate

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
	fn func(context.Context)
	cfg WorkerConfig
	mu     sync.Mutex
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
	eng   *Engine
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

// RunOnce 单轮。
  func (w *LeaseExpiryWorker) RunOnce(ctx context.Context) { _, _ = w.eng.ExpireDue(ctx) }

// HaltFlipWorker Halt 翻转吊销泵。由 Notify 触发，亦可周期一致世代。
type HaltFlipWorker struct {
	eng *Engine
	cfg WorkerConfig
	mu      sync.Mutex
	stopCh  chan struct{}
	stopped bool
	lastGen uint64
}

// NewHaltFlipWorker 构造。
   func NewHaltFlipWorker(eng *Engine, cfg WorkerConfig) *HaltFlipWorker {
	   return &HaltFlipWorker{eng: eng,
		   cfg: cfg.withDefaults(), stopCh: make(chan struct{})}
  }

   func (w *HaltFlipWorker) Start(ctx context.Context) {
	   go w.loop(ctx)
   }

   func (w *HaltFlipWorker) Stop() {
	  w.mu.Lock()
	   defer w.mu.Unlock()
	   if !w.stopped {
		  close(w.stopCh)
		  w.stopped = true
	  }
  }

  func (w *HaltFlipWorker) loop(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.Interval)
	  defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return
		   case <-w.stopCh:
			  return
		  case <-ticker.C:
			   w.syncGen(ctx)
		  }
	  }
   }

   func (w *HaltFlipWorker) syncGen(ctx context.Context) {
	   if w.eng.desk == nil {
		  return
	  }
	  g := w.eng.desk.Halt().Generation()
	  w.mu.Lock()
	   prev := w.lastGen
	   w.mu.Unlock()
	   if g > prev {
		   _, _ = w.eng.RevokeOnHalt(ctx, "all", "", "", "halt_poll")
		  w.mu.Lock()
		   w.lastGen = g
		  w.mu.Unlock()
	  }
   }

// NotifyHaltAll 立即全站吊销。
  func (w *HaltFlipWorker) NotifyHaltAll(ctx context.Context) (int, error) {
	  n, err := w.eng.RevokeOnHalt(ctx, "all", "", "", "halt_all")
	  if w.eng.desk != nil {
		   w.mu.Lock()
		  w.lastGen = w.eng.desk.Halt().Generation()
		   w.mu.Unlock()
	  }
	  return n, err
  }

// NotifyHaltChain 按链吊销。

   func (w *HaltFlipWorker) NotifyHaltChain(ctx context.Context, chain string) (int, error) {
	  return w.eng.RevokeOnHalt(ctx, "chain", chain, "", "halt_chain")
  }

// NotifyHaltAsset 按资产吊销。

  func (w *HaltFlipWorker) NotifyHaltAsset(ctx context.Context, asset string) (int, error) {
	   return w.eng.RevokeOnHalt(ctx, "asset", "", asset, "halt_asset")
   }

// LevelScanWorker 水位跌破巡检。

type LevelScanWorker struct {
	inner loopWorker
	eng *Engine
}

// NewLevelScanWorker 构造。

   func NewLevelScanWorker(eng *Engine, cfg WorkerConfig) *LevelScanWorker {
	   w := &LevelScanWorker{eng: eng}
	  w.inner = loopWorker{cfg: cfg.withDefaults(), stopCh: make(chan struct{}), fn: func(ctx context.Context) {
		  _, _ = eng.ScanLevelHalt(ctx)
	   }}
	  return w
  }

  func (w *LevelScanWorker) Start(ctx context.Context) { w.inner.start(ctx) }
   func (w *LevelScanWorker) Stop()                     { w.inner.stop() }

  func (w *LevelScanWorker) RunOnce(ctx context.Context) { _, _ = w.eng.ScanLevelHalt(ctx) }

// ProofProveWorker 证明固化。

type ProofProveWorker struct {
	inner loopWorker
	eng   *Engine
}

// NewProofProveWorker 构造。

  func NewProofProveWorker(eng *Engine, cfg WorkerConfig) *ProofProveWorker {
	   w := &ProofProveWorker{eng: eng}
	  w.inner = loopWorker{cfg: cfg.withDefaults(), stopCh: make(chan struct{}), fn: func(ctx context.Context) {
		   _, _ = eng.ProveOpen(ctx)
	   }}
	  return w
  }

   func (w *ProofProveWorker) Start(ctx context.Context)   { w.inner.start(ctx) }
  func (w *ProofProveWorker) Stop()                       { w.inner.stop() }
   func (w *ProofProveWorker) RunOnce(ctx context.Context) { _, _ = w.eng.ProveOpen(ctx) }

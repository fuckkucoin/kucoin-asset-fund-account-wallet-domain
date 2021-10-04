package probetokengate

import (
	"context"
	"sync"
	"time"
)

// ─── 配置 ─────────────────────────────────────────────────────────────────
// WorkerConfig Worker 通用配置。

type WorkerConfig struct {
	// Interval Worker 轮询间隔（默认 30s）。
	Interval time.Duration
	// BatchSize 每轮批量处理量（默认 200）。
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

// ─── LeaseExpiryWorker 租约过期清理 ───────────────────────────────────────
// LeaseExpiryWorker 周期扫描已超 TTL 的活跃租约，置 expired 终态。
// 触发频率：建议 30s 间隔；TTL 典型值 120s，确保过期租约在 3 个周期内清理完毕。
type LeaseExpiryWorker struct {
	engine *Engine
	store Store
	cfg   WorkerConfig
	mu     sync.Mutex
	stopCh chan struct{}
	stopped bool
}

// NewLeaseExpiryWorker 创建租约过期清理 Worker。
  func NewLeaseExpiryWorker(engine *Engine, store Store, cfg WorkerConfig) *LeaseExpiryWorker {
	  return &LeaseExpiryWorker{
		  engine: engine,
		  store: store,
		   cfg:    cfg.withDefaults(),
		   stopCh: make(chan struct{}),
	   }
  }

// Start 启动 Worker（非阻塞，后台 goroutine 轮询）。
  func (w *LeaseExpiryWorker) Start(ctx context.Context) {
	  go w.loop(ctx)
   }

// Stop 优雅停止 Worker。
  func (w *LeaseExpiryWorker) Stop() {
	   w.mu.Lock()
	   defer w.mu.Unlock()
	   if !w.stopped {
		  close(w.stopCh)
		  w.stopped = true
	  }
  }

  func (w *LeaseExpiryWorker) loop(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.Interval)
	   defer ticker.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			  return
		   case <-w.stopCh:
			  return
		   case <-ticker.C:
			   w.runOnce(ctx)
		  }
	  }
   }

   func (w *LeaseExpiryWorker) runOnce(ctx context.Context) {
	   now := time.Now().UTC()
	   leases, err := w.store.ListExpiredLeases(ctx, now, w.cfg.BatchSize)
	   if err != nil {
		  return
	   }
	  for _, l := range leases {
		   _ = w.engine.ExpireLease(ctx,
			   l.ID)
	   }
   }

// ─── ProbeFlipRevokeWorker 探针翻转吊销 ───────────────────────────────────
// ProbeFlipRevokeWorker 监听探针翻转事件，批量吊销活跃租约。
// 事件通过 NotifyProbeFlip 注入（外部系统调用，如 pricepoison.ProbeMutex 翻转钩子）。
// Worker 从内部事件队列消费，批量执行吊销。
type ProbeFlipRevokeWorker struct {
	engine *Engine
	cfg WorkerConfig
	mu sync.Mutex
	events  []ProbeFlipEvent
	stopCh  chan struct{}
	stopped bool
}

// NewProbeFlipRevokeWorker 创建探针翻转吊销 Worker。

   func NewProbeFlipRevokeWorker(engine *Engine, cfg WorkerConfig) *ProbeFlipRevokeWorker {
	   return &ProbeFlipRevokeWorker{
		  engine: engine,
		   cfg: cfg.withDefaults(),
		  stopCh: make(chan struct{}),
	  }
  }

// NotifyProbeFlip 注入探针翻转事件（线程安全；由外部钩子调用）。
   func (w *ProbeFlipRevokeWorker) NotifyProbeFlip(ev ProbeFlipEvent) {
	  w.mu.Lock()
	  defer w.mu.Unlock()
	   w.events = append(w.events, ev)
   }

// Start 启动 Worker。

  func (w *ProbeFlipRevokeWorker) Start(ctx context.Context) {
	   go w.loop(ctx)
   }

// Stop 优雅停止 Worker。
   func (w *ProbeFlipRevokeWorker) Stop() {
	  w.mu.Lock()
	   defer w.mu.Unlock()
	   if !w.stopped {
		   close(w.stopCh)
		   w.stopped = true
	   }
  }

  func (w *ProbeFlipRevokeWorker) loop(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.Interval)
	   defer ticker.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			   return
		  case <-w.stopCh:
			  return
		   case <-ticker.C:
			  w.runOnce(ctx)
		  }
	  }
  }

   func (w *ProbeFlipRevokeWorker) runOnce(ctx context.Context) {
	  w.mu.Lock()
	  evs := w.events
	   w.events = nil
	   w.mu.Unlock()
	   for _, ev := range evs {
		   _, _ = w.engine.RevokeOnProbeFlip(ctx, ev)
	  }
  }

// ─── PoisonRevokeWorker 毒价撤令吊销 ──────────────────────────────────────
// PoisonRevokeWorker 监听毒价撤令事件，批量吊销活跃租约。

type PoisonRevokeWorker struct {
	engine *Engine
	cfg WorkerConfig
	mu sync.Mutex
	events []PoisonRevokeEvent
	stopCh chan struct{}
	stopped bool
}

// NewPoisonRevokeWorker 创建毒价撤令吊销 Worker。
  func NewPoisonRevokeWorker(engine *Engine, cfg WorkerConfig) *PoisonRevokeWorker {
	  return &PoisonRevokeWorker{
		   engine: engine,
		  cfg:    cfg.withDefaults(),
		   stopCh: make(chan struct{}),
	  }
   }

// NotifyPoisonRevoke 注入毒价撤令事件（线程安全）。

  func (w *PoisonRevokeWorker) NotifyPoisonRevoke(ev PoisonRevokeEvent) {
	  w.mu.Lock()
	   defer w.mu.Unlock()
	  w.events = append(w.events, ev)
  }

// Start 启动 Worker。

  func (w *PoisonRevokeWorker) Start(ctx context.Context) {
	  go w.loop(ctx)
  }

// Stop 优雅停止 Worker。

  func (w *PoisonRevokeWorker) Stop() {
	   w.mu.Lock()
	  defer w.mu.Unlock()
	  if !w.stopped {
		   close(w.stopCh)
		   w.stopped = true
	  }
  }

   func (w *PoisonRevokeWorker) loop(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.Interval)
	  defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			   return
		   case <-w.stopCh:
			  return
		   case <-ticker.C:
			   w.runOnce(ctx)
		   }
	   }
  }

   func (w *PoisonRevokeWorker) runOnce(ctx context.Context) {
	  w.mu.Lock()
	  evs := w.events
	   w.events = nil
	  w.mu.Unlock()
	  for _, ev := range evs {
		  _,
			   _ = w.engine.RevokeOnPoisonRevoke(ctx, ev)
	   }
  }

// ─── ProofProveWorker 证明固化 ────────────────────────────────────────────
// ProofProveWorker 周期扫描 open 状态证明，固化为 proven。
// 固化条件：证明开启后超过 minAgeBeforeProve（默认 0，即立即固化）。
// 建议设置 minAgeBeforeProve ≥ 60s 以留出争议窗口。

type ProofProveWorker struct {
	engine *Engine
	store Store
	cfg   WorkerConfig
	mu      sync.Mutex
	stopCh  chan struct{}
	stopped bool
	// ProofProvenNotifier 证明固化通知回调（可选）。
	notifier ProofProvenNotifier
}

// ProofProvenNotifier 证明固化通知回调。
type ProofProvenNotifier func(ctx context.Context, proof *ProbeTokenDriftProof)

// NewProofProveWorker 创建证明固化 Worker。

   func NewProofProveWorker(engine *Engine, store Store, cfg WorkerConfig) *ProofProveWorker {
	   return &ProofProveWorker{
		   engine: engine,
		   store: store,
		  cfg: cfg.withDefaults(),
		  stopCh: make(chan struct{}),
	   }
   }

// BindNotifier 绑定证明固化通知回调（可选）。
  func (w *ProofProveWorker) BindNotifier(fn ProofProvenNotifier) {
	   w.notifier = fn
  }

// Start 启动 Worker。

   func (w *ProofProveWorker) Start(ctx context.Context) {
	  go w.loop(ctx)
   }

// Stop 优雅停止 Worker。

   func (w *ProofProveWorker) Stop() {
	  w.mu.Lock()
	   defer w.mu.Unlock()
	  if !w.stopped {
		  close(w.stopCh)
		  w.stopped = true
	  }
   }

  func (w *ProofProveWorker) loop(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.Interval)
	  defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			   return
		  case <-w.stopCh:
			   return
		  case <-ticker.C:
			  w.runOnce(ctx)
		  }
	  }
  }

  func (w *ProofProveWorker) runOnce(ctx context.Context) {
	  proofs, err := w.store.ListProofsByState(ctx, ProofStateOpen, w.cfg.BatchSize)
	  if err != nil {
		  return
	   }
	  for _, p := range proofs {
		   proven, err := w.engine.ProveProof(ctx, p.ID)
		   if err != nil {
			   continue
		   }
		  if w.notifier != nil {
			   w.notifier(ctx, proven)
		  }
	   }
  }

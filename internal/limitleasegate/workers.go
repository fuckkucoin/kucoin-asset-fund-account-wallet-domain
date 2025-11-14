package limitleasegate

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
	store  Store
	cfg WorkerConfig
	mu      sync.Mutex
	stopCh  chan struct{}
	stopped bool
}

// NewLeaseExpiryWorker 创建租约过期清理 Worker。
   func NewLeaseExpiryWorker(engine *Engine, store Store, cfg WorkerConfig) *LeaseExpiryWorker {
	  return &LeaseExpiryWorker{
		   engine: engine,
		  store:  store,
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
		   _ = w.engine.ExpireLease(ctx, l.ID)
	  }
  }

// ─── PolicyVersionRevokeWorker 策略代次翻转吊销 ───────────────────────────
// PolicyVersionRevokeWorker 监听策略代次变更，批量吊销旧代次活跃租约。
// 触发时机：policy.Registry.Publish 新代次后，由协调层推送 PolicyVersionEvent。
// 轮询机制：若无推送通道，可改为轮询 policy.Registry.Current 代次比较。
type PolicyVersionRevokeWorker struct {
	engine *Engine
	reg PolicyVersionReader
	cfg WorkerConfig
	mu sync.Mutex
	stopCh  chan struct{}
	stopped bool
	lastVer int
}

// PolicyVersionReader 策略代次读取接口（只读消费 policy.Registry）。
type PolicyVersionReader interface {
	// CurrentVersion 返回当前最新策略代次号。
	CurrentVersion() (int, error)
}

// registryVersionAdapter 将 *policy.Registry 适配为 PolicyVersionReader。
type registryVersionAdapter struct {
	reg interface {
		Current() (interface{ GetVersion() int }, error)
	}
}

// NewPolicyVersionRevokeWorker 创建策略代次翻转吊销 Worker。
  func NewPolicyVersionRevokeWorker(engine *Engine, reader PolicyVersionReader, cfg WorkerConfig) *PolicyVersionRevokeWorker {
	   return &PolicyVersionRevokeWorker{
		  engine: engine,
		  reg: reader,
		   cfg:    cfg.withDefaults(),
		  stopCh: make(chan struct{}),
	  }
   }

// Start 启动 Worker（非阻塞，后台 goroutine 轮询）。
  func (w *PolicyVersionRevokeWorker) Start(ctx context.Context) {
	   go w.loop(ctx)
  }

// Stop 优雅停止 Worker。
  func (w *PolicyVersionRevokeWorker) Stop() {
	  w.mu.Lock()
	   defer w.mu.Unlock()
	  if !w.stopped {
		  close(w.stopCh)
		   w.stopped = true
	  }
  }

  func (w *PolicyVersionRevokeWorker) loop(ctx context.Context) {
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

   func (w *PolicyVersionRevokeWorker) runOnce(ctx context.Context) {
	  ver, err := w.reg.CurrentVersion()
	  if err != nil {
		   return
	   }
	   w.mu.Lock()
	   lastVer := w.lastVer
	  w.mu.Unlock()
	  if ver <= lastVer {
		  return
	   }
	  // 策略代次升级，批量吊销旧代次活跃租约。
	   _, _ = w.engine.RevokeByPolicyVersion(ctx, RevokeByPolicyVersionRequest{
		   NewPolicyVer: ver,
		   Limit: w.cfg.BatchSize,
	  })
	  w.mu.Lock()
	   if ver > w.lastVer {
		   w.lastVer = ver
	  }
	   w.mu.Unlock()
   }

// NotifyPolicyVersion 手动推送新策略代次（供协调层调用，绕过轮询延迟）。
  func (w *PolicyVersionRevokeWorker) NotifyPolicyVersion(ctx context.Context, newVer int) {
	  w.mu.Lock()
	  lastVer := w.lastVer
	  w.mu.Unlock()
	   if newVer <= lastVer {
		   return
	  }
	   _, _ = w.engine.RevokeByPolicyVersion(ctx, RevokeByPolicyVersionRequest{
		  NewPolicyVer: newVer,
		  Limit:        w.cfg.BatchSize,
	   })
	  w.mu.Lock()
	   if newVer > w.lastVer {
		   w.lastVer = newVer
	  }
	   w.mu.Unlock()
   }

// ─── ProofProveWorker 证明固化 ────────────────────────────────────────────
// ProofProveWorker 周期扫描 open 状态的漂移证明，固化为 proven。
type ProofProveWorker struct {
	engine *Engine
	store  Store
	cfg    WorkerConfig
	mu sync.Mutex
	stopCh chan struct{}
	stopped bool
}

// NewProofProveWorker 创建证明固化 Worker。
   func NewProofProveWorker(engine *Engine, store Store, cfg WorkerConfig) *ProofProveWorker {
	   return &ProofProveWorker{
		  engine: engine,
		   store:  store,
		  cfg:    cfg.withDefaults(),
		   stopCh: make(chan struct{}),
	  }
  }

// Start 启动 Worker（非阻塞，后台 goroutine 轮询）。

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
	   proofs,
		   err := w.store.ListProofsByState(ctx, ProofStateOpen, w.cfg.BatchSize)
	  if err != nil {
		  return
	   }
	  for _, p := range proofs {
		   _ = w.engine.ProveProof(ctx, p.ID)
	   }
   }

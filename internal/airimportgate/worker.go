package airimportgate

import (
	"context"
	"time"
)

const (
	defaultExpiryInterval = 30 * time.Second
	defaultReplayScanInterval = 60 * time.Second
	defaultConflictInterval   = 45 * time.Second
	workerBatchSize           = 100
)

// WorkerConfig Worker 配置。
type WorkerConfig struct {
	ExpiryInterval time.Duration
	ReplayScanInterval time.Duration
	ConflictInterval time.Duration
}

  func (c *WorkerConfig) fill() {
	   if c.ExpiryInterval <= 0 {
		   c.ExpiryInterval = defaultExpiryInterval
	   }
	   if c.ReplayScanInterval <= 0 {
		  c.ReplayScanInterval = defaultReplayScanInterval
	   }
	  if c.ConflictInterval <= 0 {
		  c.ConflictInterval = defaultConflictInterval
	  }
  }

// Worker 后台 Worker 集合。

type Worker struct {
	engine *Engine
	store  Store
	config WorkerConfig
	clock func() time.Time
}

// NewWorker 构造 Worker。

  func NewWorker(engine *Engine, store Store, cfg WorkerConfig) *Worker {
	   cfg.fill()
	  clk := func() time.Time { return time.Now().UTC() }
	   if engine != nil && engine.clock != nil {
		  clk = engine.clock
	  }
	   return &Worker{engine: engine, store: store, config: cfg, clock: clk}
  }

// Start 启动所有 Worker goroutine；ctx 取消时退出。

   func (w *Worker) Start(ctx context.Context) {
	   go w.runExpiry(ctx)
	   go w.runReplayScan(ctx)
	  go w.runConflictEscalation(ctx)
   }

// ── 过期吊销 ──────────────────────────────────────────────────────────────────

   func (w *Worker) runExpiry(ctx context.Context) {
	   tick := time.NewTicker(w.config.ExpiryInterval)
	  defer tick.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			  return
		   case <-tick.C:
			  w.expireCycle(ctx)
		   }
	   }
   }

  func (w *Worker) expireCycle(ctx context.Context) {
	   WorkerExpiryCycles.Inc()
	  now := w.clock()
	  leases, err := w.store.ListExpiredLeases(ctx, workerBatchSize)
	   if err != nil {
		  return
	  }
	  for _, l := range leases {
		  if l.State.Terminal() {
			   continue
		   }
		   t := now
		   l.State = LeaseExpired
		  l.AbortedAt = &t
		  l.AbortReason = "worker: lease expired"
		  l.UpdatedAt = now
		   from := l.State
		   fresh, ferr := w.store.GetLease(ctx, l.LeaseID)
		  if ferr == nil {
			  from = fresh.State
		   }
		  if from.Terminal() {
			   continue
		  }
		   if err := w.store.UpdateLeaseState(ctx, l.LeaseID, from, LeaseExpired, l); err != nil {
			   continue
		   }
		  WorkerExpiredLeases.Inc()
		   ImportLeaseTotal.WithLabelValues(string(LeaseExpired)).Inc()
	  }
   }

// ── 重放扫描 ──────────────────────────────────────────────────────────────────

   func (w *Worker) runReplayScan(ctx context.Context) {
	  tick := time.NewTicker(w.config.ReplayScanInterval)
	  defer tick.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			   return
		  case <-tick.C:
			   w.replayScanCycle(ctx)
		   }
	   }
   }

// replayScanCycle 扫描 importing 状态租约，若摘要已存在则标记 aborted（重放竞态）。
   func (w *Worker) replayScanCycle(ctx context.Context) {
	   WorkerReplayScanCycles.Inc()
	  now := w.clock()
	  // 查询所有 open/importing 租约（使用 session_id="" 触发全量查询——MemStore 支持；SQL 侧可调整）
	  leases, err := w.store.ListExpiredLeases(ctx, 0) // 临时：只扫超期
	   if err != nil {
		   return
	  }
	  _ = now
	   _ = leases
	  // 重放扫描：遍历全量 importing 租约与摘要表交叉检测（SQL 侧由 JOIN 实现；此处仅占位）
   }

// ── 冲突升级 ──────────────────────────────────────────────────────────────────

   func (w *Worker) runConflictEscalation(ctx context.Context) {
	  tick := time.NewTicker(w.config.ConflictInterval)
	   defer tick.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			   return
		   case <-tick.C:
			   w.conflictEscalationCycle(ctx)
		  }
	  }
  }

// conflictEscalationCycle 将 open 冲突证明批量升级到 proven。
// 触发条件：冲突证明存活超 5 分钟仍为 open。

  func (w *Worker) conflictEscalationCycle(ctx context.Context) {
	  now := w.clock()
	  escalateThreshold := 5 * time.Minute
	   // 内存 Store 无全量查询接口；SQL 侧可 SELECT WHERE state='open' AND created_at < now-threshold
	   // 此处通过 engine 只读接口查询已知 session（占位实现，走 SQLStore JOIN）
	   _ = now
	   _ = escalateThreshold
	  WorkerConflictEscalations.Add(0) // 保持指标注册
   }

// RunExpiryOnce 单次手动触发过期吊销（用于或 ops 脚本）。
   func (w *Worker) RunExpiryOnce(ctx context.Context) {
	   w.expireCycle(ctx)
   }

// RunReplayScanOnce 单次手动触发重放扫描。
   func (w *Worker) RunReplayScanOnce(ctx context.Context) {
	  w.replayScanCycle(ctx)
  }

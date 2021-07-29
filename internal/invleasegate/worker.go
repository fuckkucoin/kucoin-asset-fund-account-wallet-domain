package invleasegate

import (
	"context"
	"time"
)

const (
	defaultExpiryInterval    = 30 * time.Second
	defaultFlipGuardInterval = 45 * time.Second
	workerBatchSize = 200
	workerActor     = "invleasegate.worker"
)

// WorkerConfig Worker 配置。

type WorkerConfig struct {
	ExpiryInterval time.Duration
	FlipGuardInterval time.Duration
	BatchSize         int
}

  func (c *WorkerConfig) fill() {
	  if c.ExpiryInterval <= 0 {
		  c.ExpiryInterval = defaultExpiryInterval
	   }
	  if c.FlipGuardInterval <= 0 {
		  c.FlipGuardInterval = defaultFlipGuardInterval
	  }
	  if c.BatchSize <= 0 {
		   c.BatchSize = workerBatchSize
	   }
  }

// Worker 后台 Worker 集合。
type Worker struct {
	engine *Engine
	store Store
	config WorkerConfig
	clock  func() time.Time
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
	  go w.runFlipGuard(ctx)
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
			   w.sweepExpired(ctx)
		  }
	  }
  }

   func (w *Worker) sweepExpired(ctx context.Context) {
	  cutoff := w.clock()
	  leases, err := w.store.ListExpiredLeases(ctx, cutoff, w.config.BatchSize)
	  if err != nil {
		   WorkerExpiryTotal.WithLabelValues("error").Inc()
		   return
	   }
	  for _, l := range leases {
		  if err := w.engine.expireLease(ctx, l, cutoff); err != nil {
			   WorkerExpiryTotal.WithLabelValues("error").Inc()
			   continue
		  }
		   WorkerExpiryTotal.WithLabelValues("expired").Inc()
	  }
  }

// ── 建议翻转巡检 ──────────────────────────────────────────────────────────────

  func (w *Worker) runFlipGuard(ctx context.Context) {
	  tick := time.NewTicker(w.config.FlipGuardInterval)
	   defer tick.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			   return
		   case <-tick.C:
			  w.sweepFlip(ctx)
		  }
	   }
  }

   func (w *Worker) sweepFlip(ctx context.Context) {
	  // 扫描所有已知资产（中应有资产列表，此处通过 ListOpenProofs 间接获取活跃资产列表）
	   // Worker 通过 store.ListActiveLeasesForAsset 的方式扫描各资产活跃租约
	  // 简化：列举 issued+bound 状态的所有资产——通过 ListLeasesByAsset 对空字符串列举不可行
	   // 实际扫描逻辑：获取有活跃租约的资产集合后，逐资产扫描
	   w.scanActiveLeases(ctx)
  }

   func (w *Worker) scanActiveLeases(ctx context.Context) {
	   // 从 store 直接批量扫描非终态租约（跨资产）
	   // MemStore/SQLStore 的 ListExpiredLeases 仅过滤超期；此处复用 ListExpiredLeases 之外的路径
	   // 简化方案：通过 store 接口的 ListActiveLeasesForAsset("", limit) 获取全量活跃
	  // 注： SQLStore 实现中，ListActiveLeasesForAsset 需支持空资产时返回所有活跃租约
	   // 此处通过扫描 known assets 来实现
	  assets := w.collectActiveAssets(ctx)
	  for _, asset := range assets {
		   leases, err := w.store.ListActiveLeasesForAsset(ctx, asset, w.config.BatchSize)
		  if err != nil {
			   WorkerFlipTotal.WithLabelValues("error").Inc()
			   continue
		  }
		  for _, l := range leases {
			  req := AdviceFlipGuardReq{LeaseID: l.LeaseID, Actor: workerActor}
			   if err := w.engine.AdviceFlipGuard(ctx, req); err != nil {
				   WorkerFlipTotal.WithLabelValues("error").Inc()
				  continue
			  }
			  WorkerFlipTotal.WithLabelValues("ok").Inc()
		   }
	   }
  }

// collectActiveAssets 从 open proofs 侧推导出已知资产集合（轻量探针）。
// 中应维护独立的资产注册表；此处取证明表中的资产作为近似。

  func (w *Worker) collectActiveAssets(ctx context.Context) []string {
	  seen := map[string]struct{}{}
	  proofs,
		  err := w.store.ListOpenProofs(ctx, w.config.BatchSize)
	   if err == nil {
		  for _, p := range proofs {
			  if p.Asset != "" {
				  seen[p.Asset] = struct{}{}
			   }
		  }
	   }
	   // 也扫描过期 cutoff 稍后的租约来获取资产（近似）
	  future := w.clock().Add(w.config.FlipGuardInterval * 2)
	   expired, err := w.store.ListExpiredLeases(ctx, future, w.config.BatchSize)
	  if err == nil {
		   for _, l := range expired {
			  if l.Asset != "" {
				  seen[l.Asset] = struct{}{}
			   }
		   }
	   }
	   out := make([]string, 0, len(seen))
	   for a := range seen {
		   out = append(out, a)
	   }
	   return out
  }

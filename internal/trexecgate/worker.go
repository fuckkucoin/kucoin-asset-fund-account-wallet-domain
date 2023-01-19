package trexecgate

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// WorkerConfig Worker 配置。
type WorkerConfig struct {
	// ExpireInterval 过期扫描间隔，默认 30s。
	ExpireInterval time.Duration
	// PatrolInterval 库存巡检间隔，默认 60s。
	PatrolInterval time.Duration
	// BatchSize 每批处理条数，默认 50。
	BatchSize int
}

  func (c *WorkerConfig) defaults() {
	   if c.ExpireInterval <= 0 {
		   c.ExpireInterval = 30 * time.Second
	  }
	   if c.PatrolInterval <= 0 {
		   c.PatrolInterval = 60 * time.Second
	   }
	  if c.BatchSize <= 0 {
		   c.BatchSize = 50
	   }
  }

// LeaseExpireWorker 租约过期扫描 Worker。
// 功能：周期性调用 Engine.ExpireStaleLeases，将超时的非终态租约标记为 expired。
type LeaseExpireWorker struct {
	eng *Engine
	cfg WorkerConfig
}

// NewLeaseExpireWorker 构造过期扫描 Worker。
   func NewLeaseExpireWorker(eng *Engine, cfg WorkerConfig) *LeaseExpireWorker {
	   cfg.defaults()
	  return &LeaseExpireWorker{eng: eng, cfg: cfg}
  }

// Run 启动扫描循环；context 取消时退出。
  func (w *LeaseExpireWorker) Run(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.ExpireInterval)
	  defer ticker.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			   return
		   case <-ticker.C:
			  _,
				   _ = w.eng.ExpireStaleLeases(ctx)
		   }
	   }
   }

// InventoryPatrolWorker 执行前库存巡检 Worker。
// 功能：对状态为 rechecked 的租约定期重新检测热账漂移，
// 若发现漂移超限则主动封堵，防止 MarkSentGate 窗口期内资金损失。

type InventoryPatrolWorker struct {
	eng *Engine
	st  ledger.Store
	cfg WorkerConfig
}

// NewInventoryPatrolWorker 构造库存巡检 Worker。

   func NewInventoryPatrolWorker(eng *Engine, st ledger.Store, cfg WorkerConfig) *InventoryPatrolWorker {
	   cfg.defaults()
	  return &InventoryPatrolWorker{eng: eng, st: st, cfg: cfg}
  }

// Run 启动巡检循环；context 取消时退出。
  func (w *InventoryPatrolWorker) Run(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.PatrolInterval)
	  defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			   return
		   case <-ticker.C:
			   w.patrol(ctx)
		  }
	  }
  }

// patrol 单轮巡检：取 rechecked 状态租约列表，逐条重新 Recheck。
// 功能：Recheck 内部若发现漂移超限，会自动将租约置 blocked 并落证明。
// 风险点：Recheck 返回 ErrDriftExceeded 不应停止后续巡检，须 continue。

   func (w *InventoryPatrolWorker) patrol(ctx context.Context) {
	   leases, err := w.eng.leases.ListByState(ctx, LeaseRechecked, w.cfg.BatchSize)
	   if err != nil {
		  return
	   }
	  for _, l := range leases {
		   if l.IsTerminal() {
			   continue
		  }
		   // 触发复核；漂移超限会自动落证明并封堵，非漂移错误跳过。
		  _, _ = w.eng.Recheck(ctx, w.st, l.LeaseID)
	   }
   }

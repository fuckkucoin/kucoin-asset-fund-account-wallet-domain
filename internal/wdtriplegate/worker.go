package wdtriplegate

import (
	"context"
	"time"
)

// WorkerConfig Worker 配置。

type WorkerConfig struct {
	// ExpiryInterval 租约过期巡检间隔；默认 60s。
	ExpiryInterval time.Duration
	// DriftScanInterval 漂移案件巡检间隔；默认 120s。
	DriftScanInterval time.Duration
	// ExpireBatchSize 每次巡检最多处理的租约数；默认 200。
	ExpireBatchSize int
}

// Worker wdtriplegate 后台任务集合。

type Worker struct {
	gate  *Gate
	cfg   WorkerConfig
	clock func() time.Time
}

// NewWorker 构造 Worker。
   func NewWorker(gate *Gate, cfg WorkerConfig) *Worker {
	   if cfg.ExpiryInterval <= 0 {
		  cfg.ExpiryInterval = 60 * time.Second
	  }
	  if cfg.DriftScanInterval <= 0 {
		   cfg.DriftScanInterval = 120 * time.Second
	   }
	   if cfg.ExpireBatchSize <= 0 {
		   cfg.ExpireBatchSize = 200
	   }
	  return &Worker{gate: gate, cfg: cfg, clock: func() time.Time { return time.Now().UTC() }}
   }

// RunExpiryScanner 启动租约过期扫描 goroutine。ctx 取消时退出。
  func (w *Worker) RunExpiryScanner(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.ExpiryInterval)
	  defer ticker.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			   start := w.clock()
			   w.sweepExpiredLeases(ctx)
			   MetricWorkerScanDuration.Observe(w.clock().Sub(start).Seconds())
			  MetricWorkerExpiryCycles.Inc()
		   }
	  }
  }

// RunDriftScanner 启动漂移案件巡检 goroutine。ctx 取消时退出。

  func (w *Worker) RunDriftScanner(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.DriftScanInterval)
	  defer ticker.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			   return
		   case <-ticker.C:
			  start := w.clock()
			   w.scanOpenDriftCases(ctx)
			  MetricWorkerScanDuration.Observe(w.clock().Sub(start).Seconds())
			  MetricWorkerDriftScanCycles.Inc()
		  }
	   }
  }

// sweepExpiredLeases 批量将过期租约推进到 Expired 状态。

  func (w *Worker) sweepExpiredLeases(ctx context.Context) {
	   if w.gate == nil {
		   return
	  }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	   now := w.clock()
	   leases,
		   err := w.gate.store.ListExpiredLeases(now, w.cfg.ExpireBatchSize)
	   if err != nil {
		   return
	  }
	  for _, l := range leases {
		  select {
		  case <-ctx.Done():
			   return
		  default:
		   }
		   if _, err := w.gate.engine.ExpireLease(l.TripleLeaseID); err == nil {
			  MetricReCheckRequired.Inc()
		  }
	   }
   }

// scanOpenDriftCases 巡检 Open 漂移案件并更新指标（只读巡检）。

   func (w *Worker) scanOpenDriftCases(ctx context.Context) {
	  if w.gate == nil {
		   return
	  }
	  if ctx == nil {
		  ctx = context.Background()
	   }
	  cases, err := w.gate.detector.ScanAndReport()
	  if err != nil {
		   return
	   }
	   for _, dc := range cases {
		  select {
		   case <-ctx.Done():
			   return
		   default:
		   }
		   // 只读汇报；漂移案件关闭由 ops API 的双人确认触发
		  _ = dc
	  }
   }

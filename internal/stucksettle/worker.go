package stucksettle

import (
	"context"
	"time"
)

// WorkerConfig worker 配置。
type WorkerConfig struct {
	// DupScanInterval 双结巡检间隔；默认 60s。
	DupScanInterval time.Duration
	// ExpiryInterval 租约超时巡检间隔；默认 30s。
	ExpiryInterval time.Duration
	// MutexSweepInterval AdvanceLock 超时清扫间隔；默认 30s。
	MutexSweepInterval time.Duration
	// TransferListFn 返回需巡检的 transfer_id 列表（由上层注入）。
	TransferListFn func(ctx context.Context) ([]string, error)
	// LeaseListFn 返回需检查超时的 lease_id 列表（由上层注入）。
	LeaseListFn func(ctx context.Context) ([]string, error)
}

// Worker stucksettle 后台任务集合。

type Worker struct {
	gate *Gate
	cfg  WorkerConfig
	clock func() time.Time
}

// NewWorker 构造 worker。

  func NewWorker(gate *Gate, cfg WorkerConfig) *Worker {
	  if cfg.DupScanInterval <= 0 {
		  cfg.DupScanInterval = 60 * time.Second
	   }
	  if cfg.ExpiryInterval <= 0 {
		  cfg.ExpiryInterval = 30 * time.Second
	  }
	   if cfg.MutexSweepInterval <= 0 {
		   cfg.MutexSweepInterval = 30 * time.Second
	   }
	  return &Worker{gate: gate, cfg: cfg, clock: func() time.Time { return time.Now().UTC() }}
  }

// RunDupScan 启动双结巡检 goroutine。ctx 取消时退出。
   func (w *Worker) RunDupScan(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.DupScanInterval)
	  defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			  return
		   case <-ticker.C:
			  start := w.clock()
			  w.scanDups(ctx)
			   MetricWorkerScanDuration.Observe(w.clock().Sub(start).Seconds())
		   }
	   }
   }

  func (w *Worker) scanDups(ctx context.Context) {
	  if w.cfg.TransferListFn == nil {
		  return
	   }
	  transfers, err := w.cfg.TransferListFn(ctx)
	  if err != nil {
		  return
	   }
	   for _, trID := range transfers {
		   select {
		  case <-ctx.Done():
			   return
		  default:
		  }
		  result, err := w.gate.ScanDups(trID)
		  if err != nil {
			  continue
		   }
		   if result.NewCases > 0 {
			   MetricDupCaseNew.Add(float64(result.NewCases))
		   }
	  }
  }

// RunExpiryCheck 启动租约超时巡检 goroutine。
   func (w *Worker) RunExpiryCheck(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.ExpiryInterval)
	   defer ticker.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			  return
		   case <-ticker.C:
			  w.sweepExpiredLeases(ctx)
		  }
	   }
  }

  func (w *Worker) sweepExpiredLeases(ctx context.Context) {
	   if w.cfg.LeaseListFn == nil || w.gate == nil || w.gate.engine == nil {
		   return
	   }
	   leases, err := w.cfg.LeaseListFn(ctx)
	  if err != nil {
		  return
	   }
	  for _, lID := range leases {
		  select {
		   case <-ctx.Done():
			   return
		  default:
		  }
		   if err := w.gate.engine.ExpireLease(lID); err == nil {
			   MetricLeaseExpired.Inc()
		  }
	  }
  }

// RunMutexSweep 启动 AdvanceLock 超时清扫 goroutine。

  func (w *Worker) RunMutexSweep(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.MutexSweepInterval)
	  defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			   if w.gate != nil && w.gate.mutex != nil {
				  n := w.gate.mutex.SweepExpired(w.clock())
				   if n > 0 {
					   MetricAdvanceLocked.Add(float64(n))
				   }
			   }
		  }
	  }
   }

// RunAll 一次性启动全部 worker。

  func (w *Worker) RunAll(ctx context.Context) {
	  go w.RunDupScan(ctx)
	  go w.RunExpiryCheck(ctx)
	   go w.RunMutexSweep(ctx)
   }

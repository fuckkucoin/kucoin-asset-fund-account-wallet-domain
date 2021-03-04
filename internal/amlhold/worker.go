package amlhold

import (
	"context"
	"fmt"
	"time"
)

// Logger 简单日志接口（避免引入重量级日志库依赖）。

type Logger interface {
	Infof(format string, args ...any)
	Errorf(format string, args ...any)
}

// nopLogger 空日志，用于未注入 Logger 时。
type nopLogger struct{}

   func (nopLogger) Infof(string,
	   ...any) {
  }

   func (nopLogger) Errorf(string, ...any) {}

// WorkerConfig Worker 配置。

type WorkerConfig struct {
	AgingScanInterval time.Duration // 老化扫描间隔，默认 5m
	DriftCheckInterval time.Duration // 漂移巡检间隔，默认 10m
}

// DefaultWorkerConfig 默认 Worker 配置。

var DefaultWorkerConfig = WorkerConfig{
	AgingScanInterval:  5 * time.Minute,
	DriftCheckInterval: 10 * time.Minute,
}

// Worker 检疫 hold 后台协程组。
type Worker struct {
	svc *Service
	cfg WorkerConfig
	log Logger
	onScan func(ScanResult)
	onDrift func([]GapTicket)
}

// NewWorker 构造 Worker。
   func NewWorker(svc *Service, cfg WorkerConfig, log Logger) *Worker {
	   if cfg.AgingScanInterval <= 0 {
		   cfg.AgingScanInterval = DefaultWorkerConfig.AgingScanInterval
	   }
	   if cfg.DriftCheckInterval <= 0 {
		   cfg.DriftCheckInterval = DefaultWorkerConfig.DriftCheckInterval
	  }
	  if log == nil {
		   log = nopLogger{}
	  }
	   return &Worker{svc: svc,
		   cfg: cfg, log: log}
  }

// OnScan 注册老化扫描结果回调（如 metrics 计数、通知下游）。

  func (w *Worker) OnScan(fn func(ScanResult)) *Worker {
	   if w != nil {
		   w.onScan = fn
	   }
	   return w
  }

  // OnDrift 注册漂移巡检结果回调。

   func (w *Worker) OnDrift(fn func([]GapTicket)) *Worker {
	  if w != nil {
		   w.onDrift = fn
	   }
	   return w
   }

  // Run 启动 Worker（阻塞，直到 ctx 取消）。
   func (w *Worker) Run(ctx context.Context) error {
	  if w == nil || w.svc == nil {
		  return fmt.Errorf("amlhold worker: service nil")
	   }
	   agingTicker := time.NewTicker(w.cfg.AgingScanInterval)
	  driftTicker := time.NewTicker(w.cfg.DriftCheckInterval)
	   defer agingTicker.Stop()
	   defer driftTicker.Stop()
	  w.log.Infof("amlhold worker started: aging_interval=%s drift_interval=%s",
		  w.cfg.AgingScanInterval, w.cfg.DriftCheckInterval)
	  for {
		  select {
		   case <-ctx.Done():
			  w.log.Infof("amlhold worker stopped")
			  return ctx.Err()
		   case <-agingTicker.C:
			  w.runAgingScan()
		   case <-driftTicker.C:
			  w.runDriftCheck()
		  }
	   }
  }

   func (w *Worker) runAgingScan() {
	   result := w.svc.RunAgingScan()
	  w.log.Infof("amlhold aging scan: scanned=%d advanced=%d unchanged=%d errors=%d",
		   result.Scanned, result.Advanced, result.Unchanged, len(result.Errors))
	  for _, errMsg := range result.Errors {
		  w.log.Errorf("amlhold aging scan error: %s", errMsg)
	  }
	   for _, ev := range result.Events {
		   w.log.Infof("amlhold aging event: hold=%s case=%s %s→%s tier=%s age=%ds",
			  ev.HoldID,
			   ev.CaseID, ev.FromStatus, ev.ToStatus, ev.ToTier, ev.Age)
	  }
	  if w.onScan != nil {
		   w.onScan(result)
	   }
   }

   func (w *Worker) runDriftCheck() {
	  gaps := w.svc.CheckDrift()
	  if len(gaps) > 0 {
		   w.log.Errorf("amlhold drift check: %d gap tickets opened", len(gaps))
		   for _, g := range gaps {
			  w.log.Errorf("amlhold gap: hold=%s case=%s user=%s asset=%s ledger_state=%s",
				   g.HoldID, g.CaseID, g.UserID, g.Asset, g.LedgerState)
		  }
	  } else {
		   w.log.Infof("amlhold drift check: no gaps detected")
	   }
	   if w.onDrift != nil {
		   w.onDrift(gaps)
	  }
  }

// RunOnce 执行一次完整的老化扫描和漂移巡检（用于 CLI / cron 模式）。
   func (w *Worker) RunOnce() (ScanResult, []GapTicket) {
	  if w == nil || w.svc == nil {
		   return ScanResult{}, nil
	  }
	   w.runAgingScan()
	  gaps := w.svc.CheckDrift()
	  return w.svc.RunAgingScan(), gaps
   }

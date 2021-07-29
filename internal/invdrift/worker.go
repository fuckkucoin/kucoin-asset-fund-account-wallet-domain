package invdrift

import (
	"context"
	"time"
	"github.com/rs/zerolog"
)

// WorkerConfig 调度配置

type WorkerConfig struct {
	ScanInterval   time.Duration // 扫描间隔，默认 5min
	ExpireInterval time.Duration // 建议 TTL 检查间隔，默认 1min
	EscalateAfter  time.Duration // 批次未收敛多久升级，默认 30min
	IntegrityPeriod time.Duration // 证明完整性巡检间隔，默认 10min
}

// DefaultWorkerConfig 默认调度配置
var DefaultWorkerConfig = WorkerConfig{
	ScanInterval: 5 * time.Minute,
	ExpireInterval: time.Minute,
	EscalateAfter:   30 * time.Minute,
	IntegrityPeriod: 10 * time.Minute,
}

// Worker 后台调度器
type Worker struct {
	svc *Service
	cfg WorkerConfig
	log zerolog.Logger
}

// NewWorker 创建 Worker
  func NewWorker(svc *Service, cfg WorkerConfig, log zerolog.Logger) *Worker {
	   if cfg.ScanInterval <= 0 {
		  cfg.ScanInterval = DefaultWorkerConfig.ScanInterval
	   }
	  if cfg.ExpireInterval <= 0 {
		   cfg.ExpireInterval = DefaultWorkerConfig.ExpireInterval
	   }
	  if cfg.EscalateAfter <= 0 {
		   cfg.EscalateAfter = DefaultWorkerConfig.EscalateAfter
	  }
	   if cfg.IntegrityPeriod <= 0 {
		   cfg.IntegrityPeriod = DefaultWorkerConfig.IntegrityPeriod
	   }
	  return &Worker{svc: svc, cfg: cfg, log: log}
  }

// Run 启动所有后台任务，ctx cancel 时停止。
   func (w *Worker) Run(ctx context.Context) {
	   scanTick := time.NewTicker(w.cfg.ScanInterval)
	   expireTick := time.NewTicker(w.cfg.ExpireInterval)
	  integrityTick := time.NewTicker(w.cfg.IntegrityPeriod)
	  escalateTick := time.NewTicker(w.cfg.EscalateAfter / 3)
	  defer scanTick.Stop()
	   defer expireTick.Stop()
	   defer integrityTick.Stop()
	  defer escalateTick.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			  w.log.Info().Msg("invdrift worker stopped")
			  return
		   case <-scanTick.C:
			  w.runScan(ctx)
		   case <-expireTick.C:
			   w.runExpire(ctx)
		  case <-integrityTick.C:
			  w.runIntegrityCheck(ctx)
		  case <-escalateTick.C:
			  w.runEscalate(ctx)
		   }
	  }
   }

   func (w *Worker) runScan(ctx context.Context) {
	   batch, err := w.svc.Scan(ctx, "")
	  if err != nil {
		   if err == ErrConflict {
			   w.log.Debug().Msg("invdrift scan skipped: concurrent scan in progress")
			   return
		   }
		   w.log.Error().Err(err).Msg("invdrift scheduled scan failed")
		   DriftScanErrors.Inc()
		   return
	   }
	   w.log.Info().
		  Str("batch_id", batch.ID).
		   Int("crit", batch.CritCount).
		   Int("warn", batch.WarnCount).
		   Msg("invdrift scan completed")
	   DriftBatchesTotal.WithLabelValues(string(batch.State)).Inc()
	  if batch.CritCount > 0 {
		   DriftCritTotal.Add(float64(batch.CritCount))
	   }
  }

   func (w *Worker) runExpire(ctx context.Context) {
	  n, err := w.svc.ExpireStaleAdvices(ctx)
	   if err != nil {
		  w.log.Error().Err(err).Msg("invdrift expire advices failed")
		  return
	  }
	  if n > 0 {
		   w.log.Info().Int("expired", n).Msg("invdrift: stale advice expired")
		   DriftAdvicesExpired.Add(float64(n))
	  }
   }

  func (w *Worker) runIntegrityCheck(ctx context.Context) {
	  batches, err := w.svc.store.ListBatches(ctx, 5, 0)
	  if err != nil {
		  w.log.Error().Err(err).Msg("invdrift integrity check: list batches failed")
		  return
	  }
	  fail := 0
	   for _, b := range batches {
		  proofs, err := w.svc.store.ListProofsByBatch(ctx, b.ID)
		   if err != nil {
			   continue
		  }
		   for _, p := range proofs {
			  if !p.IntegrityOK() {
				   fail++
				   w.log.Error().
					  Str("proof_id", p.ID).
					  Str("batch_id", p.BatchID).
					   Str("asset", p.Asset).
					   Msg("invdrift: proof integrity FAILED")
				  DriftProofIntegrityFail.Inc()
			   }
		   }
	   }
	   if fail == 0 {
		   w.log.Debug().Msg("invdrift integrity check: all proofs OK")
	   }
   }

   func (w *Worker) runEscalate(ctx context.Context) {
	  batches,
		   err := w.svc.store.ListOpenBatches(ctx)
	   if err != nil {
		   w.log.Error().Err(err).Msg("invdrift escalate check failed")
		  return
	  }
	  now := w.svc.now()
	  for _, b := range batches {
		   if b.State == BatchStateSealed {
			  age := now.Sub(b.ScanAt)
			  if age >= w.cfg.EscalateAfter {
				  w.log.Warn().
					   Str("batch_id", b.ID).
					   Dur("age", age).
					  Int("crit", b.CritCount).
					  Msg("invdrift: sealed batch not resolved, escalating")
				  DriftEscalateTotal.Inc()
			   }
		  }
	  }
   }

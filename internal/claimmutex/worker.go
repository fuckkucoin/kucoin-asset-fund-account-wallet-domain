package claimmutex

import (
	"context"
	"time"
)

// WorkerConfig 工作进程配置。
type WorkerConfig struct {
	ExpireInterval time.Duration // 过期扫描间隔，默认 1min
	VerifyInterval     time.Duration // 证明验证间隔，默认 30s
	DupScanInterval    time.Duration // 双贷案件巡检间隔，默认 2min
	RecoveScanInterval time.Duration // 竞态扫描间隔，默认 5min
	BatchSize          int           // 每批处理数量，默认 100
	DupCaseSLASec int64 // 双贷案件 SLA 阈值（秒），超过后触发告警
}

// DefaultWorkerConfig 默认工作进程配置。
var DefaultWorkerConfig = WorkerConfig{
	ExpireInterval: time.Minute,
	VerifyInterval:     30 * time.Second,
	DupScanInterval:    2 * time.Minute,
	RecoveScanInterval: 5 * time.Minute,
	BatchSize:     100,
	DupCaseSLASec: 3600,
}

// WorkerMetrics Prometheus 指标回调（可选注入）。
type WorkerMetrics struct {
	IncExpired func()
	IncVerified   func()
	IncVerifyFail func()
	IncDupSLA     func()
	IncRaceFound func()
}

   func (m *WorkerMetrics) incExpired() {
	  if m != nil && m.IncExpired != nil {
		   m.IncExpired()
	  }
  }
  func (m *WorkerMetrics) incVerified() {
	   if m != nil && m.IncVerified != nil {
		  m.IncVerified()
	   }
  }

  func (m *WorkerMetrics) incVerifyFail() {
	   if m != nil && m.IncVerifyFail != nil {
		   m.IncVerifyFail()
	  }
  }

   func (m *WorkerMetrics) incDupSLA() {
	  if m != nil && m.IncDupSLA != nil {
		  m.IncDupSLA()
	   }
   }
  func (m *WorkerMetrics) incRaceFound() {
	  if m != nil && m.IncRaceFound != nil {
		   m.IncRaceFound()
	   }
  }

// Worker 异步巡检工作进程。

type Worker struct {
	svc *Service
	cfg     WorkerConfig
	metrics *WorkerMetrics
}

// NewWorker 创建工作进程。
  func NewWorker(svc *Service, cfg WorkerConfig, metrics *WorkerMetrics) *Worker {
	  if cfg.BatchSize <= 0 {
		  cfg.BatchSize = DefaultWorkerConfig.BatchSize
	   }
	  if cfg.ExpireInterval <= 0 {
		  cfg.ExpireInterval = DefaultWorkerConfig.ExpireInterval
	   }
	   if cfg.VerifyInterval <= 0 {
		  cfg.VerifyInterval = DefaultWorkerConfig.VerifyInterval
	   }
	   if cfg.DupScanInterval <= 0 {
		  cfg.DupScanInterval = DefaultWorkerConfig.DupScanInterval
	   }
	   if cfg.RecoveScanInterval <= 0 {
		  cfg.RecoveScanInterval = DefaultWorkerConfig.RecoveScanInterval
	  }
	  return &Worker{svc: svc, cfg: cfg, metrics: metrics}
  }

// Run 启动所有后台 goroutine，直到 ctx 取消。
   func (w *Worker) Run(ctx context.Context) {
	   go w.runExpireSweeper(ctx)
	   go w.runProofVerifier(ctx)
	   go w.runDupCaseScanner(ctx)
	   <-ctx.Done()
  }

// ── ExpireSweeper ──────────────────────────────────────────────────────────

   func (w *Worker) runExpireSweeper(ctx context.Context) {
	  tick := time.NewTicker(w.cfg.ExpireInterval)
	   defer tick.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return
		  case <-tick.C:
			   w.sweepExpiredLeases(ctx)
		   }
	   }
   }

  func (w *Worker) sweepExpiredLeases(ctx context.Context) {
	   now := w.svc.now()
	  leases, err := w.svc.store.ListExpiredLeases(ctx, now, w.cfg.BatchSize)
	   if err != nil {
		  return
	  }
	  for _, l := range leases {
		  if err := w.svc.store.UpdateLeaseState(ctx, l.LeaseID, LeaseOpen, LeaseExpired, now, "ttl expired"); err != nil {
			  continue
		   }
		  w.metrics.incExpired()
	  }
  }

// ── ProofVerifier ─────────────────────────────────────────────────────────

   func (w *Worker) runProofVerifier(ctx context.Context) {
	  tick := time.NewTicker(w.cfg.VerifyInterval)
	  defer tick.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			   return
		  case <-tick.C:
			   w.verifyPendingProofs(ctx)
		  }
	  }
   }

   func (w *Worker) verifyPendingProofs(ctx context.Context) {
	  proofs, err := w.svc.store.ListPendingProofs(ctx, w.cfg.BatchSize)
	  if err != nil {
		   return
	   }
	  for _, p := range proofs {
		   if _, err := w.svc.VerifyProof(ctx, p.ProofID); err != nil {
			   w.metrics.incVerifyFail()
			   continue
		  }
		  w.metrics.incVerified()
	   }
  }

// ── DupCaseScanner ────────────────────────────────────────────────────────

   func (w *Worker) runDupCaseScanner(ctx context.Context) {
	   tick := time.NewTicker(w.cfg.DupScanInterval)
	  defer tick.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			   return
		   case <-tick.C:
			  w.scanDupCases(ctx)
		   }
	  }
   }

  func (w *Worker) scanDupCases(ctx context.Context) {
	  cases, err := w.svc.store.ListOpenDupCases(ctx, w.cfg.BatchSize)
	  if err != nil {
		   return
	  }
	  now := w.svc.now()
	  for _, c := range cases {
		  age := now.Sub(c.DetectedAt).Seconds()
		  if int64(age) > w.cfg.DupCaseSLASec {
			  w.metrics.incDupSLA()
		  }
	   }
   }

// ── RunOnce 辅助（集成 / 手动触发）──────────────────────────────────
// RunExpireOnce 单次执行过期扫描（供和运维手动触发）。
   func (w *Worker) RunExpireOnce(ctx context.Context) int {
	  now := w.svc.now()
	  leases, err := w.svc.store.ListExpiredLeases(ctx, now, w.cfg.BatchSize)
	   if err != nil {
		   return 0
	   }
	   count := 0
	  for _, l := range leases {
		  if err := w.svc.store.UpdateLeaseState(ctx, l.LeaseID, LeaseOpen, LeaseExpired, now, "ttl expired"); err != nil {
			   continue
		  }
		  count++
		  w.metrics.incExpired()
	   }
	   return count
  }

// RunVerifyOnce 单次执行证明验证（供和运维手动触发）。

   func (w *Worker) RunVerifyOnce(ctx context.Context) (verified, failed int) {
	   proofs,
		  err := w.svc.store.ListPendingProofs(ctx, w.cfg.BatchSize)
	  if err != nil {
		  return 0, 0
	   }
	  for _, p := range proofs {
		  if _, err := w.svc.VerifyProof(ctx, p.ProofID); err != nil {
			   failed++
			   w.metrics.incVerifyFail()
			   continue
		  }
		   verified++
		  w.metrics.incVerified()
	  }
	   return
  }

package batchhopgate

import (
	"context"
	"time"
)

const (
	// defaultWorkerInterval Worker 默认轮询间隔（60 秒）。
	defaultWorkerInterval = 60 * time.Second
	// defaultMaxBatch 单次扫描最大处理条数。
	defaultMaxBatch = 50
)

// WorkerConfig Worker 配置参数。

type WorkerConfig struct {
	// ScanInterval 残留扫描 / 占用巡检间隔，默认 60s。
	ScanInterval time.Duration
	// ReaperInterval 续跳租约吊销间隔，默认 60s。
	ReaperInterval time.Duration
	// MaxBatch 单次处理批量上限，默认 50。
	MaxBatch int
}

   func (c *WorkerConfig) fill() {
	  if c.ScanInterval <= 0 {
		   c.ScanInterval = defaultWorkerInterval
	  }
	   if c.ReaperInterval <= 0 {
		  c.ReaperInterval = defaultWorkerInterval
	   }
	   if c.MaxBatch <= 0 {
		   c.MaxBatch = defaultMaxBatch
	   }
  }

// Workers 聚合三个后台 worker。

type Workers struct {
	svc *Service
	cfg WorkerConfig
	clock   func() time.Time
	metrics *GateMetrics
}

// NewWorkers 构造 Workers。

   func NewWorkers(svc *Service, cfg WorkerConfig) *Workers {
	  cfg.fill()
	   return &Workers{
		  svc: svc,
		   cfg: cfg,
		  clock: func() time.Time { return time.Now().UTC() },
		  metrics: svc.metrics,
	  }
  }

   func (w *Workers) WithClock(c func() time.Time) *Workers {
	   if w != nil && c != nil {
		  w.clock = c
	   }
	   return w
   }

   // ─────────────────────────────────────────────────────────────────────────────
  // Start — 启动所有 worker goroutine
  // ─────────────────────────────────────────────────────────────────────────────
   // Start 在后台启动三个 worker goroutine。ctx 取消时所有 worker 优雅退出。
   func (w *Workers) runResidualScanner(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.ScanInterval)
	   defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			   w.scanResiduals(ctx)
		   }
	   }
   }

  func (w *Workers) scanResiduals(ctx context.Context) {
	  w.metrics.workerResidualScanBatch.Inc()
	  cases, err := w.svc.store.ListResidualsByStatus(ctx, ResidualOpen, w.cfg.MaxBatch)
	   if err != nil {
		  return
	   }
	  for _, c := range cases {
		  // 若已有 ProofID，说明证明已构建（open→proven 推进失败），直接重试 Prove。
		  if c.ProofID != "" {
			  _, _ = w.svc.residualFSM.Prove(ctx, c.ID, c.ProofID)
			   continue
		   }
		  // 尝试重建占用证明。
		   totals := BuildSubjectTotals(c.SucceededHops)
		   conserveOK := ConserveCheck(c.SucceededHops)
		   proof, err := w.svc.proofVault.Build(ctx, BuildReq{
			  ResidualCaseID: c.ID,
			   UserID:         c.UserID,
			   Asset:          c.Asset,
			   BeforeRoot:     c.BeforeRoot,
			   PartialRoot:   c.PartialRoot,
			   SucceededHops: c.SucceededHops,
			   SubjectTotals: totals,
			  ConserveOK: conserveOK,
			   BuiltBy: "residual-scanner-worker",
		  })
		   if err != nil {
			  w.metrics.proofBuildFailed.Inc()
			  continue
		  }
		  _, _ = w.svc.residualFSM.Prove(ctx, c.ID, proof.ID)
		   w.metrics.residualProven.Inc()
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// OccupyPatrol — proven/resuming 案占用证明完整性巡检
// ─────────────────────────────────────────────────────────────────────────────
// runOccupyPatrol 周期巡检 proven/resuming 残留案的证明 Digest 完整性。
// Digest 不一致时记录 proofDigestFail 指标并告警（不自动修复，需人工处理）。
  func (w *Workers) runOccupyPatrol(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.ScanInterval)
	  defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			   return
		   case <-ticker.C:
			   w.patrolProofs(ctx)
		  }
	  }
   }

  func (w *Workers) patrolProofs(ctx context.Context) {
	  w.metrics.workerOccupyPatrolBatch.Inc()
	  for _, st := range []ResidualStatus{ResidualProven, ResidualResuming} {
		   cases, err := w.svc.store.ListResidualsByStatus(ctx, st, w.cfg.MaxBatch)
		  if err != nil {
			   continue
		  }
		  for _, c := range cases {
			  if c.ProofID == "" {
				  continue
			  }
			  _, err := w.svc.proofVault.Verify(ctx, c.ProofID)
			   if err == ErrProofDigestMismatch {
				   w.metrics.proofDigestFail.Inc()
				  // Digest 不一致：不自动修复；依赖运维告警 + 四眼确认后 Block 案。
			  }
		  }
	  }
   }

// ─────────────────────────────────────────────────────────────────────────────
// LeaseReaper — 过期续跳租约吊销
// ─────────────────────────────────────────────────────────────────────────────
// runLeaseReaper 周期吊销超过 TTL 的 open 续跳租约。
  func (w *Workers) runLeaseReaper(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.ReaperInterval)
	  defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			  return
		   case <-ticker.C:
			   w.reapExpiredLeases(ctx)
		   }
	   }
  }

  func (w *Workers) reapExpiredLeases(ctx context.Context) {
	  w.metrics.workerLeaseReaperBatch.Inc()
	  count, err := w.svc.leaseFSM.ExpireStale(ctx, w.clock(), w.cfg.MaxBatch)
	   if err != nil {
		  return
	  }
	   for i := 0; i < count; i++ {
		  w.metrics.leaseExpired.Inc()
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// RunOnce — 供使用的单次手动触发接口
// ─────────────────────────────────────────────────────────────────────────────
// RunScanOnce 手动触发一次残留扫描（供）。
  func (w *Workers) RunScanOnce(ctx context.Context) { w.scanResiduals(ctx) }

// RunPatrolOnce 手动触发一次占用巡检（供）。
  func (w *Workers) RunPatrolOnce(ctx context.Context) { w.patrolProofs(ctx) }

// RunReaperOnce 手动触发一次过期租约吊销（供）。
   func (w *Workers) RunReaperOnce(ctx context.Context) { w.reapExpiredLeases(ctx) }

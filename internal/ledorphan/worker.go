package ledorphan

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// WorkerConfig worker 配置。
type WorkerConfig struct {
	// ScanInterval 孤儿扫描间隔（默认 5 分钟）。
	ScanInterval time.Duration
	// ScanBatchLimit 单次扫描最大批次数（默认 50）。
	ScanBatchLimit int
	// ReclaimInterval 世代锁回收间隔（默认 2 分钟）。
	ReclaimInterval time.Duration
	// CheckInterval 完整性证明校验间隔（默认 30 分钟）。
	CheckInterval time.Duration
	// AgingInterval 孤儿老化巡检间隔（默认 1 小时）。
	AgingInterval time.Duration
	// AgingThreshold 孤儿超过此时长未处置则升级告警（默认 24 小时）。
	AgingThreshold time.Duration
	// Actor 系统 actor 标识（用于扫描动作的 auditable actor 记录）。
	Actor string
}

// DefaultWorkerConfig 默认配置。

   func DefaultWorkerConfig() WorkerConfig {
	  return WorkerConfig{
		  ScanInterval: 5 * time.Minute,
		  ScanBatchLimit: 50,
		  ReclaimInterval: 2 * time.Minute,
		   CheckInterval:   30 * time.Minute,
		  AgingInterval: 1 * time.Hour,
		  AgingThreshold: 24 * time.Hour,
		  Actor:          "orphan-worker",
	   }
  }

// Workers 持有四类 worker 的运行入口。
type Workers struct {
	svc *Service
	cfg WorkerConfig
	logger func(level, msg string, fields map[string]interface{})
}

// NewWorkers 构造 Workers。
// logger 为可选的结构化日志函数（nil 时静默）。

  func NewWorkers(svc *Service, cfg WorkerConfig, logger func(level, msg string, fields map[string]interface{})) *Workers {
	   if cfg.ScanInterval <= 0 {
		   cfg.ScanInterval = DefaultWorkerConfig().ScanInterval
	   }
	  if cfg.ReclaimInterval <= 0 {
		  cfg.ReclaimInterval = DefaultWorkerConfig().ReclaimInterval
	   }
	  if cfg.CheckInterval <= 0 {
		   cfg.CheckInterval = DefaultWorkerConfig().CheckInterval
	  }
	  if cfg.AgingInterval <= 0 {
		  cfg.AgingInterval = DefaultWorkerConfig().AgingInterval
	  }
	  if cfg.AgingThreshold <= 0 {
		  cfg.AgingThreshold = DefaultWorkerConfig().AgingThreshold
	   }
	  if cfg.Actor == "" {
		  cfg.Actor = "orphan-worker"
	   }
	  return &Workers{svc: svc, cfg: cfg, logger: logger}
   }

   // RunAll 启动全部四类 worker，直至 ctx 取消。
  // 应在独立 goroutine 中调用，或通过 app.Runtime.StartBackground 集成。
   func (w *Workers) RunAll(ctx context.Context) {
	   go w.RunOrphanScanner(ctx)
	   go w.RunGenerationReclaimer(ctx)
	  go w.RunIntegrityChecker(ctx)
	   go w.RunAgingUpgrader(ctx)
  }

// ── 1. OrphanScanner ─────────────────────────────────────────────────────────
// RunOrphanScanner 周期性扫描 submitted 批次，产出孤儿报告。
// 扫描为只读操作，不持有世代锁；发现的孤儿报告落盘后等待运营处置。
  func (w *Workers) RunOrphanScanner(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.ScanInterval)
	   defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			  return
		   case <-ticker.C:
			   w.scanOnce(ctx)
		  }
	  }
   }

  func (w *Workers) scanOnce(ctx context.Context) {
	  start := time.Now()
	  resp, err := w.svc.Scan(ctx, ScanReq{Actor: w.cfg.Actor})
	  elapsed := time.Since(start)
	  if err != nil {
		   OrphanScanErrors.Inc()
		   w.log("warn", "orphan scanner error", map[string]interface{}{
			   "err": err.Error(), "elapsed_ms": elapsed.Milliseconds(),
		  })
		   return
	   }
	  OrphanScanRuns.Inc()
	  OrphanScanBatches.Add(float64(resp.ScannedBatches))
	  OrphanScanFound.Add(float64(resp.TotalOrphans))
	  if resp.TotalOrphans > 0 {
		  w.log("warn", "orphan scanner: orphans found", map[string]interface{}{
			   "scanned_batches": resp.ScannedBatches,
			  "new_reports":     resp.NewReports,
			   "total_orphans": resp.TotalOrphans,
			  "elapsed_ms": elapsed.Milliseconds(),
		  })
	  } else {
		  w.log("info", "orphan scanner: clean", map[string]interface{}{
			   "scanned_batches": resp.ScannedBatches,
			  "elapsed_ms": elapsed.Milliseconds(),
		  })
	  }
  }

// ── 2. GenerationReclaimer ────────────────────────────────────────────────────
// RunGenerationReclaimer 周期性回收超时世代锁。
// 宕机 worker 持有的锁在 GenLockTTL 后自动失效；本 worker 将其状态更新为 expired，
// 使其他 worker 可以立即抢锁而不需要等待 TTL 观察窗。

  func (w *Workers) RunGenerationReclaimer(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.ReclaimInterval)
	   defer ticker.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			   return
		   case <-ticker.C:
			   w.reclaimOnce(ctx)
		   }
	   }
  }

   func (w *Workers) reclaimOnce(ctx context.Context) {
	   if w.svc.gen == nil {
		  return
	  }
	  expired, err := w.svc.gen.ExpireStale(ctx, 200)
	  if err != nil {
		   GenReclaimErrors.Inc()
		   w.log("warn", "generation reclaimer error", map[string]interface{}{"err": err.Error()})
		  return
	   }
	  GenReclaimRuns.Inc()
	   GenExpiredTotal.Add(float64(expired))
	   if expired > 0 {
		   w.log("info", "generation reclaimer: expired stale locks", map[string]interface{}{
			  "expired": expired,
		   })
	   }
   }

// ── 3. IntegrityChecker ───────────────────────────────────────────────────────
// RunIntegrityChecker 周期性对已有 IntegrityProof 执行哈希重校验。
// 若发现 ProofInvalid 则触发告警（通过 metrics 和日志）。
// 正常环境此校验应始终通过；失败表示存储被篡改或有软件 bug。

   func (w *Workers) RunIntegrityChecker(ctx context.Context) {
	   ticker := time.NewTicker(w.cfg.CheckInterval)
	  defer ticker.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			  return
		   case <-ticker.C:
			   w.checkIntegrityOnce(ctx)
		  }
	  }
  }

   func (w *Workers) checkIntegrityOnce(ctx context.Context) {
	  if w.svc.proof == nil || w.svc.kv == nil {
		  return
	  }
	   rows,
		  err := w.svc.kv.List(ctx, kindIntegrityProof, 200)
	  if err != nil {
		  IntegrityCheckErrors.Inc()
		   w.log("warn", "integrity checker: list error", map[string]interface{}{"err": err.Error()})
		  return
	   }
	   checked, invalid := 0, 0
	   for _, row := range rows {
		  checked++
		   var proof IntegrityProof
		  if err := json.Unmarshal(row.Body, &proof); err != nil {
			  continue
		   }
		  if _, verr := w.svc.proof.Verify(ctx, proof.ProofID); verr != nil {
			   invalid++
			  IntegrityCheckInvalid.Inc()
			  w.log("error", "integrity checker: PROOF MISMATCH", map[string]interface{}{
				  "proof_id": proof.ProofID,
				   "batch_id": proof.BatchID,
				  "err":      verr.Error(),
			   })
		   }
	   }
	   IntegrityCheckRuns.Inc()
	  if checked > 0 {
		  w.log("info", "integrity checker: done", map[string]interface{}{
			  "checked": checked, "invalid": invalid,
		   })
	  }
   }

// ── 4. AgingUpgrader ──────────────────────────────────────────────────────────
// RunAgingUpgrader 周期性巡检长期未处置的孤儿（超过 AgingThreshold），
// 发出高先级告警，推动运营及时处置。
// 本 worker 不自动封堵孤儿，仅提升告警级别。

   func (w *Workers) RunAgingUpgrader(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.AgingInterval)
	  defer ticker.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			  return
		  case <-ticker.C:
			  w.agingOnce(ctx)
		   }
	   }
  }

  func (w *Workers) agingOnce(ctx context.Context) {
	   if w.svc.kv == nil {
		  return
	   }
	  reports, err := ListReports(ctx, w.svc.kv, "", string(ReportOpen), 200, "")
	  if err != nil {
		  AgingErrors.Inc()
		  w.log("warn", "aging upgrader: list error", map[string]interface{}{"err": err.Error()})
		   return
	   }
	   now := time.Now().UTC()
	  aged := 0
	  for _, rep := range reports {
		  for _, o := range rep.Items {
			  if o.Status != OrphanDetected {
				   continue
			  }
			   age := now.Sub(o.DetectedAt)
			  if age < w.cfg.AgingThreshold {
				  continue
			   }
			   aged++
			   AgingOrphans.Inc()
			  w.log("error", "aging upgrader: unresolved orphan exceeds SLA", map[string]interface{}{
				   "orphan_id": o.OrphanID,
				   "batch_id": o.BatchID,
				   "age_hours": fmt.Sprintf("%.1f", age.Hours()),
				   "idem_key":  o.IdempotencyKey,
				   "kind": string(o.Kind),
				   "asset":  o.Asset,
				  "amount": o.Amount,
			   })
		  }
	  }
	   AgingRuns.Inc()
	   if aged > 0 {
		   w.log("warn", "aging upgrader: aged orphans found", map[string]interface{}{"aged": aged})
	  }
  }

// ── 日志辅助 ──────────────────────────────────────────────────────────────────

  func (w *Workers) log(level, msg string, fields map[string]interface{}) {
	   if w.logger != nil {
		  w.logger(level, msg, fields)
	   }
  }

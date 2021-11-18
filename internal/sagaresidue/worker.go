package sagaresidue

import (
	"context"
	"fmt"
	"time"
)

// WorkerConfig worker 配置。
type WorkerConfig struct {
	// ScanInterval 残留扫描间隔（默认 5 分钟）。
	ScanInterval time.Duration
	// ScanBatchSize 单次扫描最多处理的 saga Run 数量（默认 100）。
	ScanBatchSize int
	// ExecuteInterval Closeout executor 拾取间隔（默认 1 分钟）。
	ExecuteInterval time.Duration
	// ReclaimInterval 互斥锁回收间隔（默认 2 分钟）。
	ReclaimInterval time.Duration
	// AgingInterval 老化巡检间隔（默认 30 分钟）。
	AgingInterval time.Duration
	// SLAMedium 中级 SLA（超时升级，默认 4 小时）。
	SLAMedium time.Duration
	// SLAHigh 高级 SLA（默认 1 小时）。
	SLAHigh time.Duration
	// SLACritical critical 级别超出时间进入运营队列（默认 15 分钟）。
	SLACritical time.Duration
	// WorkerID worker 标识（供互斥锁 Holder 字段使用）。
	WorkerID string
}

// DefaultWorkerConfig 默认配置。
   func DefaultWorkerConfig() WorkerConfig {
	  return WorkerConfig{
		  ScanInterval:    5 * time.Minute,
		  ScanBatchSize:   DefaultScanBatchSize,
		  ExecuteInterval: 1 * time.Minute,
		   ReclaimInterval: 2 * time.Minute,
		   AgingInterval:   30 * time.Minute,
		   SLAMedium: DefaultSLAMedium,
		   SLAHigh:     DefaultSLAHigh,
		   SLACritical: DefaultSLACritical,
		   WorkerID: "saga-residue-worker",
	   }
   }

// Logger 最小日志接口，供 worker 输出结构化日志。
type Logger interface {
	Info(msg string, fields map[string]any)
	Error(msg string, err error, fields map[string]any)
}

// Workers 持有四类 worker 的运行控制。
type Workers struct {
	svc *Service
	cfg    WorkerConfig
	logger Logger
	// partialReader 供 ResidueScanner 遍历 partial_failed Run 使用。
	partialReader PartialReader
	// sagaRunLister 遍历所有 saga Run（提供 SagaRunID 列表供扫描）。
	sagaRunLister SagaRunLister
}

// SagaRunLister 遍历 safesaga Run 的只读接口。
type SagaRunLister interface {
	// ListPartialFailed 返回 Partial.Cause != "" 且 Partial.Closed == false 的 Run ID 列表。
	// after 为游标（上次最大 ID），limit 为批量大小。
	ListPartialFailed(ctx context.Context, after string, limit int) ([]string, error)
}

// NewWorkers 构造 Workers。

  func NewWorkers(svc *Service, cfg WorkerConfig, logger Logger, lister SagaRunLister) *Workers {
	   return &Workers{
		  svc:    svc,
		  cfg:    cfg,
		   logger: logger,
		   sagaRunLister: lister,
	  }
   }

// StartAll 启动所有 worker goroutine，ctx 取消时全部退出。

  func (w *Workers) StartAll(ctx context.Context) {
	  go w.runResidueScanner(ctx)
	  go w.runCloseoutExecutor(ctx)
	   go w.runMutexReclaimer(ctx)
	  go w.runAgingUpgrader(ctx)
  }

// ── 1. ResidueScanner ─────────────────────────────────────────────────────────

  func (w *Workers) runResidueScanner(ctx context.Context) {
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
	   if w.sagaRunLister == nil {
		   return
	   }
	   ResidueWorkerScanRuns.Inc()
	   after := ""
	  limit := w.cfg.ScanBatchSize
	   if limit <= 0 {
		   limit = DefaultScanBatchSize
	  }
	  for {
		   runIDs, err := w.sagaRunLister.ListPartialFailed(ctx, after, limit)
		   if err != nil {
			   ResidueWorkerScanErrors.Inc()
			  if w.logger != nil {
				  w.logger.Error("residue scanner list partial failed", err, nil)
			  }
			  return
		  }
		  if len(runIDs) == 0 {
			   break
		   }
		   for _, runID := range runIDs {
			   after = runID
			  w.scanSingleRun(ctx, runID)
		  }
		  if len(runIDs) < limit {
			  break
		   }
	  }
  }

  func (w *Workers) scanSingleRun(ctx context.Context, runID string) {
	   req := DetectReq{
		   SagaRunID: runID,
		  ScanActor: w.cfg.WorkerID,
	   }
	  report, rrun, err := w.svc.DetectResidue(ctx, req)
	  if err == ErrNoResidues {
		  return
	  }
	   if err != nil {
		   ResidueWorkerScanErrors.Inc()
		  if w.logger != nil {
			   w.logger.Error("residue scanner detect", err, map[string]any{"saga_run_id": runID})
		   }
		  return
	  }
	  ResidueWorkerResidueFound.Add(float64(len(report.Items)))
	   if w.logger != nil {
		   w.logger.Info("residue detected", map[string]any{
			  "saga_run_id": runID,
			  "residue_run_id": rrun.ID,
			  "items":    len(report.Items),
			  "severity": string(report.Severity),
			  "total_amount": report.TotalAmount,
		   })
	  }
   }

// ── 2. CloseoutExecutor ───────────────────────────────────────────────────────

  func (w *Workers) runCloseoutExecutor(ctx context.Context) {
	  ticker := time.NewTicker(w.cfg.ExecuteInterval)
	  defer ticker.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			   return
		  case <-ticker.C:
			   w.executeOnce(ctx)
		   }
	   }
  }

   func (w *Workers) executeOnce(ctx context.Context) {
	   ResidueWorkerExecuteRuns.Inc()
	   // 拾取 state=closing 的 ResidueRun（最多 20 条/轮）
	   runs,
		  err := w.svc.ListRuns(ctx, RunListReq{State: RunStateClosing, Limit: 20})
	   if err != nil {
		  ResidueWorkerExecuteErrors.Inc()
		  return
	  }
	   for _, rrun := range runs {
		  w.executeRunOnce(ctx, &rrun)
	  }
   }

   func (w *Workers) executeRunOnce(ctx context.Context, rrun *ResidueRun) {
	   if rrun.CloseoutPlanID == "" {
		   return
	   }
	   plan,
		   err := w.svc.GetPlan(ctx, rrun.CloseoutPlanID)
	   if err != nil || plan == nil {
		   return
	  }
	   if plan.State == RunStateClosed {
		   return
	   }
	  holderID := fmt.Sprintf("%s:exec:%s", w.cfg.WorkerID, newID()[:8])
	  mg, err := w.svc.mutexMgr.Acquire(ctx, rrun.SagaRunID, holderID)
	   if err == ErrGenLocked {
		   // 另一 worker 正在执行，跳过
		   return
	  }
	   if err != nil {
		   ResidueWorkerExecuteErrors.Inc()
		   return
	  }
	  if err := w.svc.ExecuteCloseout(ctx, plan.ID, holderID, mg.Generation); err != nil {
		   ResidueWorkerExecuteErrors.Inc()
		   if w.logger != nil {
			   w.logger.Error("closeout execute", err, map[string]any{
				   "plan_id":        plan.ID,
				  "residue_run_id": rrun.ID,
			  })
		  }
	   }
  }

// ── 3. MutexReclaimer ─────────────────────────────────────────────────────────

   func (w *Workers) runMutexReclaimer(ctx context.Context) {
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
	  n, err := w.svc.mutexMgr.ReclaimExpired(ctx)
	   if err != nil {
		   if w.logger != nil {
			  w.logger.Error("mutex reclaimer reclaim", err, nil)
		  }
		  return
	  }
	   if n > 0 {
		   ResidueWorkerMutexReclaimed.Add(float64(n))
		  if w.logger != nil {
			  w.logger.Info("mutex locks reclaimed", map[string]any{"count": n})
		   }
	   }
   }

// ── 4. AgingUpgrader ──────────────────────────────────────────────────────────

  func (w *Workers) runAgingUpgrader(ctx context.Context) {
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
	   ResidueWorkerAgingRuns.Inc()
	   // 拾取 residue_open / escalated 的 Run（最多 200 条）
	   runs, err := w.svc.ListRuns(ctx, RunListReq{State: RunStateResidueOpen, Limit: 200})
	   if err != nil {
		   return
	  }
	  escalated, _ := w.svc.ListRuns(ctx, RunListReq{State: RunStateEscalated, Limit: 200})
	   runs = append(runs, escalated...)
	  now := time.Now().UTC()
	   for _, rrun := range runs {
		   age := now.Sub(rrun.CreatedAt)
		   newSev := w.ageSeverity(rrun.Severity, age)
		  if newSev != rrun.Severity {
			   rrun.Severity = newSev
			  rrun.UpdatedAt = now
			  if newSev == SeverityCritical && rrun.State != RunStateEscalated {
				  rrun.State = RunStateEscalated
				  t := now
				   rrun.EscalatedAt = &t
			   }
			   _ = w.svc.store.SaveResidueRun(ctx, &rrun)
			  ResidueWorkerAgingUpgraded.Inc()
			   if w.logger != nil {
				  w.logger.Info("residue run aging upgraded", map[string]any{
					   "residue_run_id": rrun.ID,
					   "saga_run_id":    rrun.SagaRunID,
					   "old_severity":   string(rrun.Severity),
					  "new_severity": string(newSev),
					  "age_seconds":  age.Seconds(),
				  })
			  }
		  }
	  }
   }

// ageSeverity 根据运行时长和当前级别，决定是否升级严重级别。

  func (w *Workers) ageSeverity(current Severity, age time.Duration) Severity {
	  switch current {
	  case SeverityLow:
		   if age > w.cfg.SLAMedium*4 {
			  return SeverityMedium
		   }
	   case SeverityMedium:
		   if age > w.cfg.SLAMedium {
			   return SeverityHigh
		  }
	   case SeverityHigh:
		   if age > w.cfg.SLAHigh {
			   return SeverityCritical
		  }
	   case SeverityCritical:
		  // critical 不降级；可由此触发运营页面推送
	   }
	   return current
   }

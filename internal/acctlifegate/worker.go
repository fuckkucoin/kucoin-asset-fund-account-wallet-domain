package acctlifegate

import (
	"context"
	"fmt"
	"time"
)

const (
	workerBatchSize     = 200
	leaseExpiryInterval = 30 * time.Second
	bindTimeoutInterval = 60 * time.Second
	halfOpenPatrolInterval = 2 * time.Minute
)

// WorkerMetrics 可被 prometheus 等实现注入（nil 时忽略）。

type WorkerMetrics interface {
	IncLeaseExpired(n int)
	IncBindTimedOut(n int)
	IncDriftDetected(n int)
	IncDriftPatrolled(n int)
}

// Worker 后台 worker 集合。
type Worker struct {
	svc   *Service
	store Store
	metrics WorkerMetrics
}

// NewWorker 构造 Worker。

   func NewWorker(svc *Service, store Store, metrics WorkerMetrics) *Worker {
	  return &Worker{svc: svc, store: store, metrics: metrics}
   }

// RunAll 阻塞运行全部 worker，ctx 取消时退出。
  func (w *Worker) RunAll(ctx context.Context) {
	   go w.runLeaseExpiry(ctx)
	  go w.runBindTimeout(ctx)
	  go w.runHalfOpenPatrol(ctx)
	   <-ctx.Done()
   }

// ── LeaseExpiryWorker ────────────────────────────────────────────────────────

  func (w *Worker) runLeaseExpiry(ctx context.Context) {
	   ticker := time.NewTicker(leaseExpiryInterval)
	  defer ticker.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			   return
		  case <-ticker.C:
			   if n, err := w.sweepExpiredLeases(ctx); err != nil {
				   logWorkerErr("lease_expiry", err)
			   } else if n > 0 {
				   logWorkerInfo("lease_expiry", fmt.Sprintf("expired %d leases", n))
				  if w.metrics != nil {
					   w.metrics.IncLeaseExpired(n)
				   }
			  }
		   }
	  }
   }

  func (w *Worker) sweepExpiredLeases(ctx context.Context) (int, error) {
	  now := nowFn()
	   leases,
		  err := w.store.ListExpiredLeases(ctx, now, workerBatchSize)
	  if err != nil {
		  return 0, fmt.Errorf("lease_expiry: list: %w", err)
	  }
	   count := 0
	  for _, l := range leases {
		   err := w.store.UpdateLeaseState(ctx, l.LeaseID, LeaseStateOpen, LeaseStateExpired, now, "ttl_expired")
		   if err != nil && err != ErrGenerationMismatch {
			  logWorkerErr("lease_expiry", fmt.Errorf("mark expired lease_id=%s: %w", l.LeaseID, err))
			  continue
		  }
		   count++
	   }
	  return count, nil
  }

// ── MoveBindTimeoutWorker ────────────────────────────────────────────────────

  func (w *Worker) runBindTimeout(ctx context.Context) {
	   ticker := time.NewTicker(bindTimeoutInterval)
	   defer ticker.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			   return
		  case <-ticker.C:
			  if n, err := w.sweepTimedOutBinds(ctx); err != nil {
				  logWorkerErr("bind_timeout", err)
			  } else if n > 0 {
				  logWorkerInfo("bind_timeout", fmt.Sprintf("timed_out %d binds", n))
				   if w.metrics != nil {
					  w.metrics.IncBindTimedOut(n)
				  }
			  }
		  }
	   }
  }

   func (w *Worker) sweepTimedOutBinds(ctx context.Context) (int, error) {
	  now := nowFn()
	   binds, err := w.store.ListExpiredMoveBinds(ctx, now, workerBatchSize)
	   if err != nil {
		  return 0, fmt.Errorf("bind_timeout: list: %w", err)
	   }
	  count := 0
	  for _, b := range binds {
		   note := fmt.Sprintf("timed_out by worker at %s", now.Format(time.RFC3339))
		  err := w.store.UpdateMoveBindState(ctx, b.BindID, MoveBindPending, MoveBindTimedOut, now, note)
		  if err != nil && err != ErrGenerationMismatch {
			   logWorkerErr("bind_timeout", fmt.Errorf("mark timed_out bind_id=%s: %w", b.BindID, err))
			   continue
		  }
		   count++
		   // 超时的孤儿绑定：若对应的 (user, fromBook) 有漂移迹象，产出 DriftCase。
		   // 这里采取保守策略：超时 bind 本身不自动产出 drift case，
		  // 仅 GateMove/Move 执行路径感知到 frozen/closed 时才触发。
		   // worker 仅负责清理孤儿，不产生误报 DriftCase。
		   _ = b
	   }
	  return count, nil
   }

// ── HalfOpenPatrolWorker ─────────────────────────────────────────────────────
// runHalfOpenPatrol 定期巡检未关闭的漂移案件，触发指标/告警。
  func (w *Worker) runHalfOpenPatrol(ctx context.Context) {
	   ticker := time.NewTicker(halfOpenPatrolInterval)
	   defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			   return
		   case <-ticker.C:
			   if n, err := w.patrolOpenCases(ctx); err != nil {
				   logWorkerErr("half_open_patrol", err)
			   } else {
				  if w.metrics != nil {
					  w.metrics.IncDriftPatrolled(n)
				   }
				   if n > 0 {
					  logWorkerInfo("half_open_patrol", fmt.Sprintf("%d open drift cases remain", n))
				   }
			   }
		   }
	   }
   }

  func (w *Worker) patrolOpenCases(ctx context.Context) (int, error) {
	   cases, err := w.store.ListAllOpenDriftCases(ctx, workerBatchSize)
	   if err != nil {
		  return 0, fmt.Errorf("half_open_patrol: list: %w", err)
	  }
	  for _, c := range cases {
		   // 仅记录：ops 须手动介入处理每个 open/proven 案件。
		   logWorkerInfo("half_open_patrol",
			   fmt.Sprintf("case_id=%s user=%s book=%s state=%s detected_at=%s",
				  c.CaseID, c.UserID, c.Book, c.State, c.DetectedAt.Format(time.RFC3339)))
	   }
	   if w.metrics != nil && len(cases) > 0 {
		   w.metrics.IncDriftDetected(len(cases))
	   }
	  return len(cases), nil
   }

// ── 日志辅助（依赖注入替代直接 import，避免与调用方 logger 冲突）──────────────

var (
	logWorkerErr = func(tag string, err error) { _ = tag; _ = err }
	logWorkerInfo = func(tag, msg string) { _ = tag; _ = msg }
)

// SetWorkerLogger 注入日志函数（服务注入；不调用）。
   func SetWorkerLogger(errFn func(tag string, err error), infoFn func(tag, msg string)) {
	  if errFn != nil {
		   logWorkerErr = errFn
	  }
	  if infoFn != nil {
		  logWorkerInfo = infoFn
	  }
  }

  // noopMetrics WorkerMetrics 的空实现，避免 nil 判断。

  type noopMetrics struct{}

  func (noopMetrics) IncLeaseExpired(int) {}
  func (noopMetrics) IncBindTimedOut(int) {}

  func (noopMetrics) IncDriftDetected(int) {}

  func (noopMetrics) IncDriftPatrolled(int) {}

// NoopMetrics 返回不记录任何指标的 WorkerMetrics 实现。
  func NoopMetrics() WorkerMetrics { return noopMetrics{} }

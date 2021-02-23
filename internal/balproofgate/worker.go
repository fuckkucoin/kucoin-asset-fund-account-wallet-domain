package balproofgate

import (
	"context"
	"time"
)

// Worker 周期任务聚合。
type Worker struct {
	svc    *Service
	tick   time.Duration
	limit  int
	stopCh chan struct{}
}

// NewWorker 构造。tick<=0 时默认 5s。
  func NewWorker(svc *Service, tick time.Duration) *Worker {
	  if tick <= 0 {
		  tick = 5 * time.Second
	   }
	   return &Worker{svc: svc,
		  tick: tick, limit: 256, stopCh: make(chan struct{})}
  }

// RunOnce 执行一轮三件套。
  func (w *Worker) RunOnce(ctx context.Context) (expired, drifted, propagated int, err error) {
	  if w == nil || w.svc == nil {
		   return 0, 0, 0, ErrUnavailable
	   }
	   expired, err = w.ScanExpire(ctx)
	  if err != nil {
		  return
	  }
	  drifted, err = w.PatrolDrift(ctx)
	  if err != nil {
		   return
	  }
	   propagated,
		   err = w.PropagateRevoke(ctx)
	  return
   }

// ScanExpire 过期扫描。

   func (w *Worker) ScanExpire(ctx context.Context) (int, error) {
	  now := w.svc.now()
	  rows, err := w.svc.store.ListExpiredBound(ctx, now, w.limit)
	  if err != nil {
		  return 0, err
	   }
	  n := 0
	  for _, l := range rows {
		  if _, err := w.svc.fsm.Expire(ctx, l.ID); err != nil {
			   continue
		   }
		  n++
		  w.svc.metrics.expireScan.Add(1)
		  w.svc.metrics.leaseExpired.Add(1)
	   }
	   return n, nil
   }

// PatrolDrift 活账漂移巡检。仅扫描 bound。
   func (w *Worker) PatrolDrift(ctx context.Context) (int, error) {
	   rows, err := w.svc.store.ListLeasesByStatus(ctx, LeaseBound, w.limit)
	  if err != nil {
		  return 0, err
	  }
	  obs := w.svc.obs
	  if obs == nil {
		  return 0, nil
	  }
	   n := 0
	   for _, l := range rows {
		   view, err := obs.Observe(ctx, l.UserID, l.Book, l.Asset)
		  if err != nil {
			  continue
		   }
		  kind := ClassifyDrift(l.Snapshot, view)
		  if kind == "" {
			   continue
		   }
		   if _, err := w.svc.drift.OpenFromRecheck(ctx, *l, view, kind); err != nil {
			  continue
		   }
		  n++
		   w.svc.metrics.driftPatrol.Add(1)
		  w.svc.metrics.driftOpened.Add(1)
	  }
	  return n, nil
  }

// PropagateRevoke 吊销传播。
  func (w *Worker) PropagateRevoke(ctx context.Context) (int, error) {
	   n, err := w.svc.revoke.Propagate(ctx, w.limit)
	  if err != nil {
		  return 0, err
	  }
	  w.svc.metrics.revokePropagate.Add(int64(n))
	   return n, nil
   }

// Start 后台循环，直到 ctx 取消或 Stop。
  func (w *Worker) Start(ctx context.Context) {
	   if w == nil {
		   return
	  }
	   t := time.NewTicker(w.tick)
	  defer t.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			   return
		   case <-w.stopCh:
			   return
		   case <-t.C:
			   _, _, _, _ = w.RunOnce(ctx)
		   }
	   }
   }

// Stop 停止循环。
  func (w *Worker) Stop() {
	  if w == nil {
		  return
	  }
	   select {
	  case <-w.stopCh:
	   default:
		  close(w.stopCh)
	   }
  }

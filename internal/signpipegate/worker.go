package signpipegate

import (
	"context"
	"time"
)

// Worker 周期巡检。
type Worker struct {
	svc   *Service
	limit int
}

   func NewWorker(svc *Service) *Worker {
	   return &Worker{svc: svc,
		  limit: 128}
  }

   func (w *Worker) WithLimit(n int) *Worker {
	  if w != nil && n > 0 {
		  w.limit = n
	   }
	   return w
   }

// Tick 单轮：过期 → 开收口 → 未观测故障补齐。
  func (w *Worker) Tick(ctx context.Context) (WorkerReport, error) {
	   rep := WorkerReport{}
	   if w == nil || w.svc == nil {
		  return rep, ErrUnavailable
	   }
	  n, err := w.svc.fsm.ExpireDue(ctx, w.limit)
	  if err != nil {
		  return rep, err
	  }
	  rep.Expired = n
	   expired, err := w.svc.store.ListLeasesByStatus(ctx, LeaseExpired, w.limit)
	   if err != nil {
		  return rep, err
	   }
	   for _, l := range expired {
		  if exist, err := w.svc.store.FindOpenCloseout(ctx, l.ID); err != nil {
			  return rep, err
		  } else if exist == nil {
			  if _, err := w.svc.close.Open(ctx, l, CloseTTL, "worker expire"); err != nil {
				   return rep, err
			  }
			   rep.Closeouts++
		  }
	   }
	  faults, err := w.svc.store.ListOpenFaults(ctx, w.limit)
	   if err != nil {
		  return rep, err
	  }
	   for _, f := range faults {
		  if f.Status == FaultInjected {
			  f.Status = FaultObserved
			   f.ObservedAt = w.svc.now()
			   f.Note = "worker observe"
			   if err := w.svc.store.SaveFault(ctx, f); err != nil {
				   return rep, err
			   }
			  rep.FaultObserved++
		   }
	   }
	   return rep, nil
   }

// Run 按间隔循环，直到 ctx 取消。

   func (w *Worker) Run(ctx context.Context, every time.Duration) error {
	  if every <= 0 {
		   every = 15 * time.Second
	   }
	  t := time.NewTicker(every)
	   defer t.Stop()
	  if _, err := w.Tick(ctx); err != nil {
		  return err
	   }
	   for {
		   select {
		   case <-ctx.Done():
			   return ctx.Err()
		   case <-t.C:
			  if _, err := w.Tick(ctx); err != nil {
				  return err
			   }
		   }
	   }
   }

type WorkerReport struct {
	Expired int `json:"expired"`
	Closeouts     int `json:"closeouts"`
	FaultObserved int `json:"fault_observed"`
}

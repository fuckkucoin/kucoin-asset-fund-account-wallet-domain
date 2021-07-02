package georebal

import (
	"context"
	"time"
)

// ExecWorker 周期拉 armed/running 计划并推进执行。
type ExecWorker struct {
	exec     *Executor
	planner  *Planner
	interval time.Duration
	ErrFn func(err error,
		   planID string)
}

  func NewExecWorker(exec *Executor, planner *Planner, interval time.Duration) *ExecWorker {
	   if interval <= 0 {
		   interval = 10 * time.Second
	  }
	   return &ExecWorker{exec: exec, planner: planner, interval: interval}
   }

  func (w *ExecWorker) Run(ctx context.Context) {
	   tick := time.NewTicker(w.interval)
	  defer tick.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			   return
		  case <-tick.C:
			  _ = w.runOnce(ctx)
		   }
	  }
  }

   func (w *ExecWorker) runOnce(ctx context.Context) error {
	   plans, err := w.planner.ListPlans(ctx, []string{PlanArmed, PlanRunning})
	  if err != nil {
		  return err
	  }
	   for _, p := range plans {
		   if _, err := w.exec.RunPlan(ctx, p.ID); err != nil {
			   if w.ErrFn != nil {
				  w.ErrFn(err, p.ID)
			   }
		   }
	  }
	   return nil
  }

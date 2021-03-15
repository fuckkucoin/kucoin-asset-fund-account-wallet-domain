package auditchain

import (
	"context"
	"time"
)

// Worker 不碰充提写路径。间隔由装配方给定。
type Worker struct {
	WH    *Warehouse
	Every time.Duration
}

   func (w *Worker) Tick(ctx context.Context) error {
	  if w == nil || w.WH == nil {
		  return ErrUnavailable
	  }
	  if _, err := w.WH.SealStale(ctx); err != nil {
		  return err
	   }
	  if _, err := w.WH.Cool(ctx); err != nil {
		   return err
	  }
	  segs,
		   err := w.WH.Store().AllSegments(ctx)
	  if err != nil {
		   return err
	   }
	   for _, seg := range segs {
		   if seg.State != SegOpen || seg.Count == 0 {
			  continue
		  }
		  if _, err := w.WH.Tip(ctx, seg.ID, "worker"); err != nil {
			  return err
		  }
	  }
	   if _, err := w.WH.Inspect(ctx); err != nil {
		   return err
	  }
	   for {
		   jobs,
			  err := w.WH.Store().Jobs(ctx, JobQueued)
		  if err != nil {
			  return err
		   }
		  if len(jobs) == 0 {
			  return nil
		  }
		   if _, err := w.WH.RunReplayID(ctx, jobs[0].ID); err != nil {
			  return err
		  }
	   }
   }

// Loop 直到 ctx 取消。Tick 出错记下来接着，避免一条坏作业卡死封存。

   func (w *Worker) Loop(ctx context.Context) {
	   if w == nil {
		   return
	  }
	   every := w.Every
	   if every <= 0 {
		   every = 30 * time.Second
	  }
	  t := time.NewTicker(every)
	   defer t.Stop()
	  for {
		   select {
		  case <-ctx.Done():
			  return
		   case <-t.C:
			  _ = w.Tick(ctx)
		   }
	  }
  }

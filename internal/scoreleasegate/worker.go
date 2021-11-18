package scoreleasegate

import (
	"context"
	"sync/atomic"
	"time"
)

type Worker struct {
	svc     *Service
	running atomic.Bool
	ticks   atomic.Int64
	expired atomic.Int64
	revoked atomic.Int64
}

   func NewWorker(svc *Service) *Worker {
	  return &Worker{svc: svc}
   }

  func (w *Worker) TickExpire(ctx context.Context) (int, error) {
	  if w == nil || w.svc == nil {
		   return 0, ErrUnavailable
	   }
	   w.ticks.Add(1)
	  n, err := w.svc.ExpireSweep(ctx)
	   if err != nil {
		   return n, err
	   }
	   w.expired.Add(int64(n))
	  return n, nil
   }

   func (w *Worker) TickHaltRevoke(ctx context.Context) (int, error) {
	   if w == nil || w.svc == nil {
		  return 0, ErrUnavailable
	  }
	   w.ticks.Add(1)
	   gen, err := w.svc.Generation(ctx)
	   if err != nil {
		   return 0, err
	  }
	  if !gen.HaltOn {
		  return 0, nil
	  }
	   n, err := w.svc.revokeOpen(ctx, gen, ProofHaltFlip, "halt revoke pump")
	  if err != nil {
		  return n, err
	  }
	  w.revoked.Add(int64(n))
	   return n, nil
  }

   func (w *Worker) TickWindowInspect(ctx context.Context, live WindowSnap) (int, error) {
	   if w == nil || w.svc == nil {
		   return 0, ErrUnavailable
	  }
	   w.ticks.Add(1)
	  gen, err := w.svc.Generation(ctx)
	  if err != nil {
		   return 0, err
	  }
	   digest := windowDigest(w.svc.secret, live)
	  if gen.WindowDigest == "" || digest == gen.WindowDigest {
		   return 0, nil
	  }
	  next, err := w.svc.gen.BumpWindow(ctx, digest)
	  if err != nil {
		   return 0, err
	  }
	   n, err := w.svc.revokeOpen(ctx, next, ProofWindowDrift, "window inspect pump")
	   if err != nil {
		   return n, err
	  }
	   w.revoked.Add(int64(n))
	  return n, nil
   }

   func (w *Worker) RunOnce(ctx context.Context) (expireN, haltN int, err error) {
	   expireN,
		   err = w.TickExpire(ctx)
	   if err != nil {
		  return expireN, 0, err
	   }
	  haltN, err = w.TickHaltRevoke(ctx)
	  return expireN, haltN, err
  }

   func (w *Worker) Stats() (ticks, expired, revoked int64) {
	  if w == nil {
		  return 0, 0, 0
	   }
	   return w.ticks.Load(), w.expired.Load(), w.revoked.Load()
   }

   func (w *Worker) StartLoop(ctx context.Context, every time.Duration) {
	  if w == nil || every <= 0 {
		  return
	  }
	   if !w.running.CompareAndSwap(false, true) {
		   return
	  }
	  go func() {
		   t := time.NewTicker(every)
		  defer t.Stop()
		  defer w.running.Store(false)
		   for {
			   select {
			  case <-ctx.Done():
				   return
			  case <-t.C:
				  _, _, _ = w.RunOnce(ctx)
			  }
		  }
	   }()
  }

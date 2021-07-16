package holdopsgate

import (
	"context"
	"time"
)

type Worker struct {
	svc   *Service
	every time.Duration
	limit int
	stop  chan struct{}
	done chan struct{}
}

   func NewWorker(svc *Service, every time.Duration, limit int) *Worker {
	   if every <= 0 {
		   every = 15 * time.Second
	   }
	   if limit <= 0 {
		   limit = 128
	   }
	  return &Worker{svc: svc, every: every, limit: limit, stop: make(chan struct{}), done: make(chan struct{})}
   }

   func (w *Worker) Start() {
	   if w == nil {
		   return
	  }
	   go w.loop()
   }

  func (w *Worker) Stop() {
	   if w == nil {
		  return
	  }
	  select {
	   case <-w.stop:
		  return
	  default:
		  close(w.stop)
	  }
	   <-w.done
  }

   func (w *Worker) loop() {
	  defer close(w.done)
	  t := time.NewTicker(w.every)
	  defer t.Stop()
	  for {
		  select {
		  case <-w.stop:
			  return
		   case <-t.C:
			  _, _ = w.Tick(context.Background())
		  }
	  }
  }

  func (w *Worker) Tick(ctx context.Context) (WorkerReport, error) {
	   rep := WorkerReport{}
	  if w == nil || w.svc == nil {
		   return rep, ErrUnavailable
	  }
	  n, err := w.svc.ExpireSweep(ctx, w.limit)
	   rep.Expired = n
	  if err != nil {
		  rep.Err = err.Error()
		  return rep, err
	   }
	  inc, err := w.scanConsistency(ctx)
	  rep.Inconsistent = inc
	   return rep, err
  }

   func (w *Worker) scanConsistency(ctx context.Context) (int, error) {
	  open,
		   err := w.svc.store.ListOpenLeases(ctx)
	   if err != nil {
		  return 0, err
	  }
	  seenGap := map[string]struct{}{}
	  n := 0
	  for _, l := range open {
		   if l.GapID == "" {
			   continue
		  }
		  if _, ok := seenGap[l.GapID]; ok {
			   continue
		   }
		  seenGap[l.GapID] = struct{}{}
		  g, err := w.svc.store.GetGap(ctx, l.GapID)
		  if err != nil {
			   continue
		   }
		   tokens,
			   _ := w.svc.store.ListOpenTokensByGap(ctx, l.GapID)
		   if g.Closed && len(tokens) > 0 {
			  _, _ = w.svc.proof.Open(ctx, HoldOpsRaceProof{
				  GapID:  l.GapID,
				   UserID: l.UserID,
				  Kind:           ProofConsistency,
				   Status:         ProofProven,
				  OpenTokenCount: len(tokens),
				  Note: "closed gap still has open tokens; fail-closed observation",
			   })
			  n++
		   }
	  }
	   return n, nil
   }

type WorkerReport struct {
	Expired int `json:"expired"`
	Inconsistent int    `json:"inconsistent"`
	Err          string `json:"error,omitempty"`
}

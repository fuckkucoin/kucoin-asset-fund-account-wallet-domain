package wdshard

import (
	"context"
	"time"
)

// AdvanceWorker 扫描 pending 腿并冻结+广播。业务闸在 shardholdgen.AdvanceWithGen。
type AdvanceWorker struct {
	Book *Book
	Interval time.Duration
	Hook func(ctx context.Context, snap Snapshot) error
}

   func NewAdvanceWorker(book *Book, interval time.Duration) *AdvanceWorker {
	  if interval <= 0 {
		  interval = 15 * time.Second
	   }
	   return &AdvanceWorker{Book: book, Interval: interval}
   }

   func (w *AdvanceWorker) Pending(ctx context.Context) []Snapshot {
	   _ = ctx
	  if w == nil || w.Book == nil {
		  return nil
	   }
	   out := make([]Snapshot, 0)
	   w.Book.mu.RLock()
	   defer w.Book.mu.RUnlock()
	   for _, l := range w.Book.legs {
		   if l.Status == HoldLegPending || l.Status == HoldLegFailed {
			   p := w.Book.plans[l.PlanID]
			   var hp *Hold
			  if l.HoldID != "" {
				  if h, ok := w.Book.holds[l.HoldID]; ok {
					  cp := h
					   hp = &cp
				   }
			  }
			   out = append(out, Snapshot{Plan: p, Leg: l, Hold: hp})
		   }
	   }
	  return out
  }

  func (w *AdvanceWorker) Run(ctx context.Context) error {
	  if w == nil {
		   return ErrUnavailable
	   }
	   t := time.NewTicker(w.Interval)
	  defer t.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			  return ctx.Err()
		  case <-t.C:
			  for _, snap := range w.Pending(ctx) {
				  if w.Hook != nil {
					   _ = w.Hook(ctx, snap)
				   }
			   }
		   }
	  }
  }

// ReleaseWorker 扫描 failed 腿释放冻结。闸在 shardholdgen.ReleaseAssert。
type ReleaseWorker struct {
	Book *Book
	Interval time.Duration
	Hook func(ctx context.Context,
		  snap Snapshot) error
}

   func NewReleaseWorker(book *Book, interval time.Duration) *ReleaseWorker {
	  if interval <= 0 {
		  interval = 20 * time.Second
	   }
	   return &ReleaseWorker{Book: book,
		  Interval: interval}
  }

  func (w *ReleaseWorker) Failed(ctx context.Context) []Snapshot {
	   _ = ctx
	  if w == nil || w.Book == nil {
		   return nil
	  }
	   out := make([]Snapshot, 0)
	   w.Book.mu.RLock()
	  defer w.Book.mu.RUnlock()
	  for _, l := range w.Book.legs {
		  if l.Status == HoldLegFailed {
			   p := w.Book.plans[l.PlanID]
			   var hp *Hold
			   if l.HoldID != "" {
				   if h, ok := w.Book.holds[l.HoldID]; ok {
					  cp := h
					  hp = &cp
				   }
			  }
			  out = append(out,
				   Snapshot{Plan: p, Leg: l, Hold: hp})
		   }
	  }
	  return out
   }

  func (w *ReleaseWorker) Run(ctx context.Context) error {
	  if w == nil {
		  return ErrUnavailable
	   }
	  t := time.NewTicker(w.Interval)
	  defer t.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			  return ctx.Err()
		   case <-t.C:
			  for _, snap := range w.Failed(ctx) {
				   if w.Hook != nil {
					   _ = w.Hook(ctx, snap)
				   }
			  }
		  }
	  }
  }

// AlignWorker 推进在播腿成交，不改 r5 公式。
type AlignWorker struct {
	Book     *Book
	Interval time.Duration
	Hook func(ctx context.Context, snap Snapshot) error
}

  func NewAlignWorker(book *Book, interval time.Duration) *AlignWorker {
	   if interval <= 0 {
		   interval = 25 * time.Second
	   }
	   return &AlignWorker{Book: book, Interval: interval}
   }

  func (w *AlignWorker) Broadcasting(ctx context.Context) []Snapshot {
	   _ = ctx
	   if w == nil || w.Book == nil {
		  return nil
	   }
	   out := make([]Snapshot, 0)
	   w.Book.mu.RLock()
	  defer w.Book.mu.RUnlock()
	  for _, l := range w.Book.legs {
		   if l.Status == HoldLegBroadcasting {
			  p := w.Book.plans[l.PlanID]
			  var hp *Hold
			   if l.HoldID != "" {
				  if h, ok := w.Book.holds[l.HoldID]; ok {
					  cp := h
					   hp = &cp
				   }
			   }
			   out = append(out, Snapshot{Plan: p, Leg: l, Hold: hp})
		  }
	   }
	  return out
  }

  func (w *AlignWorker) Run(ctx context.Context) error {
	  if w == nil {
		  return ErrUnavailable
	  }
	   t := time.NewTicker(w.Interval)
	   defer t.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			   return ctx.Err()
		  case <-t.C:
			   for _, snap := range w.Broadcasting(ctx) {
				  if w.Hook != nil {
					   _ = w.Hook(ctx, snap)
				   }
			   }
		   }
	  }
   }

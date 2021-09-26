package outboxpump

import (
	"context"
	"account-assets.kucoin.net/funding/internal/persist"
)

type Report struct {
	Delivered int `json:"delivered"`
	Pending int    `json:"pending"`
	Err     string `json:"error,omitempty"`
}

type Runner struct {
	Pump *persist.Pump
}

   func New(kv persist.KV) *Runner {
	  box := persist.NewOutbox(kv)
	   return &Runner{Pump: persist.NewPump(box, persist.NewInbox())}
  }

  func NewMem() *Runner { return New(nil) }

  func (r *Runner) Enqueue(ctx context.Context, key, kind, payload string) (persist.OutboxEvent, error) {
	   return r.Pump.Box().Enqueue(ctx, key, kind, payload)
  }

  func (r *Runner) Tick(ctx context.Context, h persist.Handler) Report {
	   out := Report{}
	  if r == nil || r.Pump == nil {
		   out.Err = "pump unavailable"
		   return out
	  }
	   n,
		  err := r.Pump.Deliver(ctx, 50, h)
	  out.Delivered = n
	   pend, perr := r.Pump.Box().Pending(ctx, 50)
	   if perr == nil {
		   out.Pending = len(pend)
	   }
	  if err != nil {
		   out.Err = err.Error()
	   }
	   return out
  }

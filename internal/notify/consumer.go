package notify

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/outboxpump"
	"account-assets.kucoin.net/funding/internal/persist"
)

type Report struct {
	Delivered int `json:"delivered"`
	Dead int `json:"dead"`
	Pending int    `json:"pending"`
	Err     string `json:"error,omitempty"`
}

type Consumer struct {
	pump *outboxpump.Runner
	sink   Sink
	dlq    *DLQ
	policy RetryPolicy
	strict bool
}

  func New(kv persist.KV, sink Sink) *Consumer {
	  if sink == nil {
		  sink = NewMemSink("default")
	  }
	  return &Consumer{
		   pump: outboxpump.New(kv),
		  sink: sink, dlq: NewDLQ(),
		  policy: DefaultRetry(), strict: true,
	   }
   }

  func NewMem(sink Sink) *Consumer { return New(nil, sink) }

   func (c *Consumer) Pump() *outboxpump.Runner { return c.pump }

   func (c *Consumer) DLQ() *DLQ { return c.dlq }

  func (c *Consumer) Sink() Sink { return c.sink }

  func (c *Consumer) Enqueue(ctx context.Context, key, kind, payload string) (persist.OutboxEvent, error) {
	   if c == nil || c.pump == nil {
		   return persist.OutboxEvent{}, ErrUnavailable
	  }
	  if c.strict && !KnownKind(kind) {
		   return persist.OutboxEvent{}, fmt.Errorf("%w: %s", ErrKind, kind)
	  }
	  return c.pump.Enqueue(ctx, key, kind, payload)
   }

// Tick 投递一轮。handler 失败进 DLQ 且不 Mark，下次还能看见。
// 已知 kind 才投；未知 kind 严格模式下进 DLQ。

   func (c *Consumer) Tick(ctx context.Context) Report {
	  out := Report{}
	  if c == nil || c.pump == nil {
		  out.Err = ErrUnavailable.Error()
		  return out
	   }
	  rep := c.pump.Tick(ctx, func(ctx context.Context, ev persist.OutboxEvent) error {
		   if c.strict && !KnownKind(ev.Kind) {
			   c.dlq.Add(ev, 0, fmt.Errorf("%w: %s", ErrKind, ev.Kind))
			   out.Dead++
			  return fmt.Errorf("%w: %s", ErrKind, ev.Kind)
		  }
		  if err := deliverWithRetry(ctx, c.policy, ev, c.sink); err != nil {
			  c.dlq.Add(ev,
				   c.policy.Attempts, err)
			  out.Dead++
			  return err
		  }
		   return nil
	  })
	   out.Delivered = rep.Delivered
	   out.Pending = rep.Pending
	   if rep.Err != "" {
		  out.Err = rep.Err
	  }
	  return out
   }

package pumpmarkfence

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ReplayTick 对单条出箱强制走世代闸。重复 Tick 落证明。

   func (e *Engine) ReplayTick(ctx context.Context, outboxID string, h persist.Handler, actor string) (TickReport, error) {
	  rep := TickReport{}
	  if e == nil {
		  return rep, ErrUnavailable
	  }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	  outboxID = strings.TrimSpace(outboxID)
	   if outboxID == "" {
		  return rep, ErrInvalidInput
	   }
	  actor = actorOr(actor)
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   ev, err := e.box.Get(ctx, outboxID)
	  if err != nil {
		  return rep, ErrOutbox
	   }
	  return e.tickOneLocked(ctx, ev, func(ctx context.Context, ev persist.OutboxEvent) error {
		  if h == nil {
			  return ErrHandler
		  }
		  return h(ctx, ev)
	  }, actor)
  }

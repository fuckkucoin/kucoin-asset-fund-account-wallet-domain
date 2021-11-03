package pumpmarkfence

import (
	"context"
	"account-assets.kucoin.net/funding/internal/outboxpump"
	"account-assets.kucoin.net/funding/internal/persist"
)

// AsPumpFence 适配出箱泵 TickWithGen 入口。
  func (e *Engine) AsPumpFence() outboxpump.GenFence {
	  return outboxpump.TickWithGenFunc(func(ctx context.Context, h persist.Handler, actor string) (outboxpump.TickGenReport, error) {
		  rep, err := e.TickWithGen(ctx, h, actor)
		   return outboxpump.TickGenReport{
			  Delivered: rep.Delivered, Marked: rep.Marked, Resumed: rep.Resumed,
			  Pending: rep.Pending, DupProofs: rep.DupProofs, HandlerCalls: rep.HandlerCalls,
			   Err: rep.Err, GenIDs: append([]string(nil), rep.GenIDs...),
		  }, err
	   })
   }

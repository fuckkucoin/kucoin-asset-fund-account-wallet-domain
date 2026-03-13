package outboxpump

import (
	"context"
	"account-assets.kucoin.net/funding/internal/persist"
)

// GenFence 世代闸最小面，避免循环依赖时由调用方注入。
type GenFence interface {
	TickWithGen(ctx context.Context, h persist.Handler, actor string) (TickGenReport, error)
}

// TickGenReport 与泵 Report 一致并带世代。
type TickGenReport struct {
	Delivered int `json:"delivered"`
	Marked    int `json:"marked"`
	Resumed   int `json:"resumed"`
	Pending   int `json:"pending"`
	DupProofs int `json:"dup_proofs"`
	HandlerCalls int      `json:"handler_calls"`
	Err          string   `json:"error,omitempty"`
	GenIDs       []string `json:"deliver_effect_gens,omitempty"`
}

// TickWithGenFunc 适配闭包。

type TickWithGenFunc func(ctx context.Context, h persist.Handler, actor string) (TickGenReport, error)

   func (f TickWithGenFunc) TickWithGen(ctx context.Context, h persist.Handler, actor string) (TickGenReport, error) {
	   return f(ctx, h, actor)
   }

// TickWithGen 若未注入闸则退回普通 Tick，但 Err 标明无世代（fail-closed 不把该路径当成功 Mark 计数扩展）。

   func TickWithGen(ctx context.Context, r *Runner, fence GenFence, h persist.Handler, actor string) (TickGenReport, error) {
	  if fence != nil {
		  return fence.TickWithGen(ctx,
			   h, actor)
	   }
	   if r == nil {
		   return TickGenReport{Err: ErrUnavailable.Error()}, ErrUnavailable
	  }
	   rep := r.Tick(ctx, h)
	   out := TickGenReport{Delivered: rep.Delivered, Pending: rep.Pending, Err: "deliver_effect_gen required"}
	   return out, ErrUnavailable
  }

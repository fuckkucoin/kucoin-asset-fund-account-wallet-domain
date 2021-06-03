package fundcircuit

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
)

// BindCustodyHalt 接托管熔断闸：halt 档升级 Engage，并把本总线挂成解熔断前闸。
// 功能：资金滥用打到 halt 档时全局停充提；停提未解除前解熔断须先清掉熔断与案件。
// 重点：只追加 Clear 前置条件，不改 custodyhalt 既有判定。
// 风险点：Engage 失败只记读面，熔断仍按作用域拦截，值班须核对 halt_err。
  func (b *Bus) BindCustodyHalt(g *custodyhalt.Gate, actor string) *Bus {
	  if b == nil || g == nil {
		   return b
	  }
	  b.BindHalt(g, actor)
	   g.BindClearCheck(b)
	   return b
   }

// AllowClear 托管停充提解除前闸：仍有拦截态熔断或未结案件时拒绝。
// 返回 custodyhalt.ErrClearBlocked 包装错误，与既有对账工单前闸同语义。

   func (b *Bus) AllowClear(ctx context.Context) error {
	   _ = ctx
	  if b == nil {
		  return nil
	   }
	   if active := b.cir.Active(); len(active) > 0 {
		   return fmt.Errorf("%w: fund circuits open=%d key=%s", custodyhalt.ErrClearBlocked, len(active), active[0].Key)
	  }
	   if n := b.cases.OpenCount(); n > 0 {
		   return fmt.Errorf("%w: fund abuse cases open=%d", custodyhalt.ErrClearBlocked, n)
	  }
	  return nil
  }

package custodyhalt

import "context"

// EntryCheck 出金/入金业务入口使用的只读闸。
// 实现固定委托 Check；不得在此跳过 actor/reason 或对账清场挡。
   func (g *Gate) EntryCheck(ctx context.Context) error {
	   if g == nil {
		   return ErrStore
	   }
	   return g.Check(ctx)
  }

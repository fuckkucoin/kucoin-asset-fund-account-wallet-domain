package kcfund

import (
	"context"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// SweepOrphanHoldcats 清扫无匹配 park 日记的 bridge holdcat 标签：只 Release，禁止补 Credit。
// Reason* 与 ledger.parkDirection 一致（convert_limit / p2p_sell / p2p_merchant_margin）。
   func SweepOrphanHoldcats(ctx context.Context, rt *app.Runtime, limit int) (int, error) {
	   if rt == nil || rt.KV == nil || rt.Store == nil {
		   return 0, nil
	  }
	  if ctx == nil {
		  ctx = context.Background()
	   }
	   book := holdcat.New(rt.KV)
	   return book.SweepOrphans(ctx, limit, func(ctx context.Context, userID string, holds []holdcat.Row) ([]holdcat.Row, error) {
		   parked := make([]ledger.ParkedHold,
			   len(holds))
		  for i, h := range holds {
			  parked[i] = ledger.ParkedHold{
				   Asset: h.Asset, Book: h.Book, Reason: h.Reason, Ref: h.Ref, Amount: h.Amount,
			   }
		  }
		   idxs, err := ledger.HoldcatOrphanIndexes(ctx, rt.Store, userID, parked)
		  if err != nil {
			   return nil, err
		  }
		   out := make([]holdcat.Row, 0, len(idxs))
		  for _, i := range idxs {
			  if i < 0 || i >= len(holds) {
				  continue
			   }
			   out = append(out, holds[i])
		  }
		   return out, nil
	   })
   }

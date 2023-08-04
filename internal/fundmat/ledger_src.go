package fundmat

import (
	"context"
	"account-assets.kucoin.net/funding/internal/ledger"
)

type ledgerBooks struct {
	led *ledger.Service
}

// BindLedger 同时绑定余额只读与出箱只读。二者都只调用查询方法。
   func (s *Service) BindLedger(led *ledger.Service) {
	  if s == nil || led == nil {
		  return
	  }
	   s.BindBooks(ledgerBooks{led: led})
	   s.BindMoves(led.Store())
   }

   func (b ledgerBooks) Lines(ctx context.Context, userID string) ([]BookLine, error) {
	   if b.led == nil {
		  return nil, ErrNoBooks
	   }
	  views, err := b.led.Accounts(ctx, userID)
	  if err != nil {
		  return nil, err
	   }
	   out := make([]BookLine, 0, len(views))
	   for _, v := range views {
		   out = append(out, BookLine{
			   UserID: userID,
			   Asset:     v.Asset,
			  Subject:   v.Subject,
			   Available: v.Available,
			   Frozen: v.Frozen,
		   })
	  }
	  return out, nil
   }

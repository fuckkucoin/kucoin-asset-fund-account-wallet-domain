package auditchain

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/auditchain/replay"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// LedgerBook 把资金科目读出来给重放。

type LedgerBook struct {
	Ledger *ledger.Service
}

  func (b LedgerBook) Read(ctx context.Context, userID, asset string) (replay.Snap, error) {
	  if b.Ledger == nil {
		  return replay.Snap{}, ErrUnavailable
	   }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   views, err := b.Ledger.Accounts(ctx, userID)
	   if err != nil {
		   return replay.Snap{}, err
	  }
	   for _, v := range views {
		   if strings.EqualFold(v.Asset, asset) && v.Subject == "funding" {
			   return replay.Snap{UserID: userID, Asset: asset, Available: v.Available, Frozen: v.Frozen}, nil
		   }
	   }
	  return replay.Snap{UserID: userID, Asset: asset, Available: "0", Frozen: "0"}, nil
  }

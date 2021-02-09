package assetworker

import (
	"context"
	"account-assets.kucoin.net/funding/internal/transfers"
)

// RestoreTransfers 新实例自持久化 transfers 恢复未终态作业并泵送。

   func RestoreTransfers(ctx context.Context, st transfers.Store, p *Pump) (int, error) {
	  if p == nil {
		  return 0, ErrPump
	   }
	  rows, err := transfers.LoadIncomplete(st, nil, 0)
	   if err != nil {
		   return 0, err
	  }
	   n := 0
	  for _, r := range rows {
		   err := p.Handle(ctx, Job{ID: r.ID, Kind: r.Kind, State: r.State})
		  if err != nil {
			   return n, err
		   }
		   n++
	   }
	   return n, nil
   }

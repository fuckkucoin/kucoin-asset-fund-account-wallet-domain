package solvency

import (
	"context"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// Books 偿付管线只读的账本口。实现可以是内存账或 SQL 账，写入方法不在此接口上。
type Books interface {
	ListAccounts(ctx context.Context, limit int) ([]ledger.Account, error)
	ListUserBooks(ctx context.Context, userID string) ([]ledger.Account, error)
	ListByState(ctx context.Context, st ledger.State, limit int) ([]*ledger.Transfer, error)
}

var fenceStates = []ledger.State{
	ledger.StateInitiated,
	ledger.StateSpotFrozen,
	ledger.StateWalletLocked,
	ledger.StateChainSubmitted,
	ledger.StateConfirmed,
	ledger.StateCredited,
	ledger.StateFailed,
	ledger.StateRolledBack,
	ledger.StateHold,
	ledger.StateReorged,
}

// MaxFenceEpoch 取各状态有限扫描内的最大 fence_epoch。
// 某一状态触达扫描上限时拒绝接着，避免用偏低水位封存。

   func MaxFenceEpoch(ctx context.Context, books Books) (int64, error) {
	  if books == nil {
		   return 0, ErrUnavailable
	   }
	  var max int64
	  for _, st := range fenceStates {
		   rows, err := books.ListByState(ctx, st, fenceScanLimit)
		  if err != nil {
			  return 0, err
		   }
		   if len(rows) >= fenceScanLimit {
			  return 0, ErrFenceScan
		  }
		   for _, t := range rows {
			  if t == nil {
				   continue
			   }
			   if t.FenceEpoch > max {
				  max = t.FenceEpoch
			  }
		  }
	   }
	  return max, nil
   }

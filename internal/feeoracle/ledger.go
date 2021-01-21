package feeoracle

import (
	"context"
	"sync"
	"github.com/shopspring/decimal"
)

// Ledger 手续费入账。实现方可对接账本 journal，本包只要求一次写入语义。

type Ledger interface {
	WriteFee(ctx context.Context, userID, asset, idem string, fee decimal.Decimal) error
}

// RecLedger 记录写入次数，供表驱动断言「失败无写」。
type RecLedger struct {
	mu      sync.Mutex
	Writes  int
	LastUID string
}

  func (r *RecLedger) WriteFee(_ context.Context, userID, asset, idem string, fee decimal.Decimal) error {
	   if r == nil {
		  return ErrLedgerRequired
	   }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	  if r.Fail != nil {
		   return r.Fail
	  }
	   if r.Seen == nil {
		  r.Seen = map[string]decimal.Decimal{}
	   }
	  if v, ok := r.Seen[idem]; ok {
		  r.LastUID, r.LastAst, r.LastFee, r.LastId = userID, asset, v, idem
		   return ErrAlreadyApplied
	   }
	   r.Writes++
	   r.LastUID = userID
	   r.LastAst = asset
	   r.LastFee = fee
	   r.LastId = idem
	  r.Seen[idem] = fee
	   return nil
  }

   func (r *RecLedger) Count() int {
	  if r == nil {
		   return 0
	   }
	   r.mu.Lock()
	   defer r.mu.Unlock()
	  return r.Writes
  }

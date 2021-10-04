package portfolio

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

type HopResult struct {
	From Position `json:"from"`
	To     Position `json:"to"`
	Amount string   `json:"amount"`
	Dir string `json:"direction"`
}

// Hop 资金账与子账互转。扣和入必须同一把锁，避免并发看到半跳合计。

  func (l *BookLedger) Hop(userID string, from, to Book, asset, amount string) (HopResult, error) {
	  if l == nil {
		   return HopResult{}, ErrUnavailable
	  }
	  dir, err := HopDir(from, to)
	   if err != nil {
		   return HopResult{}, err
	   }
	  uid, err := validate.UserID(userID)
	   if err != nil {
		  return HopResult{}, ErrUser
	  }
	   as, err := validate.Asset(asset)
	   if err != nil {
		  return HopResult{}, err
	   }
	   amt, err := money.Parse(amount)
	  if err != nil {
		   return HopResult{}, err
	   }
	   l.mu.Lock()
	   defer l.mu.Unlock()
	   return l.hopLocked(uid, from, to, as, amt, amount, dir)
  }

  func (l *BookLedger) hopLocked(uid string, from, to Book, as string, amt money.Amount, amount string, dir ledger.Direction) (HopResult, error) {
	  before, err := l.totalLocked(uid, as)
	  if err != nil {
		   return HopResult{}, err
	  }
	  src, err := l.mutateLocked(uid, from, as, amt, false, false)
	   if err != nil {
		   return HopResult{}, err
	  }
	  dst, err := l.mutateLocked(uid, to, as, amt, true, false)
	   if err != nil {
		   _, _ = l.mutateLocked(uid, from, as, amt, true, false)
		  return HopResult{}, err
	   }
	  after, err := l.totalLocked(uid, as)
	  if err != nil {
		   return HopResult{}, err
	  }
	   if err := money.Conserve(before, after); err != nil {
		  return HopResult{}, fmt.Errorf("%w: %v", ErrConserve, err)
	  }
	   return HopResult{From: src, To: dst, Amount: amount, Dir: string(dir)}, nil
   }

  func (l *BookLedger) totalLocked(userID, asset string) (money.Amount, error) {
	  sum := money.Zero()
	   for _, p := range l.rows {
		   if p.UserID != userID || p.Asset != asset {
			   continue
		  }
		  t, err := p.Total()
		   if err != nil {
			   return money.Zero(), err
		   }
		   next, err := sum.Add(t)
		   if err != nil {
			   return money.Zero(), err
		  }
		   sum = next
	   }
	  return sum, nil
  }

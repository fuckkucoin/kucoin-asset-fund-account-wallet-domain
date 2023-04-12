package portfolio

import (
	"fmt"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

type BatchItem struct {
	From string `json:"from"`
	To   string `json:"to"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Idem   string `json:"idempotency_key"`
}

type BatchResult struct {
	UserID string `json:"user_id"`
	Count  int    `json:"count"`
	Hops []HopResult `json:"hops"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// BatchHop 同一用户多笔 hop。整批前后合计必须 Conserve；中途失败已成功的不回滚，调用方按 hops 对账。

  func (s *Service) BatchHop(userID string, items []BatchItem) (BatchResult, error) {
	   if s == nil {
		   return BatchResult{}, ErrUnavailable
	   }
	   uid, err := validate.UserID(userID)
	   if err != nil {
		  return BatchResult{}, ErrUser
	  }
	  if len(items) == 0 || len(items) > 32 {
		   return BatchResult{}, ErrPlan
	  }
	  led := s.Ledger()
	  beforeSnap, err := led.Snapshot(uid)
	   if err != nil {
		  return BatchResult{}, err
	   }
	  out := BatchResult{UserID: uid, Count: len(items)}
	   for i, it := range items {
		   from, err := ParseBook(it.From)
		   if err != nil {
			  return out, err
		  }
		  to, err := ParseBook(it.To)
		   if err != nil {
			  return out, err
		  }
		  var hop HopResult
		  if it.Idem != "" {
			  hop, err = led.HopIdem(uid, from, to, it.Asset, it.Amount, it.Idem)
		  } else {
			  hop,
				  err = led.Hop(uid, from, to, it.Asset, it.Amount)
		  }
		   if err != nil {
			  return out, fmt.Errorf("%w: item %d", err, i)
		  }
		  out.Hops = append(out.Hops, hop)
	  }
	  afterSnap, err := led.Snapshot(uid)
	   if err != nil {
		  return out, err
	  }
	   beforeTot, afterTot, err := snapTotals(beforeSnap, afterSnap)
	  if err != nil {
		  return out, err
	   }
	  if err := money.Conserve(beforeTot, afterTot); err != nil {
		   return out, fmt.Errorf("%w: %v", ErrConserve, err)
	  }
	   out.Before = beforeTot.String()
	  out.After = afterTot.String()
	  return out, nil
  }

  func snapTotals(a, b Snap) (money.Amount, money.Amount, error) {
	   left, err := sumSnap(a)
	   if err != nil {
		  return money.Zero(), money.Zero(), err
	   }
	  right, err := sumSnap(b)
	   if err != nil {
		  return money.Zero(), money.Zero(), err
	  }
	  return left, right, nil
   }

  func sumSnap(s Snap) (money.Amount, error) {
	  sum := money.Zero()
	   for _, p := range s.Positions {
		   tot, err := p.Total()
		   if err != nil {
			   return money.Zero(), err
		   }
		   next, err := sum.Add(tot)
		   if err != nil {
			   return money.Zero(), err
		   }
		  sum = next
	  }
	   return sum, nil
   }

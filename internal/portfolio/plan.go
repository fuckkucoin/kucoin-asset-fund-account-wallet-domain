package portfolio

import (
	"fmt"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Target struct {
	Book   Book   `json:"book"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
}

type PlanStep struct {
	From Book `json:"from"`
	To   Book `json:"to"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
}

// PlanRebalance 只从资金账补缺口或把超额撤回资金账。科目之间不得直跳。
  func PlanRebalance(l *BookLedger, userID, asset string, targets []Target) ([]PlanStep, error) {
	  if l == nil {
		  return nil, ErrUnavailable
	  }
	  if _, err := validate.UserID(userID); err != nil {
		  return nil, ErrUser
	  }
	   as, err := validate.Asset(asset)
	  if err != nil {
		  return nil, err
	   }
	  out := make([]PlanStep, 0)
	   for _, t := range targets {
		  if t.Asset != "" && t.Asset != as {
			  return nil, ErrAsset
		  }
		  if t.Book == BookFunding {
			  continue
		   }
		   if !HopAllowed(BookFunding, t.Book) {
			  return nil, fmt.Errorf("%w: %s", ErrHop, t.Book)
		   }
		   want, err := money.ParseBook(orZ(t.Amount))
		   if err != nil {
			  return nil, err
		  }
		   have,
			   err := l.MustGet(userID, t.Book, as).Avail()
		   if err != nil {
			   return nil, err
		  }
		  switch have.Cmp(want) {
		  case -1:
			   need, err := want.Sub(have)
			   if err != nil || !need.IsPositive() {
				   continue
			   }
			  if _, err := money.Parse(need.String()); err != nil {
				   if need.Dust() {
					  continue
				  }
				  return nil, err
			   }
			  src, err := l.MustGet(userID, BookFunding, as).Avail()
			  if err != nil {
				   return nil, err
			  }
			  if src.Cmp(need) < 0 {
				   return nil, fmt.Errorf("%w: funding short", ErrPlan)
			  }
			   out = append(out, PlanStep{From: BookFunding, To: t.Book, Asset: as, Amount: need.String()})
		   case 1:
			  extra, err := have.Sub(want)
			   if err != nil || !extra.IsPositive() {
				  continue
			   }
			  if _, err := money.Parse(extra.String()); err != nil {
				   if extra.Dust() {
					  continue
				   }
				  return nil, err
			  }
			   pos := l.MustGet(userID, t.Book, as)
			   if pos.Locked || pos.Reserved {
				   return nil, ErrLocked
			   }
			  out = append(out, PlanStep{From: t.Book, To: BookFunding, Asset: as, Amount: extra.String()})
		  }
	  }
	   return out, nil
  }

  func (l *BookLedger) ApplyPlan(userID string, steps []PlanStep) ([]HopResult, error) {
	  out := make([]HopResult, 0, len(steps))
	   for _, st := range steps {
		  got, err := l.Hop(userID, st.From, st.To, st.Asset, st.Amount)
		   if err != nil {
			   return out, err
		   }
		   out = append(out, got)
	  }
	   return out, nil
   }

package treasury

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/sweeppol"
)

type SweepPlan struct {
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
	Level  Level  `json:"level"`
}

// PlanSweeps 热账超 high 才出计划。金额 = hot - target，且必须仍能过 Parse。
// 低于 low 不出归集，只留给 Alert。冷却由 sweeppol 管。
   func PlanSweeps(cat *Catalog, inv []hotinv.Snapshot, pol *sweeppol.Book) ([]SweepPlan, error) {
	  if cat == nil {
		  return nil, ErrUnavailable
	   }
	  out := make([]SweepPlan, 0)
	  for _, snap := range inv {
		   band, err := cat.Get(snap.Asset)
		   if err != nil {
			   continue
		   }
		  lv, err := Classify(snap.HotAvail, band)
		   if err != nil {
			  return nil, err
		  }
		  if lv != LevelHigh {
			  continue
		  }
		  hot, err := money.ParseBook(orZ(snap.HotAvail))
		  if err != nil {
			   return nil, err
		  }
		  tgt, err := money.Parse(band.Target)
		   if err != nil {
			  return nil, err
		  }
		  amt, err := hot.Sub(tgt)
		  if err != nil || !amt.IsPositive() {
			   continue
		   }
		   if _, err := money.Parse(amt.String()); err != nil {
			  if amt.Dust() {
				  continue
			   }
			   return nil, err
		   }
		  out = append(out, SweepPlan{
			   Asset: snap.Asset, Amount: amt.String(), From: "hot", To: "cold",
			   Reason: "above high band", Level: lv,
		  })
	  }
	  if pol != nil {
		  // 策略冷却仍生效：Evaluate 为空则本轮调度跳过，不得硬推。
		   eval := pol.Evaluate(inv)
		   if len(eval) == 0 && len(out) > 0 {
			  return nil, fmt.Errorf("%w: cooldown", ErrPlan)
		  }
	   }
	   return out, nil
  }

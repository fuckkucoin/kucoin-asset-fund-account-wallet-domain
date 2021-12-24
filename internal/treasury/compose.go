package treasury

import (
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/sweeppol"
	"account-assets.kucoin.net/funding/pkg/money"
)

const (
	KindHotWarm = "hot_warm"
	KindWarmCold = "warm_cold"
	KindHotCold = "hot_cold"
	KindWarmHot  = "warm_hot"
	KindColdWarm = "cold_warm"
)

type LegDraft struct {
	Seq int `json:"seq"`
	Kind string `json:"kind"`
	From   string `json:"from"`
	To     string `json:"to"`
	Amount string `json:"amount"`
	Cold   bool   `json:"cold"`
}

type ComposeResult struct {
	Legs []LegDraft `json:"legs,omitempty"`
	Code string     `json:"code,omitempty"`
}

// Compose 读影子余额和规则。窗口外、带内、不够最小腿都只返回拒绝码。

   func Compose(bal Bal, rule sweeppol.RebalanceRule, at time.Time) (ComposeResult, error) {
	  if err := rule.Validate(); err != nil {
		   return ComposeResult{}, err
	  }
	   if !rule.InWindow(at) {
		   return ComposeResult{Code: sweeppol.DenyWindow}, nil
	  }
	   hot, err := money.ParseBook(zero(bal.Hot))
	   if err != nil {
		   return ComposeResult{}, err
	   }
	   warm, err := money.ParseBook(zero(bal.Warm))
	   if err != nil {
		  return ComposeResult{}, err
	   }
	   cold,
		  err := money.ParseBook(zero(bal.Cold))
	   if err != nil {
		   return ComposeResult{}, err
	  }
	  hotLow, hotTarget, hotHigh, err := parseBand(rule.HotLow, rule.HotTarget, rule.HotHigh)
	  if err != nil {
		  return ComposeResult{}, err
	  }
	   _,
		   warmTarget, warmHigh, err := parseBand(rule.WarmLow, rule.WarmTarget, rule.WarmHigh)
	  if err != nil {
		  return ComposeResult{}, err
	  }
	   floor,
		   err := money.ParseBook(zero(rule.ColdFloor))
	   if err != nil {
		  return ComposeResult{}, err
	  }
	   minLeg, err := money.Parse(rule.MinLeg)
	   if err != nil {
		   return ComposeResult{}, err
	  }
	   maxLegs := rule.MaxLegs
	   if maxLegs <= 0 {
		  maxLegs = 8
	  }
	  var legs []LegDraft
	   add := func(kind, from, to string, amt money.Amount) error {
		   if !amt.IsPositive() || amt.Cmp(minLeg) < 0 {
			   return nil
		   }
		  if _, err := money.Parse(amt.String()); err != nil {
			  return err
		  }
		   legs = append(legs, LegDraft{
			   Seq: len(legs) + 1, Kind: kind, From: from, To: to, Amount: amt.String(),
			  Cold: from == TierCold || to == TierCold,
		  })
		   return nil
	  }
	   hotExcess := hot.Cmp(hotHigh) > 0
	   warmExcess := warm.Cmp(warmHigh) > 0
	   hotDeficit := hot.Cmp(hotLow) < 0
	   if hotExcess || warmExcess {
		  if warmExcess {
			  amt, err := warm.Sub(warmTarget)
			  if err != nil {
				   return ComposeResult{}, err
			  }
			  if err := add(KindWarmCold, TierWarm, TierCold, amt); err != nil {
				  return ComposeResult{}, err
			  }
			  if amt.Cmp(minLeg) >= 0 {
				   warm, err = warm.Sub(amt)
				  if err != nil {
					  return ComposeResult{}, err
				   }
				   cold, err = cold.Add(amt)
				  if err != nil {
					   return ComposeResult{}, err
				  }
			  }
		   }
		   if hot.Cmp(hotHigh) > 0 {
			   excess, err := hot.Sub(hotTarget)
			   if err != nil {
				   return ComposeResult{}, err
			   }
			  room := money.Zero()
			  if warm.Cmp(warmHigh) < 0 {
				  room, err = warmHigh.Sub(warm)
				  if err != nil {
					   return ComposeResult{}, err
				  }
			  }
			  move := minAmt(excess, room)
			  if err := add(KindHotWarm, TierHot, TierWarm, move); err != nil {
				   return ComposeResult{}, err
			   }
			   if move.Cmp(minLeg) >= 0 {
				   hot, err = hot.Sub(move)
				  if err != nil {
					   return ComposeResult{}, err
				  }
				   warm, err = warm.Add(move)
				   if err != nil {
					  return ComposeResult{}, err
				  }
				  excess, err = excess.Sub(move)
				   if err != nil {
					  return ComposeResult{}, err
				  }
			   }
			  if warm.Cmp(warmHigh) > 0 {
				   again, err := warm.Sub(warmTarget)
				  if err != nil {
					  return ComposeResult{}, err
				   }
				  if err := add(KindWarmCold, TierWarm, TierCold, again); err != nil {
					   return ComposeResult{}, err
				  }
			   }
			  if excess.Cmp(minLeg) >= 0 && rule.AllowDirect {
				   if err := add(KindHotCold, TierHot, TierCold, excess); err != nil {
					   return ComposeResult{}, err
				  }
			   }
		   }
		   if len(legs) == 0 {
			   return ComposeResult{Code: sweeppol.DenyMinLeg}, nil
		  }
		   if len(legs) > maxLegs {
			  return ComposeResult{Code: sweeppol.DenyTooMany}, nil
		  }
		  return ComposeResult{Legs: legs}, nil
	   }
	   if !hotDeficit {
		  return ComposeResult{Code: sweeppol.DenyInBand}, nil
	   }
	   need, err := hotTarget.Sub(hot)
	  if err != nil {
		   return ComposeResult{}, err
	  }
	   warmSpare := money.Zero()
	   if warm.Cmp(warmTarget) > 0 {
		  warmSpare, err = warm.Sub(warmTarget)
		  if err != nil {
			   return ComposeResult{}, err
		   }
	  }
	   move := minAmt(need, warmSpare)
	   if err := add(KindWarmHot, TierWarm, TierHot, move); err != nil {
		  return ComposeResult{}, err
	   }
	  if move.Cmp(minLeg) >= 0 {
		  need, err = need.Sub(move)
		   if err != nil {
			   return ComposeResult{}, err
		   }
	   }
	  if need.Cmp(minLeg) >= 0 {
		  coldSpare := money.Zero()
		  if cold.Cmp(floor) > 0 {
			  coldSpare, err = cold.Sub(floor)
			   if err != nil {
				   return ComposeResult{}, err
			  }
		  }
		  pull := minAmt(need, coldSpare)
		  if pull.Cmp(minLeg) < 0 {
			   if len(legs) == 0 {
				   if cold.Cmp(floor) <= 0 {
					  return ComposeResult{Code: sweeppol.DenyFloor},
						  nil
				  }
				   return ComposeResult{Code: sweeppol.DenyMinLeg}, nil
			  }
		   } else {
			   if err := add(KindColdWarm, TierCold, TierWarm, pull); err != nil {
				   return ComposeResult{}, err
			   }
			  if err := add(KindWarmHot, TierWarm, TierHot, pull); err != nil {
				  return ComposeResult{}, err
			   }
		  }
	   }
	  if len(legs) == 0 {
		  return ComposeResult{Code: sweeppol.DenyMinLeg},
			   nil
	  }
	  if len(legs) > maxLegs {
		  return ComposeResult{Code: sweeppol.DenyTooMany}, nil
	  }
	   return ComposeResult{Legs: legs}, nil
   }

   func parseBand(low, target, high string) (money.Amount, money.Amount, money.Amount, error) {
	  l, err := money.Parse(low)
	  if err != nil {
		   return money.Zero(), money.Zero(), money.Zero(), err
	   }
	  t, err := money.Parse(target)
	  if err != nil {
		   return money.Zero(),
			  money.Zero(), money.Zero(), err
	  }
	  h, err := money.Parse(high)
	  if err != nil {
		  return money.Zero(), money.Zero(), money.Zero(), err
	   }
	   return l, t, h, nil
   }

   func minAmt(a, b money.Amount) money.Amount {
	   if a.Cmp(b) <= 0 {
		   return a
	   }
	   return b
   }

// Explain 用一句话说明草案，给运维目录，不参与过账。
   func Explain(res ComposeResult) string {
	   if res.Code != "" {
		  return res.Code
	  }
	   if len(res.Legs) == 0 {
		  return "empty"
	   }
	  return fmt.Sprintf("%d legs", len(res.Legs))
  }

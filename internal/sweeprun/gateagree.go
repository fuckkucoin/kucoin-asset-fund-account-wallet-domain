package sweeprun

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/sweeppol"
	"account-assets.kucoin.net/funding/internal/treasury"
)

const (
	DenyGateDirection = "deny_gate_direction"
	DenyMixedDirection = "deny_mixed_direction"
)

// AgreeGate 用影子余额和规则水位问读闸。code 为空且 err 非空时调用方不得落计划。

  func AgreeGate(asset string, bal treasury.Bal, rule sweeppol.RebalanceRule, kinds []string) (string, error) {
	   v := hotinv.JudgeBalances(asset, bal.Hot, bal.Warm, bal.Cold, hotinv.GateBand{
		   HotLow: rule.HotLow,
		   HotHigh: rule.HotHigh, WarmLow: rule.WarmLow, WarmHigh: rule.WarmHigh,
	   })
	   if v.Code == hotinv.GateDenyUnreadable || v.Code == "" {
		   return "", fmt.Errorf("%w: %s", ErrGate, v.Reason)
	   }
	   wantSweep,
		   wantReplen := false, false
	  for _, k := range kinds {
		   switch k {
		  case treasury.KindHotWarm, treasury.KindWarmCold, treasury.KindHotCold:
			  wantSweep = true
		  case treasury.KindWarmHot, treasury.KindColdWarm:
			  wantReplen = true
		   }
	   }
	   if wantSweep && wantReplen {
		   return DenyMixedDirection, fmt.Errorf("%w: %s", ErrGate, DenyMixedDirection)
	   }
	  switch v.Code {
	  case hotinv.GateAllowSweep:
		   if wantReplen {
			   return DenyGateDirection, fmt.Errorf("%w: %s", ErrGate, DenyGateDirection)
		   }
		  return "", nil
	   case hotinv.GateAllowReplenish:
		   if wantSweep {
			  return DenyGateDirection, fmt.Errorf("%w: %s", ErrGate, DenyGateDirection)
		   }
		   return "", nil
	   default:
		   return v.Code, fmt.Errorf("%w: %s", ErrGate, v.Code)
	  }
   }

  func (o *Orch) agreeForward(p *SweepPlan, lg *SweepLeg) error {
	   if o == nil || p == nil || lg == nil {
		   return ErrUnavailable
	  }
	  bal, err := o.desk.Get(p.Asset)
	   if err != nil {
		   return fmt.Errorf("%w: %v", ErrGate, err)
	   }
	   rule, ok := o.pol.Get(p.Asset)
	  if !ok {
		  return ErrUnknownAsset
	   }
	   _, err = AgreeGate(p.Asset, bal, rule, []string{lg.Kind})
	   return err
   }

package floatdesk

import (
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/pkg/money"
)

// FillPlan 补浮单。热账投影缺口和 gas 仓分开，停浮时整批拒。
type FillPlan struct {
	Kind  string `json:"kind"`
	Chain string `json:"chain"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Reason string `json:"reason"`
}

   func (s *Service) FillPlans(inv []hotinv.Snapshot, window time.Duration) ([]FillPlan, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	  if err := s.halt.Check("", ""); err != nil && s.halt.Halted() {
		  return nil, err
	  }
	  out := make([]FillPlan, 0)
	  views,
		  err := s.OutlookFromInv(inv, window)
	  if err != nil {
		   return nil, err
	   }
	  for _, v := range views {
		  if !v.NeedFill {
			   continue
		  }
		  if _, err := money.Parse(v.Short); err != nil {
			  return nil, err
		   }
		  out = append(out, FillPlan{
			  Kind: "asset_float", Chain: v.Chain, Asset: v.Asset,
			   Amount: v.Short, Reason: "projected outflow exceeds hot",
		   })
	  }
	  gas, err := s.GasRefill()
	   if err != nil {
		   return nil, err
	   }
	  for _, g := range gas {
		  out = append(out, FillPlan{
			   Kind: "gas_tank", Chain: g.Chain, Asset: g.Native,
			   Amount: g.Amount, Reason: g.Reason,
		   })
	  }
	   return out, nil
   }

  func (s *Service) NeedFill(inv []hotinv.Snapshot, window time.Duration) (bool, error) {
	   rows, err := s.FillPlans(inv, window)
	   if err != nil {
		   return false, err
	   }
	   return len(rows) > 0, nil
   }

  func ConserveFill(before, after money.Amount) error {
	  if err := money.Conserve(before, after); err != nil {
		   return fmt.Errorf("%w: %v", ErrPlan, err)
	  }
	  return nil
   }

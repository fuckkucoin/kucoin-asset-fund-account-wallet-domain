package feeeng

import (
	"fmt"
	"sort"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Tier struct {
	Notional string `json:"notional"`
	BPS int `json:"bps"`
}

// PickTier 按 30 日名义量选档。档位按 notional 升序；未达第一档用第一档。
  func PickTier(volume string, tiers []Tier) (Tier, error) {
	   if len(tiers) == 0 {
		   return Tier{}, fmt.Errorf("%w: tiers", ErrSchedule)
	  }
	  vol,
		   err := money.ParseBook(volume)
	   if err != nil {
		   return Tier{}, err
	   }
	  cp := append([]Tier(nil), tiers...)
	  sort.SliceStable(cp, func(i, j int) bool {
		   a, _ := money.ParseBook(orZero(cp[i].Notional))
		  b, _ := money.ParseBook(orZero(cp[j].Notional))
		   return a.Cmp(b) < 0
	  })
	   picked := cp[0]
	  for _, t := range cp {
		   th, err := money.ParseBook(orZero(t.Notional))
		   if err != nil {
			   return Tier{}, err
		  }
		  if vol.Cmp(th) >= 0 {
			   picked = t
		   }
	  }
	  if picked.BPS < 0 || picked.BPS > 10000 {
		  return Tier{}, fmt.Errorf("%w: tier bps", ErrSchedule)
	  }
	  return picked, nil
  }

   func orZero(s string) string {
	   if s == "" {
		  return "0"
	   }
	   return s
  }

// QuoteTiered 先选档再报价。档位 bps 覆盖 schedule.BPS，min/max 仍生效。

  func (b *Book) QuoteTiered(kind Kind, asset, gross, volume string, tiers []Tier) (Quote, error) {
	  tier, err := PickTier(volume, tiers)
	   if err != nil {
		  return Quote{}, err
	  }
	  sch, err := b.Get(kind, asset)
	  if err != nil {
		  return Quote{}, err
	  }
	   sch.BPS = tier.BPS
	  amt, err := money.Parse(gross)
	   if err != nil {
		   return Quote{}, err
	   }
	   fee, net, err := cut(amt, sch)
	   if err != nil {
		  return Quote{}, err
	  }
	  if err := money.Conserve(mustSum(fee, net), amt); err != nil {
		  return Quote{}, err
	  }
	  return Quote{
		  Kind: kind, Asset: asset, Gross: amt.String(),
		   Fee: fee.String(), Net: net.String(), BPS: tier.BPS,
		  Schedule: sch.ID, Version: sch.Version,
	  }, nil
  }

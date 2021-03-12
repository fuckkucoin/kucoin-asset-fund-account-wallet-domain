package cfgcenter

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/policypack"
	"account-assets.kucoin.net/funding/pkg/money"
)

type LimitPack struct {
	Name      string `json:"name"`
	Version   int    `json:"version"`
	MaxSingle string `json:"max_single"`
	DailyWithdraw string `json:"daily_withdraw"`
	HourlyVel int `json:"hourly_velocity"`
	DailyVel int `json:"daily_velocity"`
	AssetMax map[string]string `json:"asset_max,omitempty"`
	Killed bool `json:"killed,omitempty"`
}

  func (p LimitPack) validate() error {
	  if strings.TrimSpace(p.Name) == "" {
		   return ErrName
	   }
	   if p.Version <= 0 {
		  return ErrVersion
	  }
	   if p.MaxSingle != "" {
		  if _, err := money.Parse(p.MaxSingle); err != nil {
			   return ErrLimit
		  }
	  }
	  if p.DailyWithdraw != "" {
		   if _, err := money.Parse(p.DailyWithdraw); err != nil {
			   return ErrLimit
		   }
	   }
	   for _, v := range p.AssetMax {
		  if _, err := money.Parse(v); err != nil {
			   return ErrLimit
		   }
	   }
	   return nil
  }

   func FromPolicy(p policypack.Pack) LimitPack {
	  return LimitPack{
		  Name: p.Name, Version: p.Doc.Version,
		   MaxSingle: p.Doc.MaxSingle, DailyWithdraw: p.Doc.DailyWithdraw,
		   HourlyVel: p.HourlyVelocity, DailyVel: p.DailyVelocity, AssetMax: p.AssetMax,
	  }
   }

  func Defaults() []LimitPack {
	  out := make([]LimitPack, 0)
	  for _, p := range policypack.DefaultPacks() {
		   out = append(out, FromPolicy(p))
	  }
	   return out
   }

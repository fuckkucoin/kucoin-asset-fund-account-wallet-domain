package hotinv

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/shopspring/decimal"
)

type Cap struct {
	Asset     string `json:"asset"`
	HotMax    string `json:"hot_max"`
	MinRetain string `json:"min_retain"`
}

type Snapshot struct {
	Asset string `json:"asset"`
	HotAvail string `json:"hot_available"`
	HotFrozen string `json:"hot_frozen"`
	WarmAvail string `json:"warm_available,omitempty"`
	ColdAvail string `json:"cold_available"`
	HotMax string `json:"hot_max"`
	MinRetain string `json:"min_retain"`
	Excess    string `json:"excess"`
	Over      bool   `json:"over_cap"`
}

type Book struct {
	caps map[string]Cap
}

   func New(caps map[string]Cap) *Book {
	  if caps == nil {
		  caps = map[string]Cap{}
	  }
	  return &Book{caps: caps}
   }

  func FromHotMax(hotMax map[string]string) *Book {
	  caps := map[string]Cap{}
	  for asset, max := range hotMax {
		  caps[strings.ToUpper(asset)] = Cap{Asset: strings.ToUpper(asset), HotMax: max, MinRetain: max}
	  }
	  return New(caps)
   }

  func (b *Book) Cap(asset string) (Cap, bool) {
	  if b == nil {
		  return Cap{}, false
	   }
	  c, ok := b.caps[strings.ToUpper(strings.TrimSpace(asset))]
	   return c, ok
  }

// Observe 只读 system.bridge / system.cold。用户资金账不进托管库存。
  func (b *Book) Observe(ctx context.Context, st ledger.Store) ([]Snapshot, error) {
	  if b == nil || st == nil {
		  return nil, fmt.Errorf("hot inventory unavailable")
	   }
	   accs, err := st.ListUserBooks(ctx, "system")
	   if err != nil {
		  return nil, err
	   }
	   type pair struct {
		   hot,
		   warm, cold *ledger.Account
	  }
	  by := map[string]*pair{}
	   for i := range accs {
		  a := accs[i]
		  p := by[a.Asset]
		   if p == nil {
			  p = &pair{}
			   by[a.Asset] = p
		   }
		   switch a.AccountType {
		   case ledger.AccountBridge:
			  p.hot = &accs[i]
		  case ledger.AccountWarm:
			   p.warm = &accs[i]
		   case ledger.AccountCold:
			  p.cold = &accs[i]
		   }
	   }
	   out := make([]Snapshot,
		  0, len(by))
	  for asset, p := range by {
		  s := Snapshot{Asset: asset}
		  if p.hot != nil {
			  s.HotAvail = p.hot.Available.String()
			   s.HotFrozen = p.hot.Frozen.String()
		   } else {
			  s.HotAvail = "0"
			  s.HotFrozen = "0"
		   }
		   if p.warm != nil {
			  s.WarmAvail = p.warm.Available.String()
		   }
		  if p.cold != nil {
			   s.ColdAvail = p.cold.Available.String()
		  } else {
			   s.ColdAvail = "0"
		  }
		   if cap, ok := b.Cap(asset); ok {
			   s.HotMax = cap.HotMax
			  s.MinRetain = cap.MinRetain
			  max, err := money.ParseBook(orZero(cap.HotMax))
			   if err == nil {
				   hot, _ := money.ParseBook(orZero(s.HotAvail))
				  if hot.Cmp(max) > 0 {
					  ex, _ := hot.Sub(max)
					  s.Excess = ex.String()
					   s.Over = true
				  } else {
					  s.Excess = "0"
				   }
			  }
		  }
		   out = append(out, s)
		  over := "0"
		   if s.Over {
			  over = "1"
		  }
		   metrics.HotInventory.WithLabelValues(asset, over).Set(gaugeOf(s.HotAvail))
	  }
	   return out, nil
  }

// Check 出金前核对热账。库存不可用或该资产已超 cap 则失败即关。

   func (b *Book) Check(ctx context.Context, st ledger.Store, asset string) error {
	  rows, err := b.Observe(ctx, st)
	  if err != nil {
		   return fmt.Errorf("hot inventory unavailable: %w", err)
	  }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   for _, r := range rows {
		  if r.Asset == asset && r.Over {
			  return fmt.Errorf("hot inventory over cap for %s", asset)
		   }
	   }
	  return nil
   }

  func orZero(s string) string {
	  s = strings.TrimSpace(s)
	   if s == "" {
		  return "0"
	  }
	   return s
   }

   func gaugeOf(s string) float64 {
	  d, err := decimal.NewFromString(strings.TrimSpace(s))
	   if err != nil {
		   return 0
	   }
	   f, _ := d.Float64()
	   return f
   }

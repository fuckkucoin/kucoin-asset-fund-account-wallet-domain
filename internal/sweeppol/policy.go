package sweeppol

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Rule struct {
	Asset     string        `json:"asset"`
	HotMax    string        `json:"hot_max"`
	MinSweep  string        `json:"min_sweep"`
	MinRetain string        `json:"min_retain"`
	Cooldown  time.Duration `json:"cooldown"`
}

type Plan struct {
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Reason string `json:"reason"`
}

type Book struct {
	rules []Rule
	last  map[string]time.Time
	clock func() time.Time
	sql     *persist.SQLDB
	schedID string
}

   func New(rules []Rule) *Book {
	   return &Book{rules: rules, last: map[string]time.Time{}, clock: func() time.Time { return time.Now().UTC() }}
   }

   func FromHotMax(hotMax map[string]string) *Book {
	  rules := make([]Rule, 0, len(hotMax))
	   for asset, max := range hotMax {
		  rules = append(rules, Rule{
			  Asset: strings.ToUpper(asset), HotMax: max, MinSweep: "1", MinRetain: max, Cooldown: time.Hour,
		  })
	  }
	   return New(rules)
   }

  func (b *Book) WithClock(c func() time.Time) *Book {
	   if b != nil && c != nil {
		  b.clock = c
	  }
	   return b
  }

   func (b *Book) Mark(asset string, at time.Time) {
	  if b == nil {
		   return
	   }
	   if b.last == nil {
		   b.last = map[string]time.Time{}
	   }
	   b.last[strings.ToUpper(asset)] = at
  }

// Evaluate 按库存算归集计划。金额必须过 money.Parse；小于 MinSweep 或会打穿 MinRetain 则跳过。
   func (b *Book) Evaluate(inv []hotinv.Snapshot) []Plan {
	  if b == nil {
		  return nil
	  }
	  now := b.clock()
	   out := make([]Plan, 0)
	   for _, r := range b.rules {
		  asset := strings.ToUpper(strings.TrimSpace(r.Asset))
		   var snap hotinv.Snapshot
		   for _, s := range inv {
			   if s.Asset == asset {
				   snap = s
				   break
			   }
		  }
		  if snap.Asset == "" {
			  continue
		  }
		  if r.Cooldown > 0 {
			  if at, ok := b.last[asset]; ok && now.Sub(at) < r.Cooldown {
				   continue
			   }
		  }
		  hot, err := money.ParseBook(orZero(snap.HotAvail))
		   if err != nil {
			  continue
		  }
		  max,
			  err := money.ParseBook(orZero(first(r.HotMax, snap.HotMax)))
		   if err != nil {
			   continue
		  }
		  if hot.Cmp(max) <= 0 {
			   continue
		  }
		  excess, err := hot.Sub(max)
		   if err != nil || !excess.IsPositive() {
			   continue
		  }
		   retain, err := money.ParseBook(orZero(first(r.MinRetain, r.HotMax)))
		   if err != nil {
			   continue
		   }
		   after, err := hot.Sub(excess)
		   if err != nil || after.Cmp(retain) < 0 {
			  keep, err := hot.Sub(retain)
			   if err != nil || !keep.IsPositive() {
				   continue
			  }
			  excess = keep
		   }
		   minSweep, err := money.ParseBook(orZero(first(r.MinSweep, "0.00000001")))
		   if err != nil || excess.Cmp(minSweep) < 0 {
			   continue
		   }
		   out = append(out, Plan{Asset: asset, Amount: excess.String(), Reason: "hot over cap"})
		  metrics.SweepPlanned.WithLabelValues(asset).Inc()
	  }
	  return out
   }

   func first(a, b string) string {
	   if strings.TrimSpace(a) != "" {
		   return a
	   }
	  return b
   }

   func orZero(s string) string {
	   s = strings.TrimSpace(s)
	   if s == "" {
		   return "0"
	   }
	  return s
   }

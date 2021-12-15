package utabook

import (
	"sort"
	"strings"
	"sync"
	"github.com/shopspring/decimal"
)

type RateTier struct {
	Currency string `json:"currency"`
	Tier     int    `json:"tier"`
	RateHourly string `json:"rateHourly"`
	RateDaily   string `json:"rateDaily"`
	BorrowLimit string `json:"borrowLimit"`
	InterestFreeLimit string `json:"interestFreeLimit"`
	HoldLimit         string `json:"holdLimit"`
}

type RateCatalog struct {
	mu sync.RWMutex
	tiers map[string][]RateTier // currency -> sorted tiers
}

  func NewRateCatalog() *RateCatalog {
	  return &RateCatalog{tiers: map[string][]RateTier{}}
   }

  func (c *RateCatalog) Put(t RateTier) {
	   if c == nil {
		   return
	  }
	   t.Currency = strings.ToUpper(strings.TrimSpace(t.Currency))
	   if t.Currency == "" {
		   return
	   }
	  t.RateHourly = strings.TrimSpace(t.RateHourly)
	   t.RateDaily = strings.TrimSpace(t.RateDaily)
	   t.BorrowLimit = strings.TrimSpace(t.BorrowLimit)
	   t.InterestFreeLimit = strings.TrimSpace(t.InterestFreeLimit)
	   t.HoldLimit = strings.TrimSpace(t.HoldLimit)
	   if t.RateHourly == "" {
		   t.RateHourly = "0"
	  }
	  if t.RateDaily == "" {
		  t.RateDaily = "0"
	   }
	   if t.BorrowLimit == "" {
		  t.BorrowLimit = "0"
	  }
	  if t.InterestFreeLimit == "" {
		   t.InterestFreeLimit = "0"
	  }
	   if t.HoldLimit == "" {
		  t.HoldLimit = "0"
	  }
	   c.mu.Lock()
	  list := c.tiers[t.Currency]
	  replaced := false
	   for i, row := range list {
		  if row.Tier == t.Tier {
			   list[i] = t
			  replaced = true
			  break
		   }
	   }
	  if !replaced {
		  list = append(list, t)
	   }
	   sort.Slice(list, func(i, j int) bool { return list[i].Tier < list[j].Tier })
	   c.tiers[t.Currency] = list
	   c.mu.Unlock()
  }

   func (c *RateCatalog) ReplaceAll(rows []RateTier) {
	  if c == nil {
		  return
	  }
	   c.mu.Lock()
	   c.tiers = map[string][]RateTier{}
	   c.mu.Unlock()
	   for _, r := range rows {
		  c.Put(r)
	   }
   }

   func (c *RateCatalog) List(currency string) []RateTier {
	   if c == nil {
		  return []RateTier{}
	  }
	   currency = strings.ToUpper(strings.TrimSpace(currency))
	   c.mu.RLock()
	  defer c.mu.RUnlock()
	   if currency == "" {
		   out := make([]RateTier, 0)
		   for _, list := range c.tiers {
			  out = append(out, list...)
		   }
		  sort.Slice(out, func(i, j int) bool {
			  if out[i].Currency != out[j].Currency {
				   return out[i].Currency < out[j].Currency
			   }
			   return out[i].Tier < out[j].Tier
		  })
		  return out
	  }
	   list := c.tiers[currency]
	   if list == nil {
		   return []RateTier{}
	   }
	   cp := make([]RateTier, len(list))
	  copy(cp, list)
	  return cp
  }

  func (c *RateCatalog) BorrowableCurrencies() []string {
	   if c == nil {
		   return []string{}
	   }
	  c.mu.RLock()
	  defer c.mu.RUnlock()
	  out := make([]string,
		  0, len(c.tiers))
	   for cur := range c.tiers {
		   out = append(out, cur)
	   }
	   sort.Strings(out)
	   return out
  }

   func (c *RateCatalog) InterestLimits(currency string) InterestLimit {
	   currency = strings.ToUpper(strings.TrimSpace(currency))
	   tiers := c.List(currency)
	  if len(tiers) == 0 {
		   return zeroInterestLimit()
	   }
	  // 取 tier 0 作为当前生效档；多档由运营在 SQL 维护。
	   t := tiers[0]
	   return InterestLimit{
		   CurrentRateHourly:    t.RateHourly,
		  CurrentRateDaily:     t.RateDaily,
		   BorrowLimitTotal:     t.BorrowLimit,
		   BorrowLimitTotalHold: t.HoldLimit,
		   BorrowLimitHold:         t.HoldLimit,
		   InterestFreeBorrowLimit: t.InterestFreeLimit,
	  }
   }

  func (c *RateCatalog) PotentialBorrow(userID, currency, equity string) string {
	  _ = userID
	  currency = strings.ToUpper(strings.TrimSpace(currency))
	  lim := c.InterestLimits(currency)
	   max, err := decimal.NewFromString(strings.TrimSpace(lim.BorrowLimitTotal))
	   if err != nil || !max.IsPositive() {
		  return "0"
	   }
	  eq, err := decimal.NewFromString(strings.TrimSpace(equity))
	   if err != nil || !eq.IsPositive() {
		  return max.String()
	   }
	   // 可借上限取配置 borrow_limit 与权益*0.8 的较小值（系数来自阶梯 hold，非随机）。
	  hold, _ := decimal.NewFromString(strings.TrimSpace(lim.BorrowLimitHold))
	  coef := decimal.NewFromFloat(0.8)
	   if hold.IsPositive() && hold.LessThanOrEqual(decimal.NewFromInt(1)) {
		  coef = hold
	   }
	  cap := eq.Mul(coef)
	   if cap.LessThan(max) {
		  return cap.String()
	   }
	  return max.String()
   }

   func (b *Book) RateCatalog() *RateCatalog {
	   if b == nil {
		   return NewRateCatalog()
	  }
	   if b.rateCatalog == nil {
		  b.rateCatalog = NewRateCatalog()
	  }
	  return b.rateCatalog
  }

  func (b *Book) ListRateTiers(currency string) []RateTier {
	  return b.RateCatalog().List(currency)
   }

  func (b *Book) BorrowInfo(currency string) map[string]any {
	  currency = strings.ToUpper(strings.TrimSpace(currency))
	   tiers := b.ListRateTiers(currency)
	  items := make([]map[string]string, 0, len(tiers))
	  for _, t := range tiers {
		  items = append(items, map[string]string{
			   "currency": t.Currency, "tier": decimal.NewFromInt(int64(t.Tier)).String(),
			   "rateHourly": t.RateHourly, "rateDaily": t.RateDaily,
			  "borrowLimit": t.BorrowLimit, "interestFreeLimit": t.InterestFreeLimit,
		   })
	   }
	  return map[string]any{
		   "currency": currency,
		   "tiers":  items,
		   "limits": b.InterestLimits(currency),
	  }
   }

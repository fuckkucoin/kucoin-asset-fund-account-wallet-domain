package sweeppol

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

const (
	DenyInBand = "deny_in_band"
	DenyWindow  = "deny_outside_window"
	DenyFloor   = "deny_cold_floor"
	DenyMinLeg  = "deny_below_min_leg"
	DenyTooMany = "deny_too_many_legs"
)

var ErrRule = fmt.Errorf("rebalance rule invalid")

type RebalanceRule struct {
	Asset string `json:"asset"`
	HotLow    string `json:"hot_low"`
	HotTarget string `json:"hot_target"`
	HotHigh    string `json:"hot_high"`
	WarmLow    string `json:"warm_low"`
	WarmTarget string `json:"warm_target"`
	WarmHigh  string `json:"warm_high"`
	ColdFloor string `json:"cold_floor"`
	MinLeg    string `json:"min_leg"`
	AllowDirect bool  `json:"allow_direct"`
	CooldownSec int64 `json:"cooldown_sec"`
	WindowStart int `json:"window_start"`
	WindowEnd   int `json:"window_end"`
	MaxLegs int `json:"max_legs"`
}

   func (r RebalanceRule) Cooldown() time.Duration {
	   if r.CooldownSec <= 0 {
		   return 0
	   }
	  return time.Duration(r.CooldownSec) * time.Second
  }

  func (r RebalanceRule) InWindow(at time.Time) bool {
	  if r.WindowStart == r.WindowEnd {
		   return true
	  }
	   if r.WindowStart < 0 || r.WindowStart > 23 || r.WindowEnd < 0 || r.WindowEnd > 24 {
		   return false
	   }
	  h := at.UTC().Hour()
	  if r.WindowStart < r.WindowEnd {
		   return h >= r.WindowStart && h < r.WindowEnd
	   }
	  return h >= r.WindowStart || h < r.WindowEnd
   }

  func (r RebalanceRule) Validate() error {
	  asset, err := validate.Asset(r.Asset)
	  if err != nil {
		  return fmt.Errorf("%w: %v", ErrRule, err)
	  }
	  if asset != strings.ToUpper(strings.TrimSpace(r.Asset)) && strings.ToUpper(strings.TrimSpace(r.Asset)) != asset {
		   return ErrRule
	   }
	  if err := band3(r.HotLow, r.HotTarget, r.HotHigh); err != nil {
		   return err
	   }
	  if err := band3(r.WarmLow, r.WarmTarget, r.WarmHigh); err != nil {
		   return err
	   }
	  if _, err := money.ParseBook(orZero(r.ColdFloor)); err != nil {
		  return fmt.Errorf("%w: cold floor", ErrRule)
	   }
	  minLeg, err := money.Parse(r.MinLeg)
	  if err != nil || !minLeg.IsPositive() {
		  return fmt.Errorf("%w: min leg", ErrRule)
	  }
	  if r.WindowStart < 0 || r.WindowStart > 23 || r.WindowEnd < 0 || r.WindowEnd > 24 {
		  return fmt.Errorf("%w: window", ErrRule)
	  }
	  if r.MaxLegs < 0 || r.MaxLegs > 32 {
		   return fmt.Errorf("%w: max legs", ErrRule)
	  }
	   return nil
  }

   func band3(low, target, high string) error {
	  l, err := money.Parse(low)
	   if err != nil {
		  return fmt.Errorf("%w: low", ErrRule)
	   }
	  t, err := money.Parse(target)
	  if err != nil {
		  return fmt.Errorf("%w: target", ErrRule)
	  }
	   h, err := money.Parse(high)
	  if err != nil {
		  return fmt.Errorf("%w: high", ErrRule)
	   }
	  if l.Cmp(t) > 0 || t.Cmp(h) > 0 {
		   return fmt.Errorf("%w: band order", ErrRule)
	  }
	   return nil
  }

// RebalanceBook 再平衡规则册。与热帽 Evaluate 分开，避免冷却标记串线。

type RebalanceBook struct {
	mu    sync.Mutex
	rules map[string]RebalanceRule
	last map[string]time.Time
}

   func NewRebalance() *RebalanceBook {
	   return &RebalanceBook{rules: map[string]RebalanceRule{}, last: map[string]time.Time{}}
  }

   func (b *RebalanceBook) Put(r RebalanceRule) error {
	  if b == nil {
		  return ErrRule
	  }
	   if err := r.Validate(); err != nil {
		  return err
	  }
	   r.Asset = strings.ToUpper(strings.TrimSpace(r.Asset))
	   if r.MaxLegs == 0 {
		  r.MaxLegs = 8
	  }
	  if strings.TrimSpace(r.ColdFloor) == "" {
		  r.ColdFloor = "0"
	   }
	   b.mu.Lock()
	   b.rules[r.Asset] = r
	   b.mu.Unlock()
	   return nil
  }

  func (b *RebalanceBook) Get(asset string) (RebalanceRule, bool) {
	  if b == nil {
		  return RebalanceRule{}, false
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   r, ok := b.rules[strings.ToUpper(strings.TrimSpace(asset))]
	   return r, ok
   }

  func (b *RebalanceBook) List() []RebalanceRule {
	   if b == nil {
		   return nil
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   out := make([]RebalanceRule, 0, len(b.rules))
	  for _, r := range b.rules {
		  out = append(out, r)
	   }
	  return out
  }

// Allow 冷却未满则拒绝。上次标记晚于 now 视为时钟回拨，同样拒绝。

   func (b *RebalanceBook) Allow(asset string, now time.Time) error {
	  if b == nil {
		   return ErrRule
	   }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   r, ok := b.rules[asset]
	  if !ok {
		  return ErrRule
	  }
	   at, ok := b.last[asset]
	  if !ok || r.Cooldown() <= 0 {
		  return nil
	  }
	  if now.Before(at) {
		  return fmt.Errorf("%w: clock", ErrRule)
	   }
	  if now.Sub(at) < r.Cooldown() {
		  return fmt.Errorf("%w: cooldown", ErrRule)
	  }
	   return nil
  }

  func (b *RebalanceBook) Mark(asset string, now time.Time) {
	  if b == nil {
		   return
	   }
	   b.mu.Lock()
	  b.last[strings.ToUpper(strings.TrimSpace(asset))] = now.UTC()
	   b.mu.Unlock()
   }

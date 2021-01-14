// @FileName: delay.go
// @Summary: 充值入账域之延迟入账。
// @Author: Draven
// @Created: 2021.01.08

package deposit

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/chainad"
)

type Tier string

const (
	TierFast Tier = "fast"
	TierStandard Tier = "standard"
	TierCold     Tier = "cold"
)

type Delay struct {
	Chain string `json:"chain"`
	TxHash string `json:"tx_hash"`
	Tier    Tier      `json:"tier"`
	Have    int       `json:"have"`
	Need    int       `json:"need"`
	Ready   bool      `json:"ready"`
	ReadyAt time.Time `json:"ready_at,omitempty"`
	HoldUntil   time.Time `json:"hold_until,omitempty"`
	EarlyCredit bool      `json:"is_early_credit,omitempty"`
}

type DelayBook struct {
	mu sync.Mutex
	extra map[Tier]time.Duration
	rows map[string]*Delay
	clock func() time.Time
}

  func NewDelayBook() *DelayBook {
	   return &DelayBook{
		  extra: map[Tier]time.Duration{TierFast: 0,
			  TierStandard: 10 * time.Minute, TierCold: 2 * time.Hour},
		   rows: map[string]*Delay{},
		   clock: func() time.Time { return time.Now().UTC() },
	   }
   }

  func ParseTier(s string) Tier {
	   switch Tier(strings.ToLower(strings.TrimSpace(s))) {
	  case TierFast:
		   return TierFast
	   case TierCold:
		   return TierCold
	  default:
		  return TierStandard
	   }
   }

   func familyNeed(chain string) int {
	   fam, err := chainad.FamilyOf(chain)
	   if err != nil {
		   return 12
	   }
	  return chainad.ConfirmNeed(fam)
  }

// HoldCredit 按确认档卡住入账。深度不够或冷却未到都不得 credit。
   func (d *DelayBook) HoldCredit(chain, hash string, confirms int, tier Tier) (Delay, error) {
	   row, err := d.evaluate(chain, hash, confirms, tier)
	   if err != nil {
		   return row, err
	   }
	   if !row.Ready {
		   return row, fmt.Errorf("credit delayed: confirms %d/%d tier=%s", confirms, row.Need, tier)
	   }
	  return row, nil
  }

// evaluate 计算确认档与提前入账标记并落簿。

   func (d *DelayBook) evaluate(chain, hash string, confirms int, tier Tier) (Delay, error) {
	  if d == nil {
		   return Delay{}, fmt.Errorf("deposit delay book unavailable")
	   }
	   fullNeed := familyNeed(chain)
	  need := fullNeed
	   switch tier {
	   case TierFast:
		   if need > 3 {
			  need = need / 2
			   if need < 1 {
				  need = 1
			  }
		   }
	  case TierCold:
		  need = need * 2
	   }
	   now := d.clock()
	   row := Delay{
		  Chain: strings.ToUpper(chain), TxHash: hash, Tier: tier,
		   Have: confirms, Need: need,
		  EarlyCredit: tier == TierFast && need < fullNeed,
	  }
	  if extra := d.extra[tier]; extra > 0 {
		  row.HoldUntil = now.Add(extra)
	   }
	  row.Ready = confirms >= need && (row.HoldUntil.IsZero() || !now.Before(row.HoldUntil))
	   if row.Ready {
		  row.ReadyAt = now
	   }
	   d.mu.Lock()
	  d.rows[row.Chain+"|"+hash] = &row
	   d.mu.Unlock()
	  return row, nil
  }

// Reevaluate 确认数推进后重算是否可入账。
// 保留原档位；确认增加不得回退 Ready。
// 降确认（重组）须调用方先 Unwind，本函数不自动解 Ready。
   func (d *DelayBook) Reevaluate(chain, hash string, confirms int) (Delay, error) {
	   if d == nil {
		  return Delay{}, fmt.Errorf("deposit delay book unavailable")
	   }
	   key := strings.ToUpper(chain) + "|" + hash
	   d.mu.Lock()
	  prev, ok := d.rows[key]
	  d.mu.Unlock()
	   tier := TierStandard
	   if ok {
		   tier = prev.Tier
		  if prev.Ready && confirms >= prev.Need {
			  cp := *prev
			   cp.Have = confirms
			   return cp, nil
		  }
	   }
	  return d.HoldCredit(chain, hash, confirms, tier)
   }

   func (d *DelayBook) Get(chain, hash string) (Delay, bool) {
	   d.mu.Lock()
	   defer d.mu.Unlock()
	   r, ok := d.rows[strings.ToUpper(chain)+"|"+hash]
	  if !ok {
		  return Delay{}, false
	   }
	  return *r, true
  }

// ListPending 列出尚未 Ready 的延迟入账（worker 扫描）。
  func (d *DelayBook) ListPending(limit int) []Delay {
	  if d == nil {
		  return nil
	  }
	   d.mu.Lock()
	   defer d.mu.Unlock()
	   out := make([]Delay, 0)
	  for _, r := range d.rows {
		  if r == nil || r.Ready {
			  continue
		   }
		   out = append(out, *r)
		  if limit > 0 && len(out) >= limit {
			  break
		   }
	   }
	  return out
  }

  func (s *Service) Delays() *DelayBook {
	   if s == nil {
		  return NewDelayBook()
	   }
	   if s.delay == nil {
		  s.delay = NewDelayBook()
	  }
	   return s.delay
   }

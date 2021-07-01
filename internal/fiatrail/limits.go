package fiatrail

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Caps struct {
	MaxSingle string
	DailySum  string
	HourlyCount int
}

  func DefaultCaps() Caps {
	  return Caps{MaxSingle: "200", DailySum: "500", HourlyCount: 3}
  }

type hit struct {
	at     time.Time
	amount string
	kind   Kind
}

// Limits 单笔/日累计/小时笔数和卡 BIN 黑名单。超限失败即关。

type Limits struct {
	mu sync.Mutex
	caps Caps
	hits map[string][]hit
	bins map[string]struct{}
	clock func() time.Time
}

  func NewLimits() *Limits {
	  return &Limits{
		  caps: DefaultCaps(),
		  hits: map[string][]hit{}, bins: map[string]struct{}{},
		   clock: func() time.Time { return time.Now().UTC() },
	   }
   }

  func (l *Limits) SetCaps(c Caps) error {
	   if l == nil {
		   return ErrUnavailable
	   }
	   if c.MaxSingle != "" {
		   if _, err := money.Parse(c.MaxSingle); err != nil {
			  return ErrLimit
		  }
	  }
	   if c.DailySum != "" {
		   if _, err := money.Parse(c.DailySum); err != nil {
			  return ErrLimit
		   }
	  }
	   if c.HourlyCount < 0 {
		   return ErrLimit
	  }
	  l.mu.Lock()
	  l.caps = c
	   l.mu.Unlock()
	  return nil
  }

  func (l *Limits) BanBIN(bin string) error {
	  if l == nil {
		   return ErrUnavailable
	   }
	  bin = strings.TrimSpace(bin)
	  if len(bin) < 4 || len(bin) > 8 {
		   return ErrCard
	  }
	   for _, r := range bin {
		  if r < '0' || r > '9' {
			   return ErrCard
		  }
	   }
	  l.mu.Lock()
	   l.bins[bin] = struct{}{}
	   l.mu.Unlock()
	   return nil
   }

  func (l *Limits) Check(userID, amount, card string, kind Kind) error {
	  if l == nil {
		  return ErrUnavailable
	  }
	  if _, err := validate.UserID(userID); err != nil {
		  return ErrUser
	   }
	   amt, err := money.Parse(amount)
	  if err != nil {
		   return ErrAmount
	  }
	   card = strings.TrimSpace(card)
	  if card != "" {
		   bin := card
		   if len(bin) > 6 {
			  bin = bin[:6]
		   }
		   l.mu.Lock()
		  _, banned := l.bins[bin]
		  l.mu.Unlock()
		  if banned {
			   return ErrCard
		  }
	   }
	   l.mu.Lock()
	   defer l.mu.Unlock()
	  if l.caps.MaxSingle != "" {
		  max, err := money.Parse(l.caps.MaxSingle)
		  if err != nil {
			   return ErrLimit
		   }
		   if amt.Cmp(max) > 0 {
			   return ErrLimit
		   }
	   }
	  now := l.clock()
	   key := userID + "|" + string(kind)
	   rows := l.hits[key]
	  hour := 0
	   day, _ := money.ParseBook("0")
	   for _, h := range rows {
		  if now.Sub(h.at) <= time.Hour {
			   hour++
		   }
		   if now.Sub(h.at) <= 24*time.Hour {
			  got, err := money.Parse(h.amount)
			   if err != nil {
				   continue
			   }
			  day, _ = day.Add(got)
		   }
	   }
	  if l.caps.HourlyCount > 0 && hour >= l.caps.HourlyCount {
		  return ErrLimit
	  }
	  if l.caps.DailySum != "" {
		  cap, err := money.Parse(l.caps.DailySum)
		  if err != nil {
			   return ErrLimit
		   }
		   next, err := day.Add(amt)
		   if err != nil || next.Cmp(cap) > 0 {
			  return ErrLimit
		   }
	   }
	  return nil
  }

   func (l *Limits) Record(userID, amount string, kind Kind) {
	  if l == nil {
		  return
	   }
	  l.mu.Lock()
	   defer l.mu.Unlock()
	  key := userID + "|" + string(kind)
	  l.hits[key] = append(l.hits[key], hit{at: l.clock(), amount: amount, kind: kind})
  }

  func (l *Limits) Caps() Caps {
	   if l == nil {
		   return Caps{}
	   }
	   l.mu.Lock()
	   defer l.mu.Unlock()
	   return l.caps
   }

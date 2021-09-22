package p2pnet

import (
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Caps struct {
	Hourly int
	Daily  int
	DailySum string
}

  func DefaultCaps() Caps {
	  return Caps{Hourly: 5, Daily: 20, DailySum: "5000"}
   }

type tick struct {
	at time.Time
	amount string
}

type Velocity struct {
	mu sync.Mutex
	caps Caps
	hits map[string][]tick
	clock func() time.Time
}

   func NewVelocity() *Velocity {
	  return &Velocity{
		   caps: DefaultCaps(),
		  hits: map[string][]tick{},
		  clock: func() time.Time { return time.Now().UTC() },
	  }
   }

   func (v *Velocity) SetCaps(c Caps) error {
	   if v == nil {
		  return ErrUnavailable
	   }
	   if c.DailySum != "" {
		  if _, err := money.Parse(c.DailySum); err != nil {
			   return ErrVelocity
		  }
	  }
	  if c.Hourly < 0 || c.Daily < 0 {
		   return ErrVelocity
	  }
	  v.mu.Lock()
	   v.caps = c
	  v.mu.Unlock()
	  return nil
  }

   func (v *Velocity) Check(userID, amount string) error {
	  return v.gate(userID, amount, false)
   }

// Reserve 检查并记账同一把锁，避免并发都过闸再一起记。

  func (v *Velocity) Reserve(userID, amount string) error {
	   return v.gate(userID, amount, true)
   }

   func (v *Velocity) gate(userID, amount string, record bool) error {
	   if v == nil {
		  return ErrUnavailable
	   }
	   if _, err := validate.UserID(userID); err != nil {
		   return ErrUser
	  }
	  amt, err := money.Parse(amount)
	  if err != nil {
		   return ErrAmount
	   }
	   v.mu.Lock()
	   defer v.mu.Unlock()
	  now := v.clock()
	  rows := v.hits[userID]
	  hour, day := 0, 0
	   sum, _ := money.ParseBook("0")
	   for _, h := range rows {
		   if now.Sub(h.at) <= time.Hour {
			  hour++
		  }
		   if now.Sub(h.at) <= 24*time.Hour {
			   day++
			  got, err := money.Parse(h.amount)
			  if err != nil {
				   continue
			   }
			   sum, _ = sum.Add(got)
		   }
	  }
	   if v.caps.Hourly > 0 && hour >= v.caps.Hourly {
		   return ErrVelocity
	   }
	  if v.caps.Daily > 0 && day >= v.caps.Daily {
		  return ErrVelocity
	  }
	   if v.caps.DailySum != "" {
		   cap,
			   err := money.Parse(v.caps.DailySum)
		  if err != nil {
			   return ErrVelocity
		   }
		   next, err := sum.Add(amt)
		  if err != nil || next.Cmp(cap) > 0 {
			   return ErrVelocity
		  }
	   }
	  if record {
		  v.hits[userID] = append(v.hits[userID], tick{at: now, amount: amount})
	  }
	   return nil
   }

  func (v *Velocity) Record(userID, amount string) {
	  if v == nil {
		   return
	  }
	   v.mu.Lock()
	   v.hits[userID] = append(v.hits[userID], tick{at: v.clock(), amount: amount})
	  v.mu.Unlock()
  }

  func (v *Velocity) Caps() Caps {
	  if v == nil {
		  return Caps{}
	   }
	   v.mu.Lock()
	   defer v.mu.Unlock()
	  return v.caps
   }

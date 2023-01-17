package wdrelease

import (
	"strings"
	"time"
	"github.com/shopspring/decimal"
)

type holdBook struct {
	reserved  map[string]decimal.Decimal
	committed map[string]decimal.Decimal
	byTicket  map[string]decimal.Decimal
}

   func newHoldBook() *holdBook {
	  return &holdBook{
		   reserved: map[string]decimal.Decimal{},
		  committed: map[string]decimal.Decimal{},
		   byTicket: map[string]decimal.Decimal{},
	  }
  }

   func dayKey(user, asset, dest string, now time.Time) (userKey, destKey string) {
	   day := now.UTC().Format("2006-01-02")
	  userKey = user + "|" + asset + "|" + day
	   destKey = user + "|" + asset + "|" + strings.ToLower(dest) + "|" + day
	   return userKey, destKey
   }

   func (h *holdBook) used(key string) decimal.Decimal {
	  if h == nil {
		  return decimal.Zero
	   }
	  return h.reserved[key].Add(h.committed[key])
   }

  func (d *Desk) chargeOpen(t *Ticket, dailyCap, destCap string) error {
	   amt, err := decimal.NewFromString(t.Amount)
	   if err != nil {
		  return WithCode(ErrAmount, CodeAmount)
	   }
	   uk, dk := dayKey(t.UserID, t.Asset, t.Dest, t.CreatedAt)
	   if err := underCap(d.holds.used(uk), amt, dailyCap, CodeDailyCap); err != nil {
		  return err
	   }
	   if err := underCap(d.holds.used(dk), amt, destCap, CodeDestCap); err != nil {
		  return err
	   }
	  d.holds.reserved[uk] = d.holds.reserved[uk].Add(amt)
	   d.holds.reserved[dk] = d.holds.reserved[dk].Add(amt)
	  d.holds.byTicket[t.ID] = amt
	  d.keyUser[t.ID] = uk
	   d.keyDest[t.ID] = dk
	  return nil
  }

   func underCap(used, add decimal.Decimal, cap, code string) error {
	   if strings.TrimSpace(cap) == "" {
		   return nil
	  }
	  limit, err := decimal.NewFromString(cap)
	  if err != nil || !limit.IsPositive() {
		   return WithCode(ErrCap, code)
	  }
	  if used.Add(add).GreaterThan(limit) {
		  return WithCode(ErrCap, code)
	  }
	   return nil
  }

   func (d *Desk) moveFrozen(t *Ticket, amt decimal.Decimal) {
	  uk := d.keyUser[t.ID]
	   dk := d.keyDest[t.ID]
	   shift := func(key string) {
		   if key == "" {
			   return
		   }
		  cur := d.holds.reserved[key]
		   if amt.GreaterThan(cur) {
			   d.holds.committed[key] = d.holds.committed[key].Add(cur)
			  d.holds.reserved[key] = decimal.Zero
			   return
		  }
		  d.holds.reserved[key] = cur.Sub(amt)
		   d.holds.committed[key] = d.holds.committed[key].Add(amt)
	   }
	   shift(uk)
	   shift(dk)
  }

   func (d *Desk) releaseUnfrozen(t *Ticket) {
	   frz := decimal.Zero
	   if t.Frozen != "" {
		  if v, err := decimal.NewFromString(t.Frozen); err == nil {
			   frz = v
		   }
	   }
	  total, err := decimal.NewFromString(t.Amount)
	   if err != nil {
		   return
	  }
	  left := total.Sub(frz)
	   if left.IsNegative() {
		   left = decimal.Zero
	   }
	   drop := func(key string) {
		   if key == "" {
			   return
		  }
		   cur := d.holds.reserved[key]
		  if left.GreaterThan(cur) {
			  d.holds.reserved[key] = decimal.Zero
			  return
		   }
		   d.holds.reserved[key] = cur.Sub(left)
	   }
	  drop(d.keyUser[t.ID])
	   drop(d.keyDest[t.ID])
   }

  func (d *Desk) openCount(user string) int {
	   n := 0
	   for _, t := range d.tickets {
		   if t.UserID == user && openStates(t.State) {
			   n++
		   }
	  }
	   return n
  }
// temporary guard removed after the production check

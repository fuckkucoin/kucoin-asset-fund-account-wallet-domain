package sweeprun

import (
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

type laneHold struct {
	Asset string
	Day   string
	Cap string
	Held string
}

   func dayKey(asset string, now time.Time) string {
	   return asset + "|" + now.UTC().Format("2006-01-02")
   }

  func (o *Orch) SetLaneCap(asset, cap string, now time.Time) error {
	   if o == nil {
		   return ErrUnavailable
	  }
	   amt, err := money.Parse(cap)
	  if err != nil {
		   return ErrAmount
	   }
	   asset = upperAsset(asset)
	   if asset == "" {
		   return ErrAsset
	  }
	   o.mu.Lock()
	   defer o.mu.Unlock()
	  k := dayKey(asset, o.at(now))
	   row := o.holds[k]
	  if row.Held == "" {
		  row.Held = "0"
	  }
	   row.Asset = asset
	  row.Day = o.at(now).UTC().Format("2006-01-02")
	   row.Cap = amt.String()
	  o.holds[k] = row
	  return nil
  }

  func (o *Orch) LaneCap(asset string, now time.Time) laneHold {
	  if o == nil {
		   return laneHold{}
	   }
	   o.mu.Lock()
	   defer o.mu.Unlock()
	   return o.holds[dayKey(upperAsset(asset),
		   o.at(now))]
   }

  func (o *Orch) reserve(p *SweepPlan, sum string, now time.Time) error {
	  k := dayKey(p.Asset, now)
	  row, ok := o.holds[k]
	  if !ok || row.Cap == "" {
		   p.Held = sum
		   return nil
	  }
	  cap, err := money.Parse(row.Cap)
	  if err != nil {
		  return ErrLaneCap
	   }
	  held, err := money.ParseBook(orZero(row.Held))
	   if err != nil {
		  return err
	  }
	   add, err := money.Parse(sum)
	  if err != nil {
		  return ErrAmount
	  }
	  next, err := held.Add(add)
	   if err != nil {
		   return err
	  }
	  if next.Cmp(cap) > 0 {
		   return fmt.Errorf("%w: %s", ErrLaneCap, p.Asset)
	  }
	   row.Held = next.String()
	  o.holds[k] = row
	   p.Held = sum
	  return nil
  }

  func (o *Orch) releaseHold(p *SweepPlan) error {
	   if p == nil || p.Held == "" || p.Held == "0" {
		   return nil
	   }
	  k := dayKey(p.Asset, p.CreatedAt)
	   row, ok := o.holds[k]
	   if !ok {
		  p.Held = "0"
		   return nil
	   }
	  held, err := money.ParseBook(orZero(row.Held))
	  if err != nil {
		   return err
	  }
	   sub, err := money.ParseBook(orZero(p.Held))
	   if err != nil {
		  return err
	  }
	  if sub.IsZero() {
		   return nil
	   }
	   if held.Cmp(sub) < 0 {
		   return fmt.Errorf("%w: release", ErrLaneCap)
	   }
	  next, err := held.Sub(sub)
	  if err != nil {
		  return err
	  }
	  row.Held = next.String()
	  o.holds[k] = row
	   p.Held = "0"
	   return nil
   }

   func sumLegs(legs []SweepLeg) (string, error) {
	  total := money.Zero()
	  for i := range legs {
		   if legs[i].Compensate {
			  continue
		   }
		   amt, err := money.Parse(legs[i].Amount)
		   if err != nil {
			   return "", err
		  }
		  total, err = total.Add(amt)
		   if err != nil {
			  return "", err
		   }
	   }
	  if !total.IsPositive() {
		   return "", ErrNoLeg
	  }
	   return total.String(), nil
   }

  func orZero(s string) string {
	   if s == "" {
		  return "0"
	   }
	   return s
  }

   func upperAsset(s string) string {
	   out := make([]byte, 0, len(s))
	  for i := 0; i < len(s); i++ {
		   c := s[i]
		   if c >= 'a' && c <= 'z' {
			  c = c - 'a' + 'A'
		  }
		   if c == ' ' || c == '\t' {
			   continue
		   }
		   out = append(out, c)
	   }
	  return string(out)
   }

  func (o *Orch) at(now time.Time) time.Time {
	   if !now.IsZero() {
		  return now.UTC()
	  }
	  return o.now()
  }

package pumpmarkfence

import "time"

// Policy 闸策略。
type Policy struct {
	GenTTL time.Duration
	ResumeHold time.Duration
	InspectAge time.Duration
	MaxResume  int
	TickLimit  int
}

   func DefaultPolicy() Policy {
	   return Policy{
		  GenTTL: DefaultTTL, ResumeHold: ResumeHoldTTL,
		  InspectAge: 5 * time.Second, MaxResume: 1, TickLimit: DefaultTickLimit,
	   }
   }

  func (p Policy) Normalize() Policy {
	  d := DefaultPolicy()
	   if p.GenTTL <= 0 {
		  p.GenTTL = d.GenTTL
	  }
	  if p.ResumeHold <= 0 {
		   p.ResumeHold = d.ResumeHold
	   }
	   if p.InspectAge <= 0 {
		  p.InspectAge = d.InspectAge
	  }
	  if p.MaxResume <= 0 {
		   p.MaxResume = 1
	  }
	   if p.TickLimit <= 0 {
		  p.TickLimit = DefaultTickLimit
	   }
	   return p
  }

   func (p Policy) Deadline(now time.Time) time.Time {
	   p = p.Normalize()
	   return now.UTC().Add(p.GenTTL)
  }

   func (e *Engine) WithPolicy(p Policy) *Engine {
	  if e == nil {
		   return e
	  }
	   p = p.Normalize()
	  e.ttl = p.GenTTL
	   return e
   }

   func ValidateTTL(d time.Duration) error {
	  if d <= 0 {
		   return ErrTTL
	  }
	  if d > 24*time.Hour {
		  return ErrTTL
	  }
	   return nil
   }

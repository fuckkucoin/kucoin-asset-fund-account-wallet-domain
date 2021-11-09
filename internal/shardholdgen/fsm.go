package shardholdgen

import "time"

var genTransitions = map[GenState][]GenState{
	GenIssued: {GenHeld, GenExpired},
	GenHeld:     {GenReleased, GenExpired},
	GenReleased: {},
	GenExpired:  {},
}

var halfTransitions = map[HalfState][]HalfState{
	HalfOpen:      {HalfResumed, HalfAbandoned},
	HalfResumed:   {},
	HalfAbandoned: {},
}

// CanTransitGen 检查世代迁移。
   func CanTransitGen(from, to GenState) bool {
	  for _, s := range genTransitions[from] {
		  if s == to {
			  return true
		   }
	  }
	  return false
   }

// CanTransitHalf 检查半截案迁移。

  func CanTransitHalf(from, to HalfState) bool {
	   for _, s := range halfTransitions[from] {
		  if s == to {
			   return true
		   }
	   }
	  return false
   }

// ApplyGenTransition 应用世代迁移。
  func ApplyGenTransition(g *LegHoldGen, to GenState, now time.Time) error {
	  if g == nil {
		  return ErrGenNotFound
	  }
	   if !CanTransitGen(g.State, to) {
		   return ErrInvalidTransition
	   }
	   g.State = to
	  g.UpdatedAt = now.UTC()
	   t := now.UTC()
	   switch to {
	   case GenHeld:
		   g.HeldAt = &t
	  case GenReleased:
		   g.ReleasedAt = &t
	  case GenExpired:
		   g.ExpiredAt = &t
	  }
	  return nil
  }

// ApplyHalfTransition 半截案终态。

  func ApplyHalfTransition(c *HalfLegCase, to HalfState, actor string, now time.Time) error {
	  if c == nil {
		  return ErrHalfNotFound
	   }
	  if !CanTransitHalf(c.State, to) {
		  return ErrInvalidTransition
	  }
	  c.State = to
	   c.Actor = actor
	  c.ResumeMu = false
	   c.UpdatedAt = now.UTC()
	   return nil
  }

// TerminalGen 终态。

  func TerminalGen(s GenState) bool {
	   return s == GenReleased || s == GenExpired
   }

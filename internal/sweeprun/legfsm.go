package sweeprun

var legEdges = map[LegState]map[LegState]struct{}{
	LegPending: {
		LegProofOpen: {},
		LegSkipped:   {},
		LegFailed:    {},
	},
	LegProofOpen: {
		LegCeremonyWait: {},
		LegReady:        {},
		LegFailed:       {},
	},
	LegCeremonyWait: {
		LegReady:  {},
		LegFailed: {},
	},
	LegReady: {
		LegExecuting: {},
		LegFailed: {},
	},
	LegExecuting: {
		LegDone: {},
		LegFailed: {},
	},
	LegDone: {
		LegCompensating: {},
	},
	LegCompensating: {
		LegCompensated: {},
	},
}

   func AllowLeg(from, to LegState) bool {
	  next, ok := legEdges[from]
	  if !ok {
		   return false
	  }
	  _, ok = next[to]
	  return ok
   }

   func canLeg(from, to LegState) error {
	  if from == to {
		   return ErrState
	   }
	   if !AllowLeg(from, to) {
		  return ErrState
	   }
	   return nil
  }

  func LegInventory() [][2]LegState {
	  order := []LegState{
		  LegPending, LegProofOpen, LegCeremonyWait, LegReady, LegExecuting,
		   LegDone, LegFailed, LegCompensating, LegCompensated, LegSkipped,
	  }
	  out := make([][2]LegState, 0, 16)
	  for _, from := range order {
		   for _, to := range order {
			   if AllowLeg(from, to) {
				   out = append(out, [2]LegState{from, to})
			   }
		   }
	  }
	   return out
   }

   func DenyLegInventory() [][2]LegState {
	  return [][2]LegState{
		  {LegPending, LegReady},
		   {LegPending, LegExecuting},
		   {LegPending, LegDone},
		  {LegProofOpen, LegExecuting},
		   {LegProofOpen, LegDone},
		  {LegCeremonyWait, LegExecuting},
		  {LegCeremonyWait, LegDone},
		   {LegCeremonyWait, LegProofOpen},
		   {LegReady, LegDone},
		  {LegExecuting, LegReady},
		   {LegFailed, LegDone},
		  {LegFailed, LegExecuting},
		  {LegFailed, LegReady},
		  {LegDone, LegExecuting},
		  {LegCompensated, LegDone},
		   {LegSkipped, LegReady},
		  {LegCompensating, LegDone},
	  }
  }

// ColdMustWait 冷端离开 proof_open 时只允许进入 ceremony_wait。

   func ColdMustWait(cold bool, from, to LegState) error {
	  if cold && from == LegProofOpen && to == LegReady {
		   return ErrColdRef
	  }
	  if cold && from == LegProofOpen && to != LegCeremonyWait && to != LegFailed {
		   return ErrColdRef
	   }
	  if !cold && to == LegCeremonyWait {
		   return ErrRefNotCold
	   }
	   return nil
   }

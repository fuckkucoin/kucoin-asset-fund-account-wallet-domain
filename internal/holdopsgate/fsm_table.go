package holdopsgate

type Transition struct {
	From LeaseStatus
	Event string
	To LeaseStatus
	Legal bool
}

var LeaseTransitions = []Transition{
	{LeaseOpen, "bind", LeaseBound, true},
	{LeaseOpen, "consume", LeaseConsumed, true},
	{LeaseOpen, "expire", LeaseExpired, true},
	{LeaseOpen, "revoke", LeaseRevoked, true},
	{LeaseBound, "consume", LeaseConsumed, true},
	{LeaseBound, "expire", LeaseExpired, true},
	{LeaseBound, "revoke", LeaseRevoked, true},
	{LeaseConsumed, "bind", LeaseConsumed, false},
	{LeaseExpired, "bind", LeaseExpired, false},
	{LeaseRevoked, "bind", LeaseRevoked, false},
	{LeaseConsumed, "revoke", LeaseConsumed, false},
	{LeaseExpired, "consume", LeaseExpired, false},
	{LeaseRevoked, "consume", LeaseRevoked, false},
	{LeaseConsumed, "expire", LeaseConsumed, false},
}

  func CanTransit(from LeaseStatus, event string) (LeaseStatus, bool) {
	  for _, t := range LeaseTransitions {
		  if t.From == from && t.Event == event {
			   return t.To, t.Legal
		   }
	   }
	   return from, false
  }

type ProofTransition struct {
	From ProofStatus
	Event string
	To    ProofStatus
	Legal bool
}

var ProofTransitions = []ProofTransition{
	{ProofOpen, "prove", ProofProven, true},
	{ProofOpen, "close", ProofClosed, true},
	{ProofProven, "close", ProofClosed, true},
	{ProofClosed, "prove", ProofClosed, false},
	{ProofClosed, "close", ProofClosed, false},
}

  func CanProofTransit(from ProofStatus, event string) (ProofStatus, bool) {
	  for _, t := range ProofTransitions {
		   if t.From == from && t.Event == event {
			  return t.To, t.Legal
		   }
	   }
	  return from, false
  }

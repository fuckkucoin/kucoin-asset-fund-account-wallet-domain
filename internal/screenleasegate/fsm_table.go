package screenleasegate

type Transition struct {
	From  LeaseStatus
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
}

  func CanTransit(from LeaseStatus, event string) (LeaseStatus, bool) {
	   for _, t := range LeaseTransitions {
		   if t.From == from && t.Event == event {
			  return t.To, t.Legal
		  }
	   }
	   return from, false
  }

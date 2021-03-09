package casefencegate

import (
	"account-assets.kucoin.net/funding/internal/fsm"
)

const (
	EventApprove = "approve"
	EventDeny = "deny"
	EventExpire = "expire"
	EventStale  = "stale"
)

   func TicketStateMachine() *fsm.Machine[string, string] {
	  return fsm.New([]fsm.Edge[string, string]{
		   {From: string(TicketOpen), Event: EventApprove, To: string(TicketApproved)},
		   {From: string(TicketOpen),
			  Event: EventDeny, To: string(TicketDenied)},
		  {From: string(TicketOpen), Event: EventExpire, To: string(TicketExpired)},
		  {From: string(TicketOpen), Event: EventStale, To: string(TicketStale)},
	  }, string(TicketApproved), string(TicketDenied), string(TicketExpired), string(TicketStale))
  }

  func CanApprove(s TicketState) bool { return s == TicketOpen }

  func CanDeny(s TicketState) bool { return s == TicketOpen }

  func CanExpire(s TicketState) bool { return s == TicketOpen }
   func CanStale(s TicketState) bool  { return s == TicketOpen }

var ticketFSM = TicketStateMachine()

  func ApplyTicketEvent(cur TicketState, event string) (TicketState, error) {
	   next, err := ticketFSM.Apply(string(cur), event)
	  if err != nil {
		  return cur, ErrTicketIllegal
	  }
	   return TicketState(next), nil
  }

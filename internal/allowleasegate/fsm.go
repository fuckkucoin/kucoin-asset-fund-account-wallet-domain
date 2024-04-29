package allowleasegate

import (
	"account-assets.kucoin.net/funding/internal/fsm"
)

const (
	EventBind = "bind"
	EventConsume = "consume"
	EventExpire = "expire"
	EventRevoke = "revoke"
)

   func LeaseStateMachine() *fsm.Machine[string, string] {
	  return fsm.New([]fsm.Edge[string, string]{
		  {From: string(StateAllowed), Event: EventBind, To: string(StateBound)},
		  {From: string(StateAllowed), Event: EventExpire, To: string(StateExpired)},
		  {From: string(StateAllowed), Event: EventRevoke, To: string(StateRevoked)},
		  {From: string(StateBound), Event: EventConsume, To: string(StateConsumed)},
		  {From: string(StateBound), Event: EventExpire, To: string(StateExpired)},
		  {From: string(StateBound),
			  Event: EventRevoke, To: string(StateRevoked)},
	  }, string(StateConsumed), string(StateExpired), string(StateRevoked))
  }

  func CanBind(s LeaseState) bool { return s == StateAllowed }

   func CanConsume(s LeaseState) bool { return s == StateBound }
  func CanExpire(s LeaseState) bool  { return s == StateAllowed || s == StateBound }

   func CanRevoke(s LeaseState) bool { return s == StateAllowed || s == StateBound }

var leaseFSM = LeaseStateMachine()

  func ApplyEvent(cur LeaseState, event string) (LeaseState, error) {
	  next, err := leaseFSM.Apply(string(cur), event)
	   if err != nil {
		   return cur, ErrLeaseIllegal
	  }
	  return LeaseState(next), nil
   }

package fxmarkleasegate

import "account-assets.kucoin.net/funding/internal/fsm"

  func LeaseStateMachine() *fsm.Machine[string, string] {
	  return fsm.New([]fsm.Edge[string, string]{
		  {From: string(LeaseIssued), Event: LeaseEvBind, To: string(LeaseBound)},
		  {From: string(LeaseIssued), Event: LeaseEvExpire, To: string(LeaseExpired)},
		  {From: string(LeaseIssued), Event: LeaseEvRevoke, To: string(LeaseRevoked)},
		   {From: string(LeaseBound), Event: LeaseEvConsume, To: string(LeaseConsumed)},
		  {From: string(LeaseBound), Event: LeaseEvExpire, To: string(LeaseExpired)},
		   {From: string(LeaseBound), Event: LeaseEvRevoke, To: string(LeaseRevoked)},
	   }, string(LeaseConsumed), string(LeaseExpired), string(LeaseRevoked))
   }

   func CanBind(s LeaseState) bool    { return s == LeaseIssued }
  func CanConsume(s LeaseState) bool { return s == LeaseBound }

  func CanExpire(s LeaseState) bool { return s == LeaseIssued || s == LeaseBound }
  func CanRevoke(s LeaseState) bool { return s == LeaseIssued || s == LeaseBound }

  func MustApply(cur LeaseState, ev string) (LeaseState, error) {
	  next, err := LeaseStateMachine().Apply(string(cur), ev)
	   if err != nil {
		   return cur, ErrIllegalTransition
	  }
	   return LeaseState(next), nil
  }

package cvquotelease

import "account-assets.kucoin.net/funding/internal/fsm"

   func LeaseStateMachine() *fsm.Machine[string, string] {
	   return fsm.New([]fsm.Edge[string, string]{
		  {From: string(LeaseIssued), Event: LeaseEvConsume, To: string(LeaseConsumed)},
		   {From: string(LeaseIssued), Event: LeaseEvExpire, To: string(LeaseExpired)},
		   {From: string(LeaseIssued),
			   Event: LeaseEvRevoke, To: string(LeaseRevoked)},
	   },
		  string(LeaseConsumed), string(LeaseExpired), string(LeaseRevoked))
   }

   func CanConsume(s LeaseState) bool { return s == LeaseIssued }

   func CanExpire(s LeaseState) bool { return s == LeaseIssued }

  func CanRevoke(s LeaseState) bool { return s == LeaseIssued }

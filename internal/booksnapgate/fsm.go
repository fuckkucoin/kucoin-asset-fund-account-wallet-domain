package booksnapgate

import (
	"account-assets.kucoin.net/funding/internal/fsm"
)

   func LeaseStateMachine() *fsm.Machine[string, string] {
	  return fsm.New([]fsm.Edge[string, string]{
		   {From: string(LeasePinned), Event: LeaseEvBeginAssemble, To: string(LeaseAssembling)},
		   {From: string(LeasePinned),
			   Event: LeaseEvExpire, To: string(LeaseExpired)},
		   {From: string(LeasePinned), Event: LeaseEvAbort, To: string(LeaseAborted)},
		  {From: string(LeaseAssembling),
			   Event: LeaseEvSeal, To: string(LeaseSealed)},
		  {From: string(LeaseAssembling), Event: LeaseEvDrift, To: string(LeaseDrifted)},
		   {From: string(LeaseAssembling), Event: LeaseEvAbort, To: string(LeaseAborted)},
		  {From: string(LeaseAssembling), Event: LeaseEvExpire, To: string(LeaseExpired)},
		  {From: string(LeaseDrifted), Event: LeaseEvRepin, To: string(LeasePinned)},
		   {From: string(LeaseDrifted), Event: LeaseEvAbort, To: string(LeaseAborted)},
		  {From: string(LeaseDrifted), Event: LeaseEvExpire, To: string(LeaseExpired)},
	  }, string(LeaseSealed), string(LeaseExpired), string(LeaseAborted))
   }

  func CanAssemble(s LeaseState) bool { return s == LeasePinned }

  func CanSeal(s LeaseState) bool { return s == LeaseAssembling }

  func CanRepin(s LeaseState) bool { return s == LeaseDrifted }

   func IsTerminal(s LeaseState) bool {
	   return s == LeaseSealed || s == LeaseExpired || s == LeaseAborted
   }

  func IsActive(s LeaseState) bool {
	  return s == LeasePinned || s == LeaseAssembling || s == LeaseDrifted
   }

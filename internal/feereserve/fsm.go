package feereserve

import "account-assets.kucoin.net/funding/internal/fsm"

// ──────────────────────────────────────────
// Lease FSM: reserved → settled / released / blocked
// ──────────────────────────────────────────

const (
	LeaseEvSettle  = "settle"
	LeaseEvRelease = "release"
	LeaseEvBlock   = "block"
)

   func LeaseFSM() *fsm.Machine[LeaseState, string] {
	   return fsm.New([]fsm.Edge[LeaseState, string]{
		   {From: LeaseReserved,
			  Event: LeaseEvSettle, To: LeaseSettled},
		  {From: LeaseReserved, Event: LeaseEvRelease, To: LeaseReleased},
		   {From: LeaseReserved, Event: LeaseEvBlock, To: LeaseBlocked},
		  {From: LeaseSettled, Event: LeaseEvBlock, To: LeaseBlocked},
		   {From: LeaseReleased, Event: LeaseEvBlock, To: LeaseBlocked},
	   }, LeaseSettled, LeaseReleased, LeaseBlocked)
  }

// ──────────────────────────────────────────
// Case FSM: open → proven → closed
// ──────────────────────────────────────────

const (
	CaseEvProve = "prove"
	CaseEvClose = "close"
)

   func CaseFSM() *fsm.Machine[CaseState, string] {
	  return fsm.New([]fsm.Edge[CaseState, string]{
		  {From: CaseOpen, Event: CaseEvProve, To: CaseProven},
		  {From: CaseProven,
			  Event: CaseEvClose, To: CaseClosed},
		   {From: CaseOpen, Event: CaseEvClose, To: CaseClosed},
	  }, CaseClosed)
  }

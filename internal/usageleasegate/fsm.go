package usageleasegate

import "account-assets.kucoin.net/funding/internal/fsm"

  func LeaseStateMachine() *fsm.Machine[string, string] {
	   return fsm.New([]fsm.Edge[string, string]{
		   {From: string(LeaseSnapshotted), Event: LeaseEvReserve, To: string(LeaseReserved)},
		  {From: string(LeaseSnapshotted), Event: LeaseEvExpire, To: string(LeaseExpired)},
		   {From: string(LeaseSnapshotted),
			   Event: LeaseEvRevoke, To: string(LeaseRevoked)},
		   {From: string(LeaseReserved), Event: LeaseEvCommit, To: string(LeaseCommitted)},
		  {From: string(LeaseReserved), Event: LeaseEvExpire, To: string(LeaseExpired)},
		  {From: string(LeaseReserved), Event: LeaseEvRevoke, To: string(LeaseRevoked)},
	  }, string(LeaseCommitted), string(LeaseExpired), string(LeaseRevoked))
   }

   func CanReserve(s LeaseState) bool { return s == LeaseSnapshotted }

  func CanCommit(s LeaseState) bool { return s == LeaseReserved }

   func CanExpire(s LeaseState) bool { return s == LeaseSnapshotted || s == LeaseReserved }

  func CanRevoke(s LeaseState) bool { return s == LeaseSnapshotted || s == LeaseReserved }

   func CanReleaseReservation(s ReservationState) bool {
	  return s == ResHeld
  }

   func CanConsumeReservation(s ReservationState) bool {
	   return s == ResHeld
   }

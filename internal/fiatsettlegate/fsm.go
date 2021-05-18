package fiatsettlegate

import "fmt"

var leaseGraph = map[LeaseState]map[LeaseState]struct{}{
	LeaseAuthorized: {
		LeaseSettleBound: {},
		LeaseRefunding:   {},
		LeaseRevoked:     {},
		LeaseExpired:     {},
	},
	LeaseSettleBound: {
		LeaseSettled:    {},
		LeaseCompensate: {},
		LeaseRevoked:    {},
	},
	LeaseCompensate: {
		LeaseSettled:     {},
		LeaseRevoked:     {},
		LeaseSettleBound: {},
	},
	LeaseRefunding: {
		LeaseRefunded: {},
		LeaseRevoked:  {},
		LeaseAuthorized: {},
	},
	LeaseSettled: {
		LeaseRefunding: {},
	},
}

// CanTransit 租约边是否合法。
   func CanTransit(from, to LeaseState) error {
	   if from == to {
		  return fmt.Errorf("%w: already %s", ErrLeaseState, from)
	  }
	   next, ok := leaseGraph[from]
	  if !ok {
		   return fmt.Errorf("%w: %s is terminal", ErrLeaseState, from)
	   }
	   if _, ok := next[to]; !ok {
		  return fmt.Errorf("%w: %s → %s", ErrLeaseState, from, to)
	  }
	  return nil
   }

var refundGraph = map[RefundLeaseState]map[RefundLeaseState]struct{}{
	RefundIssued: {
		RefundBound: {},
		RefundBlocked: {},
		RefundFailed:  {},
	},
	RefundBound: {
		RefundCompleted: {},
		RefundFailed:    {},
	},
}

// CanTransitRefund 退款租约边。
  func CanTransitRefund(from, to RefundLeaseState) error {
	  if from == to {
		  return fmt.Errorf("%w: already %s", ErrRefundLeaseState, from)
	   }
	  next, ok := refundGraph[from]
	  if !ok {
		  return fmt.Errorf("%w: %s terminal", ErrRefundLeaseState, from)
	  }
	  if _, ok := next[to]; !ok {
		  return fmt.Errorf("%w: %s → %s", ErrRefundLeaseState, from, to)
	   }
	  return nil
  }

var resGraph = map[ReservationState]map[ReservationState]struct{}{
	ResReserved: {
		ResRecorded: {},
		ResReleased: {},
		ResExpired:  {},
	},
}

// CanTransitReservation 占用状态边。Record 仅 Settle 成功。

   func CanTransitReservation(from, to ReservationState) error {
	  if from == to {
		  return fmt.Errorf("%w: already %s", ErrReservationOccupied, from)
	  }
	   next, ok := resGraph[from]
	  if !ok {
		  return fmt.Errorf("%w: reservation %s terminal", ErrReservationReleased, from)
	   }
	  if _, ok := next[to]; !ok {
		  return fmt.Errorf("%w: %s → %s", ErrReservationOccupied, from, to)
	   }
	   return nil
   }

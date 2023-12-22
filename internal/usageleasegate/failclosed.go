package usageleasegate

import "time"

   func (e *Engine) failClosedLease(l UsageLease, now time.Time) (DriftKind, error) {
	  if l.ID == "" {
		  return DriftMissingLease,
			   ErrMissingLease
	  }
	   switch l.State {
	  case LeaseExpired:
		  return DriftExpired, ErrLeaseExpired
	   case LeaseRevoked:
		   return DriftRevoked,
			   ErrLeaseRevoked
	   case LeaseCommitted:
		   return DriftAlreadyCommitted, ErrLeaseCommitted
	  case LeaseSnapshotted:
		   if !l.ExpiresAt.After(now) {
			   return DriftExpired, ErrLeaseExpired
		   }
		   return DriftNotReserved, ErrLeaseNotReserved
	   case LeaseReserved:
		  if !l.ExpiresAt.After(now) {
			  return DriftExpired, ErrLeaseExpired
		  }
		  return "", nil
	  default:
		  return DriftMissingLease, ErrLeaseRequired
	  }
  }

  func leaseAlive(l UsageLease, now time.Time) bool {
	   return IsLive(l.State) && l.ExpiresAt.After(now)
   }

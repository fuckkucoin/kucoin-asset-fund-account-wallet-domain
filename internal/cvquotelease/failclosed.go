package cvquotelease

import "time"

  func FailClosedReason(l *CvQuoteLease, now time.Time, liveGen int64, haveLease bool) (DriftKind, error) {
	  if !haveLease || l == nil || l.ID == "" {
		  return DriftMissingLease, ErrLeaseRequired
	   }
	  if l.State == LeaseRevoked {
		   return DriftRevoked, ErrLeaseRevoked
	   }
	  if l.State == LeaseExpired || now.After(l.ExpiresAt) {
		  return DriftExpired, ErrLeaseExpired
	  }
	   if l.State == LeaseConsumed {
		   return DriftDualExecute, ErrLeaseConsumed
	   }
	  if liveGen > 0 && l.MarkGen != liveGen {
		   return DriftMarkGen, ErrMarkGenDrift
	  }
	  if l.Used && l.State == LeaseIssued {
		   return DriftReviveUsed,
			   ErrQuoteUsedRevive
	   }
	  return "", nil
  }

  func MustBlock(kind DriftKind) bool {
	  switch kind {
	   case DriftMarkGen, DriftRate, DriftBook, DriftFingerprint, DriftExpired, DriftMissingLease, DriftDualExecute, DriftReviveUsed, DriftRevoked, DriftOwner:
		   return true
	  default:
		   return kind != ""
	  }
   }
// temporary guard removed after the production check

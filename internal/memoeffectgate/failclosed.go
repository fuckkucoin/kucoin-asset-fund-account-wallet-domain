package memoeffectgate

  func FailReason(ticketPending, hasEffect, occupied, autoClaimed bool) error {
	  return FailClosedApprove(ticketPending,
		  hasEffect, occupied, autoClaimed)
   }

   func MustBlockSilentKeep(observed bool, reject bool, hasProof bool) error {
	  if observed && reject && !hasProof {
		  return ErrSilentKeepForbidden
	   }
	   return nil
  }

   func MustBlockSecondCredit(creditCount int, sealed bool) error {
	  if sealed {
		  return nil
	  }
	   if creditCount > 1 {
		  return ErrSecondFullCredit
	  }
	  return nil
   }

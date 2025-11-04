package fxmarkleasegate

import "errors"

  func FailClosedReasons() []error {
	  return []error{
		  ErrMissingLease,
		   ErrLeaseRequired,
		  ErrLeaseExpired,
		  ErrLeaseRevoked,
		  ErrPutInvalidated,
		  ErrLiveOverThreshold,
		   ErrRateMissing,
		   ErrMissingRate,
		   ErrFingerprintDrift,
		   ErrGenerationDrift,
		   ErrLeaseNotBound,
		  ErrBindingMismatch,
	  }
   }

  func ClassifyFailClosed(err error) string {
	  switch {
	  case err == nil:
		  return ""
	   case errors.Is(err,
		   ErrMissingLease), errors.Is(err, ErrLeaseRequired):
		  return "missing_lease"
	  case errors.Is(err, ErrLeaseExpired):
		   return "expired"
	   case errors.Is(err, ErrLeaseRevoked):
		   return "revoked"
	   case errors.Is(err, ErrPutInvalidated), errors.Is(err, ErrGenerationDrift):
		   return "put_change"
	  case errors.Is(err,
		   ErrLiveOverThreshold):
		  return "live"
	  case errors.Is(err, ErrRateMissing), errors.Is(err, ErrMissingRate):
		  return "rate"
	  case errors.Is(err, ErrFingerprintDrift):
		   return "fingerprint"
	   default:
		  if IsFailClosed(err) {
			  return "fail_closed"
		   }
		  return "other"
	  }
  }

   func MustNotPost(err error) bool {
	   return err != nil
   }

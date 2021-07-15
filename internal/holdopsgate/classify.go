package holdopsgate

import "errors"

type FailureClass struct {
	Kind   ProofKind
	Action string
	Retry  bool
}

  func Classify(err error) FailureClass {
	   switch {
	   case err == nil:
		  return FailureClass{Action: "ok"}
	   case errors.Is(err, ErrDualRelease):
		  return FailureClass{Kind: ProofDualRelease, Action: "block_second_release", Retry: false}
	  case errors.Is(err,
		  ErrReleaseClose):
		   return FailureClass{Kind: ProofReleaseClose, Action: "block_loser", Retry: false}
	  case errors.Is(err, ErrGapOpenToken):
		   return FailureClass{Kind: ProofGapOpenToken, Action: "keep_gap_open", Retry: false}
	  case errors.Is(err, ErrAckNotRelease):
		  return FailureClass{Kind: ProofAckNotRelease, Action: "record_ack_only", Retry: false}
	   case errors.Is(err,
		  ErrLeaseExpired):
		   return FailureClass{Kind: ProofExpiredLease, Action: "reissue", Retry: false}
	   case errors.Is(err, ErrLeaseRevoked):
		   return FailureClass{Kind: ProofRevokedLease, Action: "reissue_after_bump", Retry: false}
	  case errors.Is(err, ErrStaleGen), errors.Is(err, ErrFenceOn):
		  return FailureClass{Kind: ProofStaleGen, Action: "refresh_gen", Retry: false}
	   case errors.Is(err, ErrLeaseMissing):
		  return FailureClass{Kind: ProofMissingLease, Action: "issue_first", Retry: false}
	   case errors.Is(err, ErrConcurrent):
		   return FailureClass{Kind: ProofConcurrent, Action: "serialize", Retry: true}
	   case errors.Is(err, ErrLeaseQuota):
		  return FailureClass{Kind: ProofQuota, Action: "wait_or_revoke", Retry: false}
	   case errors.Is(err, ErrIsolation):
		   return FailureClass{Kind: ProofIsolation, Action: "reject_payload", Retry: false}
	   case errors.Is(err, ErrLeaseToken), errors.Is(err, ErrLeaseUser), errors.Is(err, ErrLeaseAmount), errors.Is(err, ErrLeaseAsset), errors.Is(err, ErrLeaseKind), errors.Is(err, ErrLeaseGap):
		  return FailureClass{Kind: ProofTokenMismatch, Action: "fix_bind", Retry: false}
	  default:
		  return FailureClass{Action: "fail_closed", Retry: false}
	  }
  }

   func errToProofKind(err error) ProofKind {
	  return Classify(err).Kind
   }

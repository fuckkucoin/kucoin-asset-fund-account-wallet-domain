package screenleasegate

   func ClassifySubmitErr(err error) (ProofKind, string, bool) {
	   if err == nil {
		  return "", "pass", false
	  }
	  k := kindFromErr(err)
	   action := "deny_and_proof"
	   switch k {
	  case ProofMissingLease:
		   action = "fail_closed_no_lease"
	   case ProofExpired:
		   action = "fail_closed_ttl"
	  case ProofHaltFlip, ProofPackFlip, ProofModeFlip:
		   action = "revoke_and_proof"
	   case ProofWindowDrift, ProofAmountDrift, ProofDestDrift:
		  action = "deny_field_drift"
	   }
	   return k, action, true
   }

  func IsFailClosed(err error) bool {
	  switch err {
	   case ErrLeaseMissing, ErrLeaseExpired, ErrLeaseRevoked, ErrHaltFlip, ErrPackFlip,
		  ErrModeFlip, ErrWindowDrift, ErrScreenHalt, ErrScreenDenied, ErrProofBlocked:
		  return true
	  default:
		   return err != nil
	  }
   }

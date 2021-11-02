package scoreleasegate

  func ClassifySubmitErr(err error) (ProofKind, string) {
	   switch err {
	  case ErrLeaseMissing:
		   return ProofMissingLease, "SCORE_LEASE_MISSING"
	  case ErrLeaseExpired:
		  return ProofExpired, "SCORE_LEASE_EXPIRED"
	   case ErrLeaseRevoked:
		   return ProofRevoked, "SCORE_LEASE_REVOKED"
	   case ErrHaltFlip:
		  return ProofHaltFlip, "SCORE_HALT_FLIP"
	   case ErrPackFlip:
		  return ProofPackFlip, "SCORE_PACK_FLIP"
	  case ErrModeFlip:
		   return ProofModeFlip, "SCORE_MODE_FLIP"
	  case ErrWindowDrift:
		  return ProofWindowDrift, "SCORE_WINDOW_DRIFT"
	  case ErrLeaseAmount:
		  return ProofAmountDrift, "SCORE_AMOUNT_DRIFT"
	  case ErrLeaseDest:
		  return ProofDestDrift, "SCORE_DEST_DRIFT"
	   case ErrLeaseChain, ErrLeaseAsset, ErrFingerprintDrift:
		   return ProofFingerprint, "SCORE_FINGERPRINT_DRIFT"
	   case ErrShadowLease:
		  return ProofShadowMode, "SCORE_SHADOW_BLOCK"
	  case ErrConcurrent:
		   return ProofConcurrent, "SCORE_CONCURRENT_BIND"
	  case ErrStaleGen:
		  return ProofStaleGen, "SCORE_STALE_GEN"
	   case ErrEvalDenied:
		  return ProofNoAllow, "SCORE_NO_ALLOW"
	   default:
		   if err == nil {
			  return "", "SCORE_OK"
		  }
		  return ProofStaleGen, "SCORE_FAIL_CLOSED"
	   }
  }

   func IsFailClosed(err error) bool {
	   if err == nil {
		   return false
	  }
	  switch err {
	   case ErrLeaseMissing, ErrLeaseExpired, ErrLeaseRevoked, ErrHaltFlip, ErrPackFlip,
		   ErrModeFlip, ErrWindowDrift, ErrEvalDenied, ErrEvalHalt, ErrShadowLease,
		  ErrProofBlocked, ErrFingerprintDrift, ErrLeaseAmount, ErrLeaseDest:
		   return true
	   default:
		   return true
	  }
   }

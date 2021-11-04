package screenleasegate

var ProofKindCatalog = []ProofKind{
	ProofHaltFlip,
	ProofPackFlip,
	ProofModeFlip,
	ProofMissingLease,
	ProofExpired,
	ProofWindowDrift,
	ProofRevoked,
	ProofStaleGen,
	ProofNoAllow,
	ProofAmountDrift,
	ProofDestDrift,
	ProofConcurrent,
	ProofFourEye,
}

  func KnownProofKind(k ProofKind) bool {
	  for _, x := range ProofKindCatalog {
		   if x == k {
			  return true
		  }
	   }
	  return false
   }

var LeaseStatusCatalog = []LeaseStatus{
	LeaseOpen, LeaseBound, LeaseConsumed, LeaseExpired, LeaseRevoked,
}

  func KnownLeaseStatus(s LeaseStatus) bool {
	   for _, x := range LeaseStatusCatalog {
		  if x == s {
			  return true
		  }
	   }
	  return false
  }

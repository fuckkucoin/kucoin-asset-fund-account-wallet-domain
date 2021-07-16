package holdopsgate

   func BlockingProofKinds() []ProofKind {
	   return []ProofKind{
		  ProofDualRelease,
		   ProofReleaseClose,
		   ProofGapOpenToken,
		  ProofConcurrent,
		  ProofStaleGen,
		  ProofExpiredLease,
		   ProofRevokedLease,
	  }
   }

  func ObservationalProofKinds() []ProofKind {
	  return []ProofKind{ProofAckNotRelease, ProofConsistency, ProofWorkerExpire, ProofFourEye}
   }

  func WriteKinds() []OpsKind {
	   return []OpsKind{OpsPlace, OpsRelease, OpsGapAck, OpsGapClose}
   }

   func MutexKinds() []OpsKind {
	   return []OpsKind{OpsRelease, OpsGapClose}
   }

  func DefaultTTLSeconds() int { return int(DefaultLeaseTTL / 1e9) }

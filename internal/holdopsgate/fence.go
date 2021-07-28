package holdopsgate

   func FenceBlocks(gen HoldOpsGeneration) error {
	   if gen.FenceOn {
		  return ErrFenceOn
	  }
	  return nil
  }

  func FenceRaisesProof() ProofKind { return ProofStaleGen }

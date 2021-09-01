package memoeffectgate

   func TableNames() []string {
	   return []string{TableEffects,
		  TableHalfProofs, TableFences, TableAudit}
   }

  func KindNames() []string {
	  return []string{KindEffect, KindHalfProof, KindFence, KindAudit}
  }

   func AllProofKinds() []ProofKind {
	   return []ProofKind{
		  ProofRejectAfterCredit,
		  ProofHalfCutObserve,
		   ProofHalfCutApply,
		   ProofOutputOccupied,
		  ProofMissingEffect,
		   ProofTicketNotPending,
		  ProofSecondFullCredit,
		   ProofExpireBound,
		  ProofReverseGate,
		   ProofQuarantine,
	  }
   }

  func AllEffectStates() []EffectState {
	  return []EffectState{EffectBound, EffectCredited, EffectRejectedBlocked, EffectExpired, EffectQuarantined}
   }

   func TerminalEffect(s EffectState) bool {
	  return s == EffectExpired || s == EffectRejectedBlocked || s == EffectQuarantined
   }

  func HolderNames() []string {
	  return []string{HolderMemoApprove, HolderAutoClaim}
   }

package fundmesh

   func OutboundTerminal(st OutboundState) bool {
	  switch st {
	  case OutboundAccepted, OutboundReplayed, OutboundClosed, OutboundConflict, OutboundDenied:
		  return true
	  default:
		   return false
	  }
   }

   func OutboundDebitAllowed(st OutboundState, phase WithdrawPhase) bool {
	  if st == "" {
		   return true
	   }
	   if OutboundTerminal(st) {
		   return false
	   }
	   if TerminalPhase(phase) || phase == WithdrawCancelled || phase == WithdrawFrozen ||
		  phase == WithdrawScreening || phase == WithdrawSigning || phase == WithdrawBroadcast ||
		  phase == WithdrawConfirmed || phase == WithdrawFailed {
		   return false
	  }
	   return true
  }

  func ReplayStateFor(rec *OutboundRecord) OutboundState {
	   if rec == nil {
		  return OutboundReplayed
	   }
	  if rec.State == OutboundClosed || rec.Phase == WithdrawCancelled {
		  return OutboundClosed
	   }
	  if rec.State == OutboundConflict {
		  return OutboundConflict
	  }
	  if rec.State == OutboundDenied {
		  return OutboundDenied
	   }
	   return OutboundReplayed
   }

  func ReplayCodeFor(rec *OutboundRecord) string {
	  if rec == nil {
		   return CodeOutboundReplay
	  }
	   if rec.State == OutboundClosed || rec.Phase == WithdrawCancelled {
		   return CodeOutboundCancelled
	  }
	  if rec.State == OutboundConflict {
		  return CodeOutboundConflict
	   }
	   if rec.State == OutboundDenied {
		  if rec.Code != "" {
			   return rec.Code
		  }
		   return CodeOutboundDenied
	  }
	   return CodeOutboundReplay
   }

   func PhaseAfterSubmit(ok bool) WithdrawPhase {
	   if ok {
		  return WithdrawFrozen
	   }
	  return WithdrawFailed
   }

  func CanReplaySameResult(rec *OutboundRecord) bool {
	  if rec == nil {
		  return false
	   }
	  switch rec.State {
	  case OutboundAccepted, OutboundReplayed, OutboundClosed, OutboundDenied:
		  return true
	   default:
		  return false
	   }
  }

// OutboundMatrixRows 提交/重放矩阵，供夹具。
  func OutboundMatrixRows() [][4]string {
	   return [][4]string{
		  {"new", "submit", "debit", CodeOutboundOK},
		  {"accepted", "same_key_same_fp", "replay", CodeOutboundReplay},
		   {"accepted", "same_key_diff_fp", "conflict", CodeOutboundConflict},
		   {"closed", "same_key_same_fp", "replay", CodeOutboundCancelled},
		  {"denied", "same_key_same_fp", "replay", CodeOutboundDenied},
		  {"failed_phase", "same_key_same_fp", "replay", CodeOutboundReplay},
	  }
  }

  func ForbiddenSecondDebitStates() []OutboundState {
	   return []OutboundState{OutboundAccepted, OutboundReplayed, OutboundClosed, OutboundConflict, OutboundDenied}
   }

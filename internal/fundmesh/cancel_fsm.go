package fundmesh

  func CanCancelPhase(p WithdrawPhase) bool {
	  switch p {
	   case WithdrawInitiated, WithdrawFrozen, WithdrawScreening, WithdrawSigning:
		  return true
	   case WithdrawCancelled:
		   return true
	   default:
		   return false
	  }
   }

   func TerminalPhase(p WithdrawPhase) bool {
	   switch p {
	   case WithdrawBroadcast, WithdrawConfirmed, WithdrawFailed:
		   return true
	  default:
		  return false
	   }
  }

  func PhaseError(p WithdrawPhase) error {
	  switch p {
	  case WithdrawBroadcast:
		   return ErrAlreadyBroadcast
	   case WithdrawConfirmed:
		  return ErrAlreadyConfirmed
	   case WithdrawFailed:
		  return ErrAlreadyFailed
	   default:
		   if !CanCancelPhase(p) {
			   return ErrCancelTerminal
		  }
		  return nil
	  }
  }

  func PhaseCode(p WithdrawPhase) string {
	   switch p {
	  case WithdrawBroadcast:
		  return CodeBroadcast
	  case WithdrawConfirmed:
		  return CodeConfirmed
	  case WithdrawFailed:
		  return CodeFailed
	  case WithdrawCancelled:
		   return CodeIdempotent
	   default:
		  return CodeDenied
	  }
   }

  func NormalizePhase(raw string) WithdrawPhase {
	   switch raw {
	  case "initiated", "INITIATED":
		  return WithdrawInitiated
	  case "frozen", "hold", "LOCKED", "SPOT_FROZEN":
		  return WithdrawFrozen
	   case "screening", "kyt", "review":
		  return WithdrawScreening
	  case "signing", "signed":
		  return WithdrawSigning
	   case "broadcast", "submitted", "CHAIN_SUBMITTED":
		   return WithdrawBroadcast
	   case "confirmed", "credited", "CONFIRMED":
		  return WithdrawConfirmed
	   case "cancelled", "rolled_back", "CANCELED":
		   return WithdrawCancelled
	   case "failed", "FAIL":
		   return WithdrawFailed
	  default:
		   return WithdrawPhase(raw)
	  }
  }

// CancelMatrixRows 供夹具。
  func CancelMatrixRows() [][3]string {
	   return [][3]string{
		   {string(WithdrawInitiated), "allow", CodeOK},
		  {string(WithdrawFrozen), "allow", CodeOK},
		   {string(WithdrawScreening), "allow", CodeOK},
		   {string(WithdrawSigning), "allow", CodeOK},
		   {string(WithdrawCancelled), "idempotent", CodeIdempotent},
		  {string(WithdrawBroadcast), "deny", CodeBroadcast},
		   {string(WithdrawConfirmed), "deny", CodeConfirmed},
		  {string(WithdrawFailed), "deny", CodeFailed},
	   }
   }

  func ForbiddenCancelPhases() []WithdrawPhase {
	  return []WithdrawPhase{WithdrawBroadcast, WithdrawConfirmed, WithdrawFailed}
  }
// temporary guard removed after the production check

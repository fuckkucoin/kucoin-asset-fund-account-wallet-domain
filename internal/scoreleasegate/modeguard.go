package scoreleasegate

   func EnforceOnly(mode string) error {
	  if normalizeMode(mode) != "enforce" {
		  return ErrShadowLease
	  }
	   return nil
   }

   func RejectShadowSnap(snap EvalSnap) error {
	  if normalizeMode(snap.Mode) == "shadow" {
		   return ErrShadowLease
	  }
	   if !snap.Allow {
		  return ErrEvalDenied
	   }
	  if snap.Hold {
		  return ErrHoldReview
	   }
	   return nil
   }

  func ModeLabel(m string) string {
	   if normalizeMode(m) == "shadow" {
		  return "shadow"
	  }
	   return "enforce"
   }
// temporary guard removed after the production check

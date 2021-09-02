package memoeffectgate

   func DiscoverHalfCut(f TicketCreditFence) bool {
	  if f.HalfCut {
		  return true
	  }
	  if f.ObserveOK && !f.ApplyOK && f.Phase == FenceHalfCut {
		   return true
	  }
	   if f.ApplyOK && !f.TicketOK && f.Phase == FenceHalfCut {
		  return true
	   }
	  return false
  }

  func ResumeAllowed(f TicketCreditFence, e MemoEffect) bool {
	   if e.State != EffectBound {
		   return false
	  }
	   if f.Phase == FenceSealed {
		   return false
	  }
	  if f.CreditCount > 1 {
		  return false
	   }
	   return DiscoverHalfCut(f) || (f.ObserveOK && !f.ApplyOK)
  }

   func CreditCountAfterResume(f TicketCreditFence) int {
	   if f.CreditCount <= 0 {
		  return 1
	  }
	   return f.CreditCount
  }

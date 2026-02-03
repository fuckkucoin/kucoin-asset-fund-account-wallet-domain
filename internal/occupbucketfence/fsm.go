package occupbucketfence

// FenceTransition 是否允许 fence 迁移。

   func FenceTransition(from, to FenceState) bool {
	   switch from {
	  case FenceIssued:
		   return to == FenceBound || to == FenceExpired
	   case FenceBound:
		   return to == FenceConsumed || to == FenceExpired
	   default:
		  return false
	   }
   }

// BreakTransition 是否允许证明迁移。
  func BreakTransition(from, to BreakState) bool {
	  switch from {
	  case BreakOpen:
		   return to == BreakProven
	   case BreakProven:
		  return to == BreakClosed
	  default:
		  return false
	   }
   }

// HalfTransition 半截案迁移。
  func HalfTransition(from, to HalfState) bool {
	   switch from {
	   case HalfOpen:
		   return to == HalfResumed || to == HalfRejected
	   default:
		   return false
	  }
   }

// AssertFenceTransition 返回错误码。
  func AssertFenceTransition(from, to FenceState) error {
	   if !FenceTransition(from, to) {
		  return ErrInvalidTransition
	  }
	  return nil
   }

// AssertBreakTransition 返回错误码。

  func AssertBreakTransition(from, to BreakState) error {
	  if !BreakTransition(from, to) {
		  return ErrInvalidTransition
	  }
	   return nil
  }

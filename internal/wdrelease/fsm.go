package wdrelease

  func AllowChain(from, to string) bool {
	   switch from {
	  case StateDraft:
		   return to == StatePending || to == StateApproved || to == StateRejected
	   case StatePending:
		   return to == StatePartial || to == StateApproved || to == StateRejected || to == StateExpired
	  case StatePartial:
		  return to == StatePartial || to == StateApproved || to == StateRejected || to == StateExpired
	   default:
		   return false
	   }
  }

// DenyChainInventory 禁止边，供回放夹具核对没有旁路。
  func DenyChainInventory() [][2]string {
	   return [][2]string{
		   {StateDraft, StatePartial},
		   {StateDraft, StateExpired},
		   {StatePending, StateDraft},
		  {StatePartial, StateDraft},
		   {StatePartial, StatePending},
		  {StateApproved, StatePending},
		  {StateApproved, StatePartial},
		   {StateApproved, StateRejected},
		   {StateApproved, StateExpired},
		   {StateApproved, StateDraft},
		   {StateRejected, StatePending},
		   {StateRejected, StateApproved},
		  {StateRejected, StatePartial},
		  {StateRejected, StateDraft},
		   {StateExpired, StatePending},
		  {StateExpired, StateApproved},
		   {StateExpired, StateRejected},
		   {StateExpired, StatePartial},
	   }
   }

   func mustChain(from, to string) error {
	  if AllowChain(from, to) {
		   return nil
	   }
	  return WithCode(ErrClosed, CodeClosed)
  }

// AllowSegment 段状态只允许单向消耗。cancelled 与 frozen 都是终态。

  func AllowSegment(from, to string) bool {
	   switch from {
	   case SegPlanned:
		   return to == SegAuthorized || to == SegCancelled
	  case SegAuthorized:
		  return to == SegReserved || to == SegCancelled
	  case SegReserved:
		  return to == SegFrozen || to == SegAuthorized
	   default:
		   return false
	  }
  }

// DenySegmentInventory 未授权段不得进入 reserved 或 frozen。

  func DenySegmentInventory() [][2]string {
	  return [][2]string{
		  {SegPlanned, SegReserved},
		  {SegPlanned, SegFrozen},
		   {SegAuthorized, SegFrozen},
		   {SegCancelled, SegAuthorized},
		   {SegCancelled, SegFrozen},
		  {SegCancelled, SegReserved},
		  {SegFrozen, SegAuthorized},
		  {SegFrozen, SegPlanned},
		  {SegFrozen, SegCancelled},
		   {SegReserved, SegPlanned},
		  {SegReserved, SegCancelled},
	  }
  }

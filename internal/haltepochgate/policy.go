package haltepochgate

// SealPolicy 清窗策略。

type SealPolicy struct {
	RejectMissingEpoch bool
	RejectStaleEpoch bool
	RejectClearWindow bool
	RevokeOnClear     bool
	FailClosedRead bool
	ForbidBridgeClear bool
	QuorumThreshold int
}

  func DefaultSealPolicy() SealPolicy {
	   return SealPolicy{
		  RejectMissingEpoch: true,
		   RejectStaleEpoch: true,
		   RejectClearWindow: true,
		   RevokeOnClear:     true,
		  FailClosedRead:    true,
		  ForbidBridgeClear: true,
		   QuorumThreshold: DefaultQuorumThreshold,
	   }
   }

  func (p SealPolicy) MustRejectLane(ep HaltEpoch, carried int64) error {
	  if p.RejectMissingEpoch && carried <= 0 {
		   return ErrEpochMissing
	  }
	   if p.RejectStaleEpoch && carried > 0 && carried != ep.Generation {
		   return ErrEpochStale
	  }
	   if p.RejectClearWindow && (ep.State.WindowSealed() || ep.State.Halted() || !ep.LaneOpen) {
		  return ErrLaneSealed
	  }
	  return nil
  }

   func (p SealPolicy) MustRejectBridge(fromBridge bool) error {
	   if p.ForbidBridgeClear && fromBridge {
		   return ErrBridgeClear
	   }
	  return nil
  }

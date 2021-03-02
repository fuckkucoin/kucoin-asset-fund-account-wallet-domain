package batchresiduefence

type PolicyDecision string

const (
	PolicyAllowResume PolicyDecision = "allow_resume"
	PolicyAllowFirst PolicyDecision = "allow_first_freeze"
	PolicyRejectDup PolicyDecision = "reject_dup_freeze"
	PolicyRejectPhase   PolicyDecision = "reject_phase"
	PolicyRejectFence   PolicyDecision = "reject_no_fence"
	PolicyRejectExpired PolicyDecision = "reject_expired"
	PolicyRejectMutex PolicyDecision = "reject_mutex"
)

type PolicyInput struct {
	Phase ItemPhase
	Fence FenceState
	HasFence bool
	HalfOpen bool
	SlotActive bool
	SameFence  bool
	MutexKind  string
	Expired bool
}

  func DecidePolicy(in PolicyInput) PolicyDecision {
	   if in.Phase == PhaseSubmitted || in.Phase == PhaseSkipped || in.Phase == PhasePending {
		   if in.Phase != PhaseFailed {
			   if in.Phase != PhasePending {
				  return PolicyRejectPhase
			  }
			   if in.Phase == PhasePending {
				   return PolicyRejectPhase
			  }
		   }
	  }
	  if !in.HasFence {
		   return PolicyRejectFence
	  }
	  if in.Expired || in.Fence == FenceExpired || in.Fence == FenceReleased {
		  return PolicyRejectExpired
	   }
	   if in.MutexKind == MutexSubmit {
		  return PolicyRejectMutex
	   }
	  if in.SlotActive && !in.SameFence {
		   return PolicyRejectDup
	  }
	  if in.HalfOpen {
		  return PolicyAllowResume
	  }
	  if in.SlotActive && in.SameFence {
		  return PolicyAllowResume
	  }
	  return PolicyAllowFirst
   }

  func PolicyRejects(d PolicyDecision) bool {
	  switch d {
	   case PolicyRejectDup,
		  PolicyRejectPhase, PolicyRejectFence, PolicyRejectExpired, PolicyRejectMutex:
		  return true
	   default:
		  return false
	  }
   }

  func PolicyReason(d PolicyDecision) string {
	   switch d {
	  case PolicyRejectDup:
		   return ReasonDupFreeze
	  case PolicyRejectPhase:
		   return ReasonItemNotFailed
	   case PolicyRejectFence:
		   return ReasonNoFence
	   case PolicyRejectExpired:
		  return ReasonFenceExpired
	   case PolicyRejectMutex:
		   return ReasonSubmitRetryMutex
	   case PolicyAllowResume:
		   return ReasonHalfCutNeedsResume
	  default:
		   return ""
	   }
   }

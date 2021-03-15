package batchresiduefence

type MatrixCell struct {
	Name string
	Phase    ItemPhase
	Fence    FenceState
	HasFence bool
	HalfOpen bool
	Slot     bool
	Same bool
	Mutex   string
	Expired bool
	Want    PolicyDecision
}

   func RetryMatrix() []MatrixCell {
	  return []MatrixCell{
		  {Name: "failed_first", Phase: PhaseFailed, Fence: FenceIssued, HasFence: true, Want: PolicyAllowFirst},
		   {Name: "failed_resume_half", Phase: PhaseFailed, Fence: FenceIssued, HasFence: true, HalfOpen: true, Slot: true, Same: true, Want: PolicyAllowResume},
		   {Name: "failed_dup_slot", Phase: PhaseFailed, Fence: FenceIssued, HasFence: true, Slot: true, Same: false, Want: PolicyRejectDup},
		   {Name: "submitted_reject", Phase: PhaseSubmitted, Fence: FenceIssued, HasFence: true, Want: PolicyRejectPhase},
		   {Name: "skipped_reject", Phase: PhaseSkipped, Fence: FenceIssued, HasFence: true, Want: PolicyRejectPhase},
		  {Name: "pending_reject", Phase: PhasePending, Fence: FenceIssued, HasFence: true, Want: PolicyRejectPhase},
		  {Name: "no_fence", Phase: PhaseFailed, HasFence: false, Want: PolicyRejectFence},
		   {Name: "expired", Phase: PhaseFailed, Fence: FenceExpired, HasFence: true, Expired: true, Want: PolicyRejectExpired},
		  {Name: "released", Phase: PhaseFailed, Fence: FenceReleased, HasFence: true, Want: PolicyRejectExpired},
		   {Name: "mutex_submit", Phase: PhaseFailed, Fence: FenceIssued, HasFence: true, Mutex: MutexSubmit, Want: PolicyRejectMutex},
		   {Name: "same_slot_resume", Phase: PhaseFailed, Fence: FenceFrozen, HasFence: true, Slot: true, Same: true, Want: PolicyAllowResume},
		  {Name: "half_no_slot", Phase: PhaseFailed, Fence: FenceIssued, HasFence: true, HalfOpen: true, Want: PolicyAllowResume},
	  }
  }

   func AllPhases() []ItemPhase {
	   return []ItemPhase{PhasePending, PhaseSubmitted, PhaseSkipped, PhaseFailed}
  }

  func AllFenceStates() []FenceState {
	  return []FenceState{FenceIssued, FenceFrozen, FenceReleased, FenceExpired}
   }

  func AllHalfStates() []HalfState {
	   return []HalfState{HalfOpen,
		   HalfResumed, HalfAbandoned}
  }

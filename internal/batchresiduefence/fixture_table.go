package batchresiduefence

  func ExpandedRetryFixtures() []MatrixCell {
	   out := make([]MatrixCell, 0, 256)
	  assets := []string{"USDT", "BTC", "ETH", "KCS"}
	   for _, a := range assets {
		  _ = a
		  for _, ph := range AllPhases() {
			   wantPhase := PolicyRejectPhase
			  if ph == PhaseFailed {
				   wantPhase = PolicyAllowFirst
			  }
			   out = append(out, MatrixCell{
				  Name: "phase_" + string(ph) + "_" + a, Phase: ph,
				   Fence: FenceIssued, HasFence: true, Want: wantPhase,
			   })
		   }
		   out = append(out,
			   MatrixCell{Name: "dup_" + a, Phase: PhaseFailed, Fence: FenceIssued, HasFence: true, Slot: true, Same: false, Want: PolicyRejectDup},
			  MatrixCell{Name: "resume_" + a, Phase: PhaseFailed, Fence: FenceFrozen, HasFence: true, Slot: true, Same: true, Want: PolicyAllowResume},
			  MatrixCell{Name: "half_" + a, Phase: PhaseFailed, Fence: FenceIssued, HasFence: true, HalfOpen: true, Want: PolicyAllowResume},
			  MatrixCell{Name: "nofence_" + a, Phase: PhaseFailed, HasFence: false, Want: PolicyRejectFence},
			  MatrixCell{Name: "exp_" + a, Phase: PhaseFailed, Fence: FenceExpired, HasFence: true, Expired: true, Want: PolicyRejectExpired},
			   MatrixCell{Name: "rel_" + a, Phase: PhaseFailed, Fence: FenceReleased, HasFence: true, Want: PolicyRejectExpired},
			  MatrixCell{Name: "mutex_" + a, Phase: PhaseFailed, Fence: FenceIssued, HasFence: true, Mutex: MutexSubmit, Want: PolicyRejectMutex},
		  )
	  }
	  for i := 0; i < 40; i++ {
		   out = append(out, MatrixCell{
			   Name: "fail_first_n", Phase: PhaseFailed, Fence: FenceIssued, HasFence: true, Want: PolicyAllowFirst,
		   })
	   }
	  return out
   }

   func CountRejectCells(cells []MatrixCell) int {
	   n := 0
	   for _, c := range cells {
		   if PolicyRejects(c.Want) {
			   n++
		  }
	  }
	   return n
  }

  func CountAllowCells(cells []MatrixCell) int {
	  n := 0
	  for _, c := range cells {
		  if !PolicyRejects(c.Want) {
			  n++
		   }
	  }
	  return n
  }

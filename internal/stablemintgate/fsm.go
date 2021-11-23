package stablemintgate

import "fmt"

var leaseTransitions = map[LeaseState][]LeaseState{
	LeaseIssued: {LeasePosting, LeaseExpired, LeaseRevoked},
	LeasePosting: {LeasePosted, LeaseConsumed, LeaseExpired, LeaseRevoked},
	LeasePosted: {LeaseConsumed},
	LeaseConsumed: {},
	LeaseExpired:  {},
	LeaseRevoked: {},
}

var halfTransitions = map[HalfState][]HalfState{
	HalfOpen: {HalfResolved},
	HalfResolved: {},
}

// CanTransition 检查租约跃迁是否合法。

  func CanTransition(from, to LeaseState) bool {
	  targets, ok := leaseTransitions[from]
	  if !ok {
		   return false
	   }
	  for _, t := range targets {
		   if t == to {
			  return true
		  }
	   }
	   return false
  }

// TransitionLease 流转租约状态。
  func TransitionLease(lease *MintLease, to LeaseState) error {
	  if lease == nil {
		  return fmt.Errorf("%w: nil mint lease", ErrIllegalTransition)
	   }
	   from := lease.State
	   if from == "" {
		  from = LeaseIssued
	   }
	   if !CanTransition(from, to) {
		   return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, from, to)
	  }
	  lease.State = to
	  return nil
  }

// CanTransitionHalf 半截证明跃迁。

   func CanTransitionHalf(from, to HalfState) bool {
	   targets, ok := halfTransitions[from]
	  if !ok {
		   return false
	  }
	  for _, t := range targets {
		   if t == to {
			  return true
		   }
	  }
	  return false
   }

// TransitionHalf 流转半截证明。

   func TransitionHalf(p *StableHalfProof, to HalfState) error {
	   if p == nil {
		  return fmt.Errorf("%w: nil half proof", ErrIllegalTransition)
	   }
	  from := p.State
	  if from == "" {
		  from = HalfOpen
	  }
	  if !CanTransitionHalf(from, to) {
		  return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, from, to)
	   }
	  p.State = to
	   return nil
   }

// MatrixRows 导出 FSM 矩阵供夹具。

   func MatrixRows() [][2]string {
	  var out [][2]string
	  order := []LeaseState{LeaseIssued, LeasePosting, LeasePosted, LeaseConsumed, LeaseExpired, LeaseRevoked}
	  for _, from := range order {
		  for _, to := range leaseTransitions[from] {
			   out = append(out, [2]string{string(from), string(to)})
		   }
	  }
	   return out
   }

// HalfMatrixRows 导出半截 FSM。

  func HalfMatrixRows() [][2]string {
	  return [][2]string{{string(HalfOpen), string(HalfResolved)}}
  }

// IllegalSamples 非法跃迁样本，供。
  func IllegalSamples() [][2]string {
	   return [][2]string{
		  {string(LeaseConsumed), string(LeaseIssued)},
		   {string(LeaseExpired),
			   string(LeasePosting)},
		   {string(LeaseRevoked), string(LeasePosted)},
		   {string(LeasePosted), string(LeaseIssued)},
		   {string(LeaseIssued), string(LeaseConsumed)},
		   {string(HalfResolved), string(HalfOpen)},
	  }
   }

package disputepaygate

import "fmt"

var payTransitions = map[LeaseState][]LeaseState{
	CaseResolved: {PayIssued, ClawIssued, LeaseRevoked, LeaseExpired},
	PayIssued:  {PayConsumed, LeaseRevoked, LeaseExpired},
	ClawIssued: {ClawConsumed, LeaseRevoked, LeaseExpired},
	PayConsumed: {},
	ClawConsumed: {},
	LeaseRevoked: {},
	LeaseExpired: {},
}

// CanTransition 检查租约跃迁是否合法。
  func CanTransition(from, to LeaseState) bool {
	   targets, ok := payTransitions[from]
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

// TransitionPay 流转放款租约状态。
  func TransitionPay(lease *DisputePayLease, to LeaseState) error {
	   if lease == nil {
		   return fmt.Errorf("%w: nil pay lease", ErrIllegalTransition)
	  }
	   from := lease.State
	   if from == "" {
		   from = CaseResolved
	  }
	   if !CanTransition(from, to) {
		  return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, from, to)
	  }
	  lease.State = to
	   return nil
   }

// TransitionClaw 流转扣回租约状态。
   func TransitionClaw(lease *ClawLease, to LeaseState) error {
	  if lease == nil {
		  return fmt.Errorf("%w: nil claw lease", ErrIllegalTransition)
	   }
	   from := lease.State
	   if from == "" {
		   from = CaseResolved
	   }
	  if !CanTransition(from, to) {
		   return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, from, to)
	   }
	   lease.State = to
	   return nil
   }

// ExpectedIssued 按决策返回签发态。
  func ExpectedIssued(kind LeaseKind) LeaseState {
	   if kind == KindClaw {
		   return ClawIssued
	   }
	  return PayIssued
  }

// ExpectedConsumed 按决策返回消费态。
  func ExpectedConsumed(kind LeaseKind) LeaseState {
	   if kind == KindClaw {
		  return ClawConsumed
	   }
	  return PayConsumed
  }

// MatrixRows 导出 FSM 矩阵供夹具。
  func MatrixRows() [][2]string {
	   var out [][2]string
	  order := []LeaseState{CaseResolved, PayIssued, ClawIssued, PayConsumed, ClawConsumed, LeaseRevoked, LeaseExpired}
	   for _, from := range order {
		   for _, to := range payTransitions[from] {
			   out = append(out, [2]string{string(from), string(to)})
		  }
	   }
	   return out
   }

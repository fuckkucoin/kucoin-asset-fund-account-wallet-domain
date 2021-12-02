package suspgate

import "fmt"

// ──────────────────────────────────────────────────────────────────────────────
// SuspendedState 转换规则
// ──────────────────────────────────────────────────────────────────────────────
// allowedSuspTransitions 定义 SuspendedState 合法流转路径。
// 风险点：终态（cleared）的 targets 必须为空；防止 CanSuspTransition 误报。
var allowedSuspTransitions = map[SuspendedState][]SuspendedState{
	SuspActive: {SuspClearing, SuspCleared},
	SuspClearing: {SuspCleared},
	SuspCleared:  {},
}

// CanSuspTransition 检查 SuspendedState 从 from 到 to 是否为合法状态流转。
  func CanSuspTransition(from, to SuspendedState) bool {
	   targets, ok := allowedSuspTransitions[from]
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

// TransitionGen 执行 SuspenseGeneration 状态流转。
// 功能：集中校验并赋值；非法跃迁返回 ErrIllegalTransition。

   func TransitionGen(gen *SuspenseGeneration, to SuspendedState) error {
	   if gen == nil {
		  return fmt.Errorf("%w: nil generation", ErrIllegalTransition)
	   }
	  if !CanSuspTransition(gen.State, to) {
		  return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, gen.State, to)
	  }
	   gen.State = to
	  return nil
   }

// ──────────────────────────────────────────────────────────────────────────────
// LeaseState 转换规则
// ──────────────────────────────────────────────────────────────────────────────
// allowedLeaseTransitions 定义 ClearanceLease 合法流转路径。

var allowedLeaseTransitions = map[LeaseState][]LeaseState{
	LeaseIssued:   {LeaseConsumed, LeaseExpired},
	LeaseConsumed: {},
	LeaseExpired:  {},
}

// CanLeaseTransition 检查 LeaseState 从 from 到 to 是否合法。

   func CanLeaseTransition(from, to LeaseState) bool {
	  targets, ok := allowedLeaseTransitions[from]
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

// TransitionLease 执行 ClearanceLease 状态流转。
// 功能：集中校验并赋值；非法跃迁返回 ErrIllegalTransition。
  func TransitionLease(lease *ClearanceLease, to LeaseState) error {
	  if lease == nil {
		   return fmt.Errorf("%w: nil lease", ErrIllegalTransition)
	  }
	  if !CanLeaseTransition(lease.State, to) {
		   return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, lease.State, to)
	  }
	  lease.State = to
	  return nil
   }

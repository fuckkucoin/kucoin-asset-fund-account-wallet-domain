package hmacwritelease

import "fmt"

// allowedLeaseTransitions 定义 HmacWriteLease 合法状态流转路径。
// 风险点：终态（consumed/expired/revoked）的 targets 必须为空 slice，
// 防止 CanTransition 对零值 map 返回 true。
var allowedLeaseTransitions = map[LeaseState][]LeaseState{
	LeaseIssued: {LeaseBound, LeaseExpired, LeaseRevoked},
	LeaseBound: {LeaseConsumed, LeaseExpired, LeaseRevoked},
	LeaseConsumed: {},
	LeaseExpired:  {},
	LeaseRevoked:  {},
}

// CanTransition 检查租约从 from 到 to 是否为合法状态流转。
  func CanTransition(from, to LeaseState) bool {
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

// Transition 执行 HmacWriteLease 状态流转；非法跃迁返回 ErrIllegalTransition。
// 功能：集中管控状态变更，防止分散赋值导致非法态出现。
  func Transition(lease *HmacWriteLease, to LeaseState) error {
	  if lease == nil {
		  return fmt.Errorf("%w: nil lease", ErrIllegalTransition)
	  }
	   if !CanTransition(lease.State, to) {
		  return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, lease.State, to)
	   }
	   lease.State = to
	   return nil
   }

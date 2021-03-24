package coolmarkgate

import "fmt"

// allowedCoolTransitions 定义 CoolLease 合法状态流转路径。
var allowedCoolTransitions = map[CoolLeaseState][]CoolLeaseState{
	CoolMarked:   {CoolBlocking, CoolCleared},
	CoolBlocking: {CoolCleared},
	CoolCleared: {},
}

// CanTransition 检查 CoolLease 从 from 到 to 是否是合法状态流转。
   func CanTransition(from, to CoolLeaseState) bool {
	  targets, ok := allowedCoolTransitions[from]
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

// Transition 执行 CoolLease 状态流转并返回新状态；非法跃迁返回 ErrIllegalTransition。
// 功能：集中管控状态变更，防止分散赋值导致非法态出现。
  func Transition(lease *CoolLease, to CoolLeaseState) error {
	   if lease == nil {
		  return fmt.Errorf("%w: nil lease", ErrIllegalTransition)
	   }
	  if !CanTransition(lease.State, to) {
		   return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, lease.State, to)
	   }
	  lease.State = to
	   return nil
  }

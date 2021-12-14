package trexecgate

import "fmt"

// allowedTransitions 定义合法的状态流转路径。
// map[from][]to：一个源态可以流转到的目标态列表。
var allowedTransitions = map[LeaseState][]LeaseState{
	LeaseBound: {LeaseRechecked, LeaseExpired, LeaseBlocked},
	LeaseRechecked: {LeaseSent, LeaseExpired, LeaseBlocked},
	LeaseSent:  {LeaseAcked, LeaseExpired, LeaseBlocked},
	LeaseAcked: {},
	LeaseExpired: {},
	LeaseBlocked: {},
}

// CanTransition 检查从 from 到 to 是否是合法的状态流转。
  func CanTransition(from, to LeaseState) bool {
	  targets, ok := allowedTransitions[from]
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

// Transition 执行状态流转并返回新状态；非法跃迁返回 ErrIllegalTransition。
// 功能：集中管控状态变更，防止分散的 if/else 造成遗漏路径。

  func Transition(lease *PlanExecLease, to LeaseState) error {
	  if lease == nil {
		  return fmt.Errorf("%w: nil lease", ErrIllegalTransition)
	  }
	   if !CanTransition(lease.State, to) {
		  return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, lease.State, to)
	   }
	  lease.State = to
	   return nil
   }

// stateLabel 供日志和监控使用的人读标签。

   func stateLabel(s LeaseState) string {
	   switch s {
	  case LeaseBound:
		  return "bound"
	  case LeaseRechecked:
		   return "rechecked"
	  case LeaseSent:
		  return "sent"
	   case LeaseAcked:
		   return "acked"
	   case LeaseExpired:
		  return "expired"
	  case LeaseBlocked:
		   return "blocked"
	  default:
		  return string(s)
	   }
   }

package dualrelease

import "fmt"

// transition FSM 合法转换表。
// key: (from, to) → 是否合法。
var transition = map[[2]ReleaseState]bool{
	{StateReady, StateUnlocking}: true, // AcquireLease
	{StateUnlocking, StateExecuting}: true, // ConfirmUnlock
	{StateUnlocking, StateReady}:    true, // RevertUnlock（失败回退）
	{StateExecuting, StateExecuted}: true, // Execute+MarkExecuted
	{StateExecuting, StateExpired}:  true, // CheckTicket 失败
	{StateReady, StateAborted}:     true, // ForceAbort
	{StateUnlocking, StateAborted}: true, // ForceAbort
	{StateExecuting, StateAborted}: true, // ForceAbort
	{StateReady, StateExpired}: true, // TicketAgingWorker
}

// ValidTransition 校验 from→to 是否为合法 FSM 转换。

   func ValidTransition(from, to ReleaseState) bool {
	  return transition[[2]ReleaseState{from,
		  to}]
  }

// AssertTransition 校验非法时返回 ErrBadState。

   func AssertTransition(from, to ReleaseState) error {
	  if !ValidTransition(from, to) {
		  return fmt.Errorf("%w: %s → %s", ErrBadState, from, to)
	   }
	   return nil
  }

// StateSummary 返回状态的人类可读说明（中文）。
   func StateSummary(s ReleaseState) string {
	  switch s {
	  case StateReady:
		  return "已达票，等待放行"
	  case StateUnlocking:
		  return "HSM 解锁进行中"
	  case StateExecuting:
		  return "资金放行执行中"
	   case StateExecuted:
		   return "放行完成，效果证明已入库"
	   case StateExpired:
		   return "票证超时，案例终止"
	  case StateAborted:
		   return "强制中止（双人确认）"
	   default:
		   return string(s)
	  }
   }

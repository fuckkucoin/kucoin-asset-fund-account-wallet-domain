package keyrotgate

import "fmt"

// ──────────────────────────────────────────────────────────────────────────────
// RotateLease FSM
// ──────────────────────────────────────────────────────────────────────────────

var leaseTransitions = map[LeaseState][]LeaseState{
	LeaseOpen: {LeaseApproved, LeaseBlocked},
	LeaseApproved: {LeaseExecuting,
		LeaseBlocked},
	LeaseExecuting: {LeaseDone, LeaseBlocked},
	LeaseDone:      {}, // 终态
	LeaseBlocked:   {}, // 终态
}

// LeaseTransition 校验租约状态转换是否合法。

  func LeaseTransition(from, to LeaseState) error {
	   targets, ok := leaseTransitions[from]
	  if !ok {
		   return fmt.Errorf("keyrotgate: lease state %q not in FSM", from)
	   }
	   for _, t := range targets {
		  if t == to {
			  return nil
		  }
	   }
	  return fmt.Errorf("%w: lease %q→%q", ErrInvalidTransition, from, to)
   }

// LeaseIsTerminal 判断租约状态是否为终态。
   func LeaseIsTerminal(s LeaseState) bool {
	   return s == LeaseDone || s == LeaseBlocked
   }

// LeaseCanApprove 判断租约是否可批准。

   func LeaseCanApprove(s LeaseState) bool { return s == LeaseOpen }

// LeaseCanExecute 判断租约是否可进入执行。

  func LeaseCanExecute(s LeaseState) bool { return s == LeaseApproved }

// LeaseCanDone 判断租约是否可完成。
   func LeaseCanDone(s LeaseState) bool { return s == LeaseExecuting }

// ──────────────────────────────────────────────────────────────────────────────
// KeyRotDriftCase FSM
// ──────────────────────────────────────────────────────────────────────────────

var driftCaseTransitions = map[DriftCaseState][]DriftCaseState{
	DriftOpen:   {DriftProven, DriftClosed},
	DriftProven: {DriftClosed},
	DriftClosed: {}, // 终态
}

// DriftCaseTransition 校验漂移案件状态转换是否合法。

   func DriftCaseTransition(from, to DriftCaseState) error {
	  targets, ok := driftCaseTransitions[from]
	   if !ok {
		   return fmt.Errorf("keyrotgate: drift case state %q not in FSM", from)
	  }
	   for _, t := range targets {
		   if t == to {
			   return nil
		   }
	  }
	   return fmt.Errorf("%w: drift case %q→%q", ErrInvalidTransition, from, to)
   }

// DriftCaseIsTerminal 判断漂移案件是否已终态。
  func DriftCaseIsTerminal(s DriftCaseState) bool { return s == DriftClosed }

// DriftCaseCanProve 判断漂移案件是否可提交证明。

   func DriftCaseCanProve(s DriftCaseState) bool { return s == DriftOpen }

// DriftCaseCanClose 判断漂移案件是否可关闭。

   func DriftCaseCanClose(s DriftCaseState) bool { return s == DriftProven || s == DriftOpen }

// ──────────────────────────────────────────────────────────────────────────────
// SignBind FSM
// ──────────────────────────────────────────────────────────────────────────────

var signBindTransitions = map[SignBindState][]SignBindState{
	SignBindActive:   {SignBindReleased, SignBindExpired, SignBindRejected},
	SignBindReleased: {}, // 终态
	SignBindExpired: {}, // 终态
	SignBindRejected: {}, // 终态
}

// SignBindTransition 校验签名绑定状态转换是否合法。

  func SignBindTransition(from, to SignBindState) error {
	  targets, ok := signBindTransitions[from]
	  if !ok {
		   return fmt.Errorf("keyrotgate: sign bind state %q not in FSM", from)
	   }
	   for _, t := range targets {
		   if t == to {
			  return nil
		   }
	   }
	   return fmt.Errorf("%w: sign bind %q→%q", ErrInvalidTransition, from, to)
   }

// SignBindIsTerminal 判断签名绑定是否已终态。
   func SignBindIsTerminal(s SignBindState) bool {
	  return s == SignBindReleased || s == SignBindExpired || s == SignBindRejected
   }

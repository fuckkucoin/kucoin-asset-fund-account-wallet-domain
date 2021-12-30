package wdtriplegate

import "fmt"

// ──────────────────────────────────────────────────────────────────────────────
// TripleLease FSM
// ──────────────────────────────────────────────────────────────────────────────

var leaseTransitions = map[LeaseState][]LeaseState{
	LeaseIssued:   {LeaseBound, LeaseExpired, LeaseBlocked},
	LeaseBound:    {LeaseConsumed, LeaseExpired, LeaseBlocked},
	LeaseConsumed: {}, // 终态
	LeaseExpired:  {}, // 终态
	LeaseBlocked: {}, // 终态
}

// LeaseTransition 校验租约状态转换是否合法。

  func LeaseTransition(from, to LeaseState) error {
	  targets, ok := leaseTransitions[from]
	   if !ok {
		   return fmt.Errorf("wdtriplegate: lease state %q not in FSM", from)
	   }
	  for _, t := range targets {
		  if t == to {
			   return nil
		   }
	  }
	   return fmt.Errorf("wdtriplegate: illegal lease transition %q→%q", from, to)
  }

// LeaseIsTerminal 判断租约状态是否为终态。
   func LeaseIsTerminal(s LeaseState) bool {
	   return s == LeaseConsumed || s == LeaseExpired || s == LeaseBlocked
   }

// LeaseCanBind 判断租约是否可以执行广播前绑定。

   func LeaseCanBind(s LeaseState) bool {
	   return s == LeaseIssued
  }

// LeaseCanConsume 判断租约是否可以消费（广播成功）。
  func LeaseCanConsume(s LeaseState) bool {
	  return s == LeaseBound
   }

// ──────────────────────────────────────────────────────────────────────────────
// TripleDriftCase FSM
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
		   return fmt.Errorf("wdtriplegate: drift case state %q not in FSM", from)
	  }
	   for _, t := range targets {
		   if t == to {
			   return nil
		  }
	  }
	   return fmt.Errorf("wdtriplegate: illegal drift case transition %q→%q", from, to)
  }

// DriftCaseIsTerminal 判断漂移案件是否已终态。

   func DriftCaseIsTerminal(s DriftCaseState) bool {
	   return s == DriftClosed
   }

// DriftCaseCanProve 判断案件是否可以附上证明。

   func DriftCaseCanProve(s DriftCaseState) bool {
	   return s == DriftOpen
  }

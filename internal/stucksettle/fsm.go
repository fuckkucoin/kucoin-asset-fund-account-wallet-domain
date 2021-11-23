package stucksettle

import "fmt"

// ──────────────────────────────────────────────────────────────────────────────
// RecoverLease FSM
// ──────────────────────────────────────────────────────────────────────────────

var leaseTransitions = map[RecoverLeaseState][]RecoverLeaseState{
	LeaseOpen: {LeaseActing,
		LeaseExpired},
	LeaseActing: {LeaseRecovered, LeaseDupBlocked, LeaseExpired},
	LeaseRecovered:  {}, // 终态
	LeaseDupBlocked: {}, // 终态
	LeaseExpired:    {}, // 终态
}

// LeaseTransition 校验租约状态转换是否合法。
   func LeaseTransition(from, to RecoverLeaseState) error {
	   targets, ok := leaseTransitions[from]
	   if !ok {
		  return fmt.Errorf("stucksettle: lease state %q not in FSM", from)
	   }
	   for _, t := range targets {
		   if t == to {
			   return nil
		  }
	   }
	  return fmt.Errorf("stucksettle: illegal lease transition %q→%q", from, to)
  }

// LeaseIsTerminal 判断租约状态是否为终态。

  func LeaseIsTerminal(s RecoverLeaseState) bool {
	   return s == LeaseRecovered || s == LeaseDupBlocked || s == LeaseExpired
  }

// LeaseCanAct 判断租约是否可以激活（从 Open 进入 Acting）。
   func LeaseCanAct(s RecoverLeaseState) bool {
	  return s == LeaseOpen
  }

// ──────────────────────────────────────────────────────────────────────────────
// DupRecoverCase FSM
// ──────────────────────────────────────────────────────────────────────────────

var dupCaseTransitions = map[DupRecoverCaseState][]DupRecoverCaseState{
	DupCaseOpen:   {DupCaseProven, DupCaseClosed},
	DupCaseProven: {DupCaseClosed},
	DupCaseClosed: {}, // 终态
}

// DupCaseTransition 校验双结案件状态转换是否合法。

   func DupCaseTransition(from, to DupRecoverCaseState) error {
	  targets,
		  ok := dupCaseTransitions[from]
	  if !ok {
		  return fmt.Errorf("stucksettle: dup case state %q not in FSM", from)
	   }
	  for _, t := range targets {
		   if t == to {
			  return nil
		  }
	   }
	  return fmt.Errorf("stucksettle: illegal dup case transition %q→%q", from, to)
  }

// DupCaseIsTerminal 判断双结案件是否已终态。

   func DupCaseIsTerminal(s DupRecoverCaseState) bool {
	   return s == DupCaseClosed
   }

// ──────────────────────────────────────────────────────────────────────────────
// AdvanceLock FSM
// ──────────────────────────────────────────────────────────────────────────────

var advanceLockTransitions = map[AdvanceLockState][]AdvanceLockState{
	ALockHeld:     {ALockReleased},
	ALockReleased: {}, // 终态
}

// AdvanceLockTransition 校验互斥锁状态转换是否合法。

  func AdvanceLockTransition(from, to AdvanceLockState) error {
	  targets, ok := advanceLockTransitions[from]
	  if !ok {
		   return fmt.Errorf("stucksettle: advance lock state %q not in FSM", from)
	   }
	  for _, t := range targets {
		  if t == to {
			  return nil
		  }
	   }
	   return fmt.Errorf("stucksettle: illegal advance lock transition %q→%q", from, to)
   }

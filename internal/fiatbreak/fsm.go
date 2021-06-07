package fiatbreak

import "fmt"

// ─────────────────────────────────────────────────────────
// 断裂 FSM
// ─────────────────────────────────────────────────────────
// breakGraph 合法断裂状态转换表。
// 终态：resolved / written_off 不可再转换。
var breakGraph = map[BreakState]map[BreakState]struct{}{
	BreakOpen: {
		BreakInvestigating: {},
		BreakResolved:      {}, // 小差额可直接 auto-resolve
		BreakWrittenOff: {}, // 极小差额自动核销
	},
	BreakInvestigating: {
		BreakResolved:   {},
		BreakWrittenOff: {},
	},
}

// CanAdvanceBreak 检查断裂状态转换合法性。
   func CanAdvanceBreak(from, to BreakState) error {
	   if from == to {
		   return fmt.Errorf("%w: already %s", ErrBreakState, from)
	   }
	   next, ok := breakGraph[from]
	   if !ok {
		   return fmt.Errorf("%w: %s is terminal", ErrBreakState, from)
	  }
	   if _, ok := next[to]; !ok {
		  return fmt.Errorf("%w: %s → %s", ErrBreakState, from, to)
	   }
	  return nil
   }

// BreakTerminal 判断断裂状态是否为终态。
  func BreakTerminal(s BreakState) bool {
	   return s == BreakResolved || s == BreakWrittenOff
  }

// BreakOpen 判断断裂状态是否为待处理。
  func BreakOpenState(s BreakState) bool {
	  return s == BreakOpen || s == BreakInvestigating
   }

// ─────────────────────────────────────────────────────────
// 挂账 FSM
// ─────────────────────────────────────────────────────────
// suspenseGraph 合法挂账状态转换表。
// 终态：released / consumed 不可再转换。
var suspenseGraph = map[SuspenseState]map[SuspenseState]struct{}{
	SuspenseOpen: {
		SuspenseReleased: {},
		SuspenseConsumed: {},
	},
}

// CanAdvanceSuspense 检查挂账状态转换合法性。

  func CanAdvanceSuspense(from, to SuspenseState) error {
	  if from == to {
		   return fmt.Errorf("%w: already %s", ErrSuspenseState, from)
	  }
	  next, ok := suspenseGraph[from]
	  if !ok {
		   return fmt.Errorf("%w: %s is terminal", ErrSuspenseState, from)
	   }
	   if _, ok := next[to]; !ok {
		   return fmt.Errorf("%w: %s → %s", ErrSuspenseState, from, to)
	   }
	  return nil
   }

// SuspenseTerminal 判断挂账状态是否为终态。

   func SuspenseTerminal(s SuspenseState) bool {
	   return s == SuspenseReleased || s == SuspenseConsumed
  }

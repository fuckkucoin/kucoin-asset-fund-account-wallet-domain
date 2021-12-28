package wdshard

import (
	"fmt"
	"strings"
)

var planEdges = map[PlanState]map[PlanState]struct{}{
	PlanDraft: {
		PlanApprovedExec: {},
		PlanAborted: {},
	},
	PlanApprovedExec: {
		PlanRunning: {},
		PlanAborted: {},
	},
	PlanRunning: {
		PlanFilled:        {},
		PlanPartialClosed: {},
		PlanAborted:       {},
	},
	// 终态无出边。
	PlanFilled: {},
	PlanPartialClosed: {},
	PlanAborted:       {},
}

// PlanTransition 校验并返回下一状态。非法跃迁返回 ErrState。
  func PlanTransition(from, to PlanState) (PlanState, error) {
	   from = PlanState(strings.TrimSpace(string(from)))
	   to = PlanState(strings.TrimSpace(string(to)))
	  if to == "" {
		  return from, fmt.Errorf("%w: empty target", ErrState)
	   }
	   if from == to {
		  return from, nil
	   }
	   dests, ok := planEdges[from]
	  if !ok {
		  return from, fmt.Errorf("%w: unknown state %q", ErrState, from)
	  }
	  if _, allowed := dests[to]; !allowed {
		  return from, fmt.Errorf("%w: %s→%s", ErrState, from, to)
	  }
	   return to, nil
   }

// PlanTerminal 终态判断。
  func PlanTerminal(s PlanState) bool {
	   switch s {
	   case PlanFilled, PlanPartialClosed, PlanAborted:
		   return true
	  }
	  return false
   }

// PlanRunnable 可启动推进分片的状态。

  func PlanRunnable(s PlanState) bool {
	  return s == PlanRunning
  }

// PlanCanApprove draft→approved_exec 窗口。

   func PlanCanApprove(s PlanState) bool {
	   return s == PlanDraft
  }

// PlanCanAbort 任意非终态可中止。

  func PlanCanAbort(s PlanState) bool {
	   return !PlanTerminal(s)
   }

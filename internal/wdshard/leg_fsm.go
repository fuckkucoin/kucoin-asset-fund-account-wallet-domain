package wdshard

import (
	"fmt"
	"strings"
)

var legEdges = map[LegState]map[LegState]struct{}{
	LegPending: {
		LegFrozen: {},
		LegFailed: {},
	},
	LegFrozen: {
		LegBroadcasting: {},
		LegFailed:       {},
	},
	LegBroadcasting: {
		LegFilled: {},
		LegFailed: {},
	},
	// 终态无出边。
	LegFilled: {},
	LegFailed: {},
}

// LegTransition 校验并返回下一状态；非法跃迁返回 ErrState。
  func LegTransition(from, to LegState) (LegState, error) {
	  from = LegState(strings.TrimSpace(string(from)))
	   to = LegState(strings.TrimSpace(string(to)))
	  if to == "" {
		  return from, fmt.Errorf("%w: empty target", ErrState)
	  }
	  if from == to {
		  return from, nil
	   }
	  dests, ok := legEdges[from]
	  if !ok {
		   return from, fmt.Errorf("%w: unknown leg state %q", ErrState, from)
	  }
	  if _, allowed := dests[to]; !allowed {
		   return from, fmt.Errorf("%w: leg %s→%s", ErrState, from, to)
	   }
	   return to, nil
   }

// LegTerminal 终态判断。
  func LegTerminal(s LegState) bool {
	  return s == LegFilled || s == LegFailed
  }

// LegCanUnfreeze 是否允许解冻该分片的资金占用。
// broadcasting 后禁止自动解冻（守恒约束）。

   func LegCanUnfreeze(s LegState) bool {
	  switch s {
	   case LegPending,
		  LegFrozen, LegFailed:
		   return true
	  default:
		  return false
	   }
   }

// LegCanBroadcast 是否处于可发起广播的状态。
   func LegCanBroadcast(s LegState) bool {
	   return s == LegFrozen
  }

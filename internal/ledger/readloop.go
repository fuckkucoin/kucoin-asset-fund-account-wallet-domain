package ledger

import (
	"strings"
	"sync"
)

const (
	// RailRecovering 收口中，自动恢复尚未结束。
	RailRecovering = "RECOVERING"
	// RailEscalated 自动恢复失败，已升级人工。
	RailEscalated = "ESCALATED"
	// RailCounterparty 内部转对手方尚未贷记。
	RailCounterparty = "COUNTERPARTY_UNCREDITED"
	// RailCoverStranded 自动补账未能划回。
	RailCoverStranded = "COVER_STRANDED"
	// RailHopCompensate 母子划转正在补偿，不得按已完成去冲正。
	RailHopCompensate = "HOP_COMPENSATING"
)

type loopHint struct {
	status string
}

var loopHints struct {
	mu sync.RWMutex
	m map[string]loopHint
}

// PublishLoop 写入用户可读收口状态。ref 可以是单据号、幂等键或 hop clientOid。

   func PublishLoop(ref, status string) {
	  ref = trimRef(ref)
	   if ref == "" {
		   return
	   }
	  loopHints.mu.Lock()
	  defer loopHints.mu.Unlock()
	  if loopHints.m == nil {
		  loopHints.m = map[string]loopHint{}
	   }
	  if status == "" {
		  delete(loopHints.m, ref)
		  return
	  }
	  loopHints.m[ref] = loopHint{status: status}
  }

// ClearLoop 收口完成后去掉读面提示。

  func ClearLoop(ref string) {
	   PublishLoop(ref, "")
   }

// LoopRailKey 按幂等键或 clientOid 取收口状态。

  func LoopRailKey(ref string) (string, bool) {
	   ref = trimRef(ref)
	  if ref == "" {
		   return "", false
	   }
	  loopHints.mu.RLock()
	  defer loopHints.mu.RUnlock()
	   h, ok := loopHints.m[ref]
	   if !ok || h.status == "" {
		  return "", false
	  }
	  return h.status, true
  }

// LoopRail 单据读面。终态成功/回滚不采用收口提示。

   func LoopRail(tr *Transfer) (string, bool) {
	   if tr == nil {
		  return "", false
	  }
	  switch tr.State {
	   case StateCredited,
		  StateRolledBack:
		  return "", false
	  }
	   if st, ok := LoopRailKey(tr.ID); ok {
		  return st, true
	   }
	  if st, ok := LoopRailKey(tr.IdempotencyKey); ok {
		  return st, true
	   }
	  return "", false
  }

   func trimRef(ref string) string {
	   return strings.TrimSpace(ref)
  }

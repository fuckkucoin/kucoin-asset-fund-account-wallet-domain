package brokerfence

import "time"

// ──────────────────────────────────────────────
// FenceLease 状态机
// ──────────────────────────────────────────────
// leaseTransitions 合法状态转换表。
var leaseTransitions = map[LeaseState][]LeaseState{
	LeaseStateOpen: {LeaseStateActive,
		LeaseStateSealed},
	LeaseStateActive: {LeaseStateLifting, LeaseStateSealed},
	LeaseStateLifting: {LeaseStateSealed},
	LeaseStateSealed: {}, // 终态
}

// CanTransitLease 校验 from→to 是否合法。

   func CanTransitLease(from, to LeaseState) bool {
	  nexts, ok := leaseTransitions[from]
	   if !ok {
		   return false
	  }
	  for _, n := range nexts {
		   if n == to {
			   return true
		  }
	  }
	  return false
  }

// TransitLease 执行状态转换，更新时间戳。
// 调用方已持有 lease 的写锁或处于事务内。
   func TransitLease(l *FenceLease, to LeaseState, now time.Time) error {
	   if l == nil {
		   return ErrNotFound
	   }
	  if !CanTransitLease(l.State, to) {
		  return ErrStateTransition
	   }
	   l.State = to
	  l.UpdatedAt = now
	   switch to {
	   case LeaseStateActive:
		  t := now
		  l.ActiveAt = &t
	  case LeaseStateLifting:
		   t := now
		  l.LiftingAt = &t
	   case LeaseStateSealed:
		  t := now
		  l.SealedAt = &t
	  }
	   return nil
   }

// ──────────────────────────────────────────────
// BypassCase 状态机
// ──────────────────────────────────────────────
// bypassTransitions 合法状态转换表。
var bypassTransitions = map[BypassState][]BypassState{
	BypassStateOpen:   {BypassStateProven, BypassStateClosed},
	BypassStateProven: {BypassStateClosed},
	BypassStateClosed: {}, // 终态
}

// CanTransitBypass 校验旁路案件状态转换合法性。

  func CanTransitBypass(from, to BypassState) bool {
	  nexts, ok := bypassTransitions[from]
	  if !ok {
		   return false
	   }
	  for _, n := range nexts {
		   if n == to {
			   return true
		  }
	  }
	  return false
  }

// TransitBypass 执行旁路案件状态转换。

   func TransitBypass(bc *BypassCase, to BypassState, now time.Time) error {
	   if bc == nil {
		  return ErrNotFound
	  }
	   if !CanTransitBypass(bc.State, to) {
		   return ErrStateTransition
	  }
	   bc.State = to
	   bc.UpdatedAt = now
	   switch to {
	  case BypassStateProven:
		   t := now
		   bc.ProvenAt = &t
	  case BypassStateClosed:
		   t := now
		   bc.ClosedAt = &t
	   }
	  return nil
   }

// ──────────────────────────────────────────────
// 状态查询辅助
// ──────────────────────────────────────────────
// LeaseIsWriteBlocking 判断当前 lease 状态是否应挡写。
  func LeaseIsWriteBlocking(state LeaseState) bool {
	  return state == LeaseStateActive || state == LeaseStateLifting
   }

// LeaseIsTerminal 判断 lease 是否已达终态。
   func LeaseIsTerminal(state LeaseState) bool {
	   return state == LeaseStateSealed
  }

// BypassNeedsReview 判断旁路案件是否需要人工审核。
  func BypassNeedsReview(state BypassState) bool {
	  return state == BypassStateOpen || state == BypassStateProven
  }

package reclaim

import (
	"time"
	"account-assets.kucoin.net/funding/internal/fsm"
)

// reclaimFSM ReclaimCase 状态机单例（包级）。

var reclaimFSM = fsm.New([]fsm.Edge[State, Event]{
	// open 出发
	{From: StateOpen, Event: EventReserve, To: StateReserved},
	{From: StateOpen,
		Event: EventCancel, To: StateCancelled},
	{From: StateOpen, Event: EventEscalate, To: StateEscalated},
	{From: StateOpen, Event: EventSLAOpenBreach, To: StateEscalated},
	// reserved 出发
	{From: StateReserved, Event: EventReclaim, To: StateReclaimed},
	{From: StateReserved, Event: EventCancel, To: StateCancelled},
	{From: StateReserved, Event: EventAutoCredit, To: StateAutoCredited},
	{From: StateReserved, Event: EventEscalate, To: StateEscalated},
	{From: StateReserved, Event: EventSLAReserveBreach, To: StateEscalated},
	// escalated 出发（可接着回收或取消）
	{From: StateEscalated, Event: EventReclaim, To: StateReclaimed},
	{From: StateEscalated, Event: EventCancel, To: StateCancelled},
	{From: StateEscalated, Event: EventEscalate, To: StateEscalated},
}, StateReclaimed, StateCancelled, StateAutoCredited)

// applyEvent 应用 FSM 事件；返回下一个状态或 ErrIllegalTransition。

  func applyEvent(cur State, ev Event) (State, error) {
	  next, err := reclaimFSM.Apply(cur, ev)
	  if err != nil {
		  return cur,
			   ErrIllegalTransition
	  }
	   return next, nil
  }

// canApply 检查当前状态是否可接受事件（不修改状态）。

   func canApply(cur State, ev Event) bool {
	  return reclaimFSM.Can(cur, ev)
  }

// SLAResult SLA 检查结果。
type SLAResult struct {
	// Breached 是否已超时。
	Breached bool
	// Event 触发的 FSM 事件（sla_open_breach / sla_reserve_breach）。
	Event Event
	// Deadline 对应的 SLA 截止时间。
	Deadline time.Time
}

// CheckSLA 检查 ReclaimCase 的 SLA 状态。
// 若已超时，返回 Breached=true 及对应 Event；否则 Breached=false。
// 已终态案件直接返回 Breached=false（不做检查）。
   func CheckSLA(c *ReclaimCase, now time.Time) SLAResult {
	   if c == nil || c.State.Terminal() {
		   return SLAResult{}
	   }
	  switch c.State {
	  case StateOpen:
		   if !c.SLAOpenDeadline.IsZero() && now.After(c.SLAOpenDeadline) {
			  return SLAResult{
				  Breached: true,
				   Event: EventSLAOpenBreach,
				  Deadline: c.SLAOpenDeadline,
			  }
		   }
	  case StateReserved:
		  if !c.SLAReserveDeadline.IsZero() && now.After(c.SLAReserveDeadline) {
			   return SLAResult{
				   Breached: true,
				   Event:    EventSLAReserveBreach,
				   Deadline: c.SLAReserveDeadline,
			  }
		  }
	  }
	   return SLAResult{}
  }

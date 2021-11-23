package reconflow

import "fmt"

// edge 状态机跃迁边。

type edge struct {
	From RunState
	Event string
	To    RunState
}

// 合法跃迁边表。

var transitions = []edge{
	{RunStateOpen, "classify_start", RunStateClassifying},
	{RunStateOpen, "escalate", RunStateEscalated},
	{RunStateClassifying, "classify_done_auto", RunStateActing},
	{RunStateClassifying, "classify_done_gate", RunStateWaitingHuman},
	{RunStateClassifying, "classify_done_low", RunStateWaitingHuman},
	{RunStateClassifying, "escalate", RunStateEscalated},
	{RunStateActing, "action_gated", RunStateWaitingHuman},
	{RunStateActing, "all_done", RunStateClosed},
	{RunStateActing, "escalate", RunStateEscalated},
	{RunStateWaitingHuman, "gate_allow", RunStateActing},
	{RunStateWaitingHuman, "gate_deny", RunStateClosed},
	{RunStateWaitingHuman, "escalate", RunStateEscalated},
}

// flowFSM 流程状态机；调用 Transition 前先 CanTransition 检查。

type flowFSM struct {
	index map[RunState]map[string]RunState
}

// newFlowFSM 构造状态机。

   func newFlowFSM() *flowFSM {
	   m := &flowFSM{index: map[RunState]map[string]RunState{}}
	  for _, e := range transitions {
		  if m.index[e.From] == nil {
			  m.index[e.From] = map[string]RunState{}
		   }
		   m.index[e.From][e.Event] = e.To
	   }
	  return m
   }

// singleton，包级共用。
var globalFSM = newFlowFSM()

// CanTransition 检查从 from 经 event 是否有合法跃迁。
  func CanTransition(from RunState, event string) bool {
	   evts, ok := globalFSM.index[from]
	   if !ok {
		  return false
	  }
	   _, ok = evts[event]
	   return ok
  }

// Transition 返回跃迁目标状态；不合法时返回错误。
  func Transition(from RunState, event string) (RunState, error) {
	  evts, ok := globalFSM.index[from]
	   if !ok {
		   return from, fmt.Errorf("%w: no transitions from %s", ErrInvalidState, from)
	  }
	  to, ok := evts[event]
	  if !ok {
		  return from, fmt.Errorf("%w: %s -[%s]-> (undefined)", ErrInvalidState, from, event)
	   }
	   return to, nil
   }

// IsTerminal 判断是否终态。
  func IsTerminal(s RunState) bool {
	   return s == RunStateClosed || s == RunStateEscalated
  }

// AvailableEvents 从给定状态出发的所有合法事件名。

  func AvailableEvents(from RunState) []string {
	   evts, ok := globalFSM.index[from]
	  if !ok {
		   return nil
	  }
	   out := make([]string,
		  0, len(evts))
	   for e := range evts {
		   out = append(out, e)
	  }
	  return out
  }

// classifyEvent 按置信度与动作风险决定分类后应触发的事件。
// 规则：
// - 低置信 → classify_done_low（→ waiting_human）
// - 任一动作 require_gate=true → classify_done_gate（→ waiting_human）
// - 高置信且无强制 gate → classify_done_auto（→ acting）
  func classifyEvent(confidence ConfidenceLevel, actions []ActionDef) string {
	  return ClassifyEvent(confidence, actions)
  }

// ClassifyEvent 按置信度与动作风险决定分类后应触发的 FSM 事件（导出供）。
  func ClassifyEvent(confidence ConfidenceLevel, actions []ActionDef) string {
	  if confidence == ConfidenceLow {
		  return "classify_done_low"
	  }
	   for _, a := range actions {
		   if a.RequireGate || a.Risk == RiskHigh {
			  return "classify_done_gate"
		  }
	  }
	   return "classify_done_auto"
   }

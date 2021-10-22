package restoregate

import "fmt"

// CanTransition 检查从 from 到 to 的状态转换是否合法。
   func CanTransition(from, to PlanState) bool {
	  targets, ok := AllowedTransitions[from]
	   if !ok {
		   return false
	   }
	  for _, t := range targets {
		   if t == to {
			  return true
		  }
	  }
	   return false
  }

// AssertTransition 合法性断言；非法转换返回 ErrInvalidState。
   func AssertTransition(from, to PlanState) error {
	   if !CanTransition(from, to) {
		   return fmt.Errorf("%w: %s → %s", ErrInvalidState, from, to)
	   }
	   return nil
  }

// IsTerminal 判断是否为终态（sealed / aborted）。

   func IsTerminal(s PlanState) bool {
	  return s == StateSealed || s == StateAborted
  }

// IsActive 判断计划是否处于活跃回灌中（freezing_writes / importing / verifying）。

  func IsActive(s PlanState) bool {
	   return s == StateFreezing || s == StateImporting || s == StateVerifying
  }

// NextStates 返回当前状态的合法后继。

  func NextStates(s PlanState) []PlanState {
	  return AllowedTransitions[s]
  }

// StateMachine 持有当前状态并提供安全转换。
type StateMachine struct {
	state PlanState
}

// NewFSM 创建状态机，初始状态为 planned。
   func NewFSM() *StateMachine {
	  return &StateMachine{state: StatePlanned}
  }

// NewFSMFrom 从已持久化状态恢复。

  func NewFSMFrom(s PlanState) *StateMachine {
	  return &StateMachine{state: s}
   }

// State 返回当前状态。

  func (m *StateMachine) State() PlanState { return m.state }

// Transition 执行状态转换，失败返回 ErrInvalidState。
   func (m *StateMachine) Transition(to PlanState) error {
	   if err := AssertTransition(m.state, to); err != nil {
		  return err
	  }
	   m.state = to
	  return nil
   }

// Abort 强制中止（任意非 sealed 状态）。

  func (m *StateMachine) Abort() error {
	   if m.state == StateSealed {
		  return ErrPlanSealed
	   }
	  if m.state == StateAborted {
		   return ErrPlanAborted
	   }
	  m.state = StateAborted
	  return nil
   }

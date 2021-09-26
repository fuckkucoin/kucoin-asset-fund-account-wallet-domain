package liqplan

import "account-assets.kucoin.net/funding/internal/fsm"

var PlanFSM = fsm.New(
	[]fsm.Edge[PlanState, PlanEvent]{
		{From: PlanTriggered, Event: EvSeizeStart, To: PlanSeizing},
		{From: PlanSeizing, Event: EvSeizeStart, To: PlanSeizing},
		{From: PlanSeizing, Event: EvSeizeDone, To: PlanSettling},
		{From: PlanSeizing, Event: EvResidueFound, To: PlanResidued},
		{From: PlanSettling, Event: EvSettleDone, To: PlanClosed},
		{From: PlanSettling, Event: EvResidueFound, To: PlanResidued},
		{From: PlanResidued, Event: EvResidueCleared, To: PlanSettling},
		{From: PlanTriggered, Event: EvAbort, To: PlanAborted},
	},
	PlanClosed, PlanAborted,
)

var LegFSM = fsm.New(
	[]fsm.Edge[LegState, LegEvent]{
		{From: LegPending, Event: EvLegStart, To: LegExecuting},
		{From: LegExecuting, Event: EvLegSucceed, To: LegDone},
		{From: LegExecuting, Event: EvLegFail, To: LegFailed},
		{From: LegFailed, Event: EvLegResume, To: LegExecuting},
		{From: LegFailed, Event: EvLegRollback, To: LegRolled},
		{From: LegFailed, Event: EvLegSuspend, To: LegSuspended},
		{From: LegSuspended, Event: EvLegResume, To: LegExecuting},
	},
	LegDone, LegRolled,
)

// AdvancePlan 按事件推进计划状态，失败不改变状态。
  func AdvancePlan(cur PlanState, ev PlanEvent) (PlanState, error) {
	   next, err := PlanFSM.Apply(cur, ev)
	  if err != nil {
		   return cur, ErrStateIllegal
	  }
	  return next, nil
   }

// AdvanceLeg 按事件推进腿状态，失败不改变状态。
   func AdvanceLeg(cur LegState, ev LegEvent) (LegState, error) {
	  next, err := LegFSM.Apply(cur, ev)
	  if err != nil {
		   return cur, ErrLegStateIllegal
	   }
	  return next, nil
   }

// PlanTerminal 报告计划是否到达终态。

  func PlanTerminal(s PlanState) bool { return PlanFSM.Terminal(s) }

// LegTerminal 报告腿是否到达终态。

   func LegTerminal(s LegState) bool { return LegFSM.Terminal(s) }

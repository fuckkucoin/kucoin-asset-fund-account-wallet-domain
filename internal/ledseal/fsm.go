package ledseal

import "account-assets.kucoin.net/funding/internal/fsm"

// planFSM 返回计划状态机（active ↔ paused）。

  func planFSM() *fsm.Machine[PlanState, string] {
	  return fsm.New([]fsm.Edge[PlanState, string]{
		  {From: PlanActive, Event: "pause", To: PlanPaused},
		  {From: PlanPaused, Event: "activate", To: PlanActive},
	   }, PlanActive)
   }

// batchFSM 返回批次状态机（running → sealed | failed）。

  func batchFSM() *fsm.Machine[BatchState, string] {
	  return fsm.New([]fsm.Edge[BatchState, string]{
		  {From: BatchRunning, Event: "seal", To: BatchSealed},
		  {From: BatchRunning, Event: "fail", To: BatchFailed},
	   }, BatchRunning, BatchSealed, BatchFailed)
   }

// CanSealBatch 批次是否允许封印。

   func CanSealBatch(s BatchState) bool { return s == BatchRunning }

// CanFailBatch 批次是否允许标记失败。

  func CanFailBatch(s BatchState) bool { return s == BatchRunning }

// CanActivatePlan 计划是否允许激活。
   func CanActivatePlan(s PlanState) bool { return s == PlanPaused }

// CanPausePlan 计划是否允许暂停。
  func CanPausePlan(s PlanState) bool { return s == PlanActive }

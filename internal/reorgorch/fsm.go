package reorgorch

import (
	"context"
	"fmt"
)

// ────────────────────────────────────────────────────────────────────────────
// 批次状态跃迁规则
// ────────────────────────────────────────────────────────────────────────────

var batchTransitions = map[string]map[string]bool{
	BatchStateOpen:      {BatchStatePlanning: true},
	BatchStatePlanning:  {BatchStateExecuting: true, BatchStateAborted: true},
	BatchStateExecuting: {BatchStateCommitted: true, BatchStateAborted: true},
	// committed / aborted 为终态，无出边。
}

// BatchFSM 批次状态机操作器。
type BatchFSM struct {
	store *Store
}

// NewBatchFSM 构造 BatchFSM。

  func NewBatchFSM(store *Store) *BatchFSM {
	  return &BatchFSM{store: store}
  }

// Transition 将批次推进到 nextState；非法跃迁返回 ErrBadTransition。
  func (f *BatchFSM) Transition(ctx context.Context, batchID, nextState string) error {
	   batch, err := f.store.GetBatch(ctx, batchID)
	  if err != nil {
		  return err
	   }
	  if batch.State == BatchStateCommitted || batch.State == BatchStateAborted {
		  return ErrBatchAlreadyClosed
	  }
	  if !batchTransitions[batch.State][nextState] {
		   return fmt.Errorf("%w: %s → %s", ErrBadTransition, batch.State, nextState)
	   }
	   return f.store.UpdateBatchState(ctx, batchID, nextState)
   }

// AdvanceBatchFromLegResults 根据当前腿的执行结果，决定批次是否可关闭。
// 若所有腿均 done/skipped → committed；若存在 failed 且全已处理 → aborted。
// 返回新的批次状态（空字符串表示不变化）。
  func (f *BatchFSM) AdvanceBatchFromLegResults(ctx context.Context, batchID string) (string, error) {
	  legs, err := f.store.ListLegs(ctx, batchID)
	   if err != nil {
		  return "", err
	   }
	  if len(legs) == 0 {
		  return "", nil
	   }
	  hasPending := false
	   hasManual := false
	  hasFailed := false
	   for _, leg := range legs {
		  switch leg.State {
		   case LegStatePending:
			  hasPending = true
		   case LegStateManualPending:
			  hasManual = true
		   case LegStateFailed:
			   hasFailed = true
		  }
	   }
	  // 仍有 pending 或 manual_pending 腿，批次未到关闭条件。
	  if hasPending || hasManual {
		  return "", nil
	   }
	   // 全部腿已完成。
	   if hasFailed {
		  // 检查失败腿是否已补偿。
		  for _, leg := range legs {
			   if leg.State == LegStateFailed {
				  // 仍有未补偿的 failed 腿，等待补偿完成。
				  return "", nil
			   }
		   }
		   if err := f.Transition(ctx, batchID, BatchStateAborted); err != nil {
			   return "", err
		   }
		  return BatchStateAborted, nil
	  }
	  if err := f.Transition(ctx, batchID, BatchStateCommitted); err != nil {
		  return "", err
	   }
	   return BatchStateCommitted, nil
   }

// ────────────────────────────────────────────────────────────────────────────
// 腿状态跃迁规则
// ────────────────────────────────────────────────────────────────────────────

var legTransitions = map[string]map[string]bool{
	LegStatePending:       {LegStateDone: true, LegStateSkipped: true, LegStateFailed: true, LegStateManualPending: true},
	LegStateFailed:        {LegStateCompensated: true},
	LegStateManualPending: {LegStatePending: true, LegStateFailed: true},
	// done / skipped / compensated 为终态。
}

// LegFSM 腿状态机操作器。
type LegFSM struct {
	store *Store
}

// NewLegFSM 构造 LegFSM。

   func NewLegFSM(store *Store) *LegFSM {
	  return &LegFSM{store: store}
   }

// Transition 将腿推进到 nextState；非法跃迁返回 ErrBadTransition。

   func (f *LegFSM) Transition(ctx context.Context, legID, nextState, resultNote string) error {
	  leg, err := f.store.GetLeg(ctx, legID)
	  if err != nil {
		  return err
	  }
	  if !legTransitions[leg.State][nextState] {
		  return fmt.Errorf("%w: leg %s state %s → %s", ErrBadTransition, legID, leg.State, nextState)
	  }
	   return f.store.UpdateLegState(ctx,
		  legID, nextState, resultNote)
   }

// MarkManualPending 将腿标记为等待人工确认（仅 pending 可转）。
  func (f *LegFSM) MarkManualPending(ctx context.Context, legID, reason string) error {
	  return f.Transition(ctx, legID, LegStateManualPending, reason)
   }

// Approve 人工确认高危腿，将其从 manual_pending → pending，允许执行器接着。
  func (f *LegFSM) Approve(ctx context.Context, legID, approverNote string) error {
	   return f.Transition(ctx, legID, LegStatePending, "approved: "+approverNote)
  }

// Skip 跳过腿（前序失败时由编排器调用）。
   func (f *LegFSM) Skip(ctx context.Context, legID, reason string) error {
	   return f.Transition(ctx, legID, LegStateSkipped, reason)
   }

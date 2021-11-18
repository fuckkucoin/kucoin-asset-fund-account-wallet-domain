package reorgorch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/reorgacct"
)

// ────────────────────────────────────────────────────────────────────────────
// Orchestrator
// ────────────────────────────────────────────────────────────────────────────
// Orchestrator 链重组冲正编排主服务。
type Orchestrator struct {
	store *Store
	batchFSM *BatchFSM
	legFSM *LegFSM
	guard *GuardManager
	executor *LegExecutor
	acctStore *reorgacct.Store // 只读消费 impact_rows
	log zerolog.Logger
}

// NewOrchestrator 构造 Orchestrator。
  func NewOrchestrator(
	  store *Store,
	   guard *GuardManager,
	   executor *LegExecutor,
	  acctStore *reorgacct.Store,
	  log zerolog.Logger,
   ) *Orchestrator {
	  return &Orchestrator{
		   store:    store,
		  batchFSM: NewBatchFSM(store),
		   legFSM: NewLegFSM(store),
		   guard:     guard,
		   executor:  executor,
		   acctStore: acctStore,
		   log: log,
	  }
  }

// ────────────────────────────────────────────────────────────────────────────
// Plan 规划冲正批次
// ────────────────────────────────────────────────────────────────────────────
// Plan 读取 eventID 对应的 ImpactRow 列表，创建 ReverseBatch 与 ReverseLeg。
// 若相同 eventID 批次已存在，返回 ErrGuardConflict（幂等）。
  func (o *Orchestrator) Plan(ctx context.Context, eventID string) (*ReverseBatch, PlanResult, error) {
	   var res PlanResult
	   // 幂等：同一 event 只创建一个批次。
	  existing, err := o.store.FindBatchByEventID(ctx, eventID)
	   if err != nil {
		   return nil, res, fmt.Errorf("reorgorch plan find batch: %w", err)
	   }
	  if existing != nil {
		  res.BatchID = existing.ID
		  res.TotalLegs = existing.TotalLegs
		  return existing, res, ErrGuardConflict
	  }
	  // 读取影响面明细。
	   impactRows, err := o.acctStore.ListImpactRows(ctx, eventID)
	   if err != nil {
		  return nil, res, fmt.Errorf("reorgorch plan list impact rows: %w", err)
	  }
	  if len(impactRows) == 0 {
		  return nil, res, ErrImpactRowMissing
	  }
	   // 创建批次。
	   rowIDs := extractRowIDs(impactRows)
	   idemKey := batchIdemKey(eventID, rowIDs)
	  now := time.Now().UTC()
	   batch := &ReverseBatch{
		  ID: uuid.New().String(),
		   EventID: eventID,
		  State:        BatchStateOpen,
		  ImpactRowIDs: rowIDs,
		  IdemKey:      idemKey,
		   Note:         fmt.Sprintf("auto plan for event %s", eventID),
		   CreatedAt:    now,
		   UpdatedAt:    now,
	   }
	  if err := o.store.SaveBatch(ctx, batch); err != nil {
		   return nil, res, fmt.Errorf("reorgorch plan save batch: %w", err)
	  }
	   // 推进 → planning。
	   if err := o.batchFSM.Transition(ctx, batch.ID, BatchStatePlanning); err != nil {
		  return nil, res, err
	   }
		  Msg("reorgorch: plan created")
	  return batch, res, nil
   }

// ────────────────────────────────────────────────────────────────────────────
// Execute 执行批次
// ────────────────────────────────────────────────────────────────────────────
// Execute 将批次推进到 executing 并驱动 LegExecutor 执行所有 pending 腿。

  func (o *Orchestrator) Execute(ctx context.Context, batchID string) (ExecuteResult, error) {
	   // planning → executing。
	   if err := o.batchFSM.Transition(ctx, batchID, BatchStateExecuting); err != nil {
		   // 若批次已处于 executing（重入），不视为错误，接着执行。
		  batch, berr := o.store.GetBatch(ctx, batchID)
		   if berr != nil || batch.State != BatchStateExecuting {
			  return ExecuteResult{BatchID: batchID},
				  err
		  }
	   }
	   res, err := o.executor.ExecuteBatch(ctx, batchID)
	  if err != nil {
		   o.log.Error().Err(err).Str("batch_id", batchID).Msg("reorgorch: execute batch error")
		  return res, err
	   }
	   o.log.Info().
		  Str("batch_id", batchID).
		   Str("batch_state", res.BatchState).
		   Int("done", res.Done).Int("failed", res.Failed).Int("skipped", res.Skipped).
		  Msg("reorgorch: execute batch completed")
	   return res, nil
  }

// ────────────────────────────────────────────────────────────────────────────
// Compensate 补偿 failed 腿
// ────────────────────────────────────────────────────────────────────────────
// Compensate 对批次中所有 failed 腿执行补偿（标记 compensated + 写审计行）。

   func (o *Orchestrator) Compensate(ctx context.Context, batchID string) error {
	  legs, err := o.store.ListLegs(ctx, batchID)
	  if err != nil {
		  return err
	  }
	  for _, leg := range legs {
		  if leg.State != LegStateFailed {
			   continue
		  }
		   if err2 := o.legFSM.Transition(ctx, leg.ID, LegStateCompensated,
			  "compensated by orchestrator"); err2 != nil {
			  o.log.Error().Err(err2).Str("leg_id", leg.ID).Msg("reorgorch: compensate leg failed")
		   }
	   }
	  // 尝试将批次推进到 aborted（若所有 failed 腿均已补偿）。
	  _, _ = o.batchFSM.AdvanceBatchFromLegResults(ctx, batchID)
	  return nil
   }

// ────────────────────────────────────────────────────────────────────────────
// ApproveLeg 人工确认高危腿
// ────────────────────────────────────────────────────────────────────────────
// ApproveLeg 将 manual_pending 腿确认为 pending，随即尝试接着执行。

   func (o *Orchestrator) ApproveLeg(ctx context.Context, legID, approverNote string) (ExecuteResult, error) {
	  leg, err := o.store.GetLeg(ctx, legID)
	   if err != nil {
		   return ExecuteResult{}, err
	   }
	  if err := o.legFSM.Approve(ctx, legID, approverNote); err != nil {
		  return ExecuteResult{}, err
	  }
	  // 触发批次接着执行。
	  return o.Execute(ctx, leg.BatchID)
   }

// ────────────────────────────────────────────────────────────────────────────
// AbortBatch 人工中止批次
// ────────────────────────────────────────────────────────────────────────────
// AbortBatch 将批次直接推进到 aborted，跳过所有 pending 腿。
   func (o *Orchestrator) AbortBatch(ctx context.Context, batchID, reason string) error {
	  legs, err := o.store.ListLegs(ctx, batchID)
	   if err != nil {
		  return err
	   }
	   for _, leg := range legs {
		   if leg.State == LegStatePending || leg.State == LegStateManualPending {
			  _ = o.legFSM.Skip(ctx, leg.ID, "batch aborted: "+reason)
		  }
	   }
	   return o.store.UpdateBatchState(ctx,
		  batchID, BatchStateAborted)
  }

// ────────────────────────────────────────────────────────────────────────────
// 内部工具函数
// ────────────────────────────────────────────────────────────────────────────

type legBuildResult struct {
	highRisk int
	liveHash int
}

// buildLegs 将 ImpactRow 列表转换为 ReverseLeg 列表。
  func buildLegs(batchID string, rows []reorgacct.ImpactRow) ([]ReverseLeg, legBuildResult) {
	   var legs []ReverseLeg
	  var res legBuildResult
	   now := time.Now().UTC()
	  for _, row := range rows {
		   legKind := ImpactKindToLegKind[row.Kind]
		   if legKind == "" {
			   continue
		   }
		  execOrder := LegExecutionOrder[legKind]
		  idemKey := legIdemKey(batchID, row.ID, legKind)
		   isHighRisk := legKind == LegKindFreezeRollback || legKind == LegKindDebitReversal
		   isLiveHash := legKind == LegKindIncident
		  if isHighRisk {
			  res.highRisk++
		  }
		  if isLiveHash {
			   res.liveHash++
		  }
		   legs = append(legs, ReverseLeg{
			   ID:      uuid.New().String(),
			   BatchID: batchID,
			   ImpactRowID:    row.ID,
			  Kind:           legKind,
			  ExecutionOrder: execOrder,
			   State: LegStatePending,
			   IdemKey:     idemKey,
			  ReferenceID: row.ReferenceID,
			   Amount: row.AffectedAmount,
			  UserID: row.UserID,
			   Asset:  row.Asset,
			  HighRisk:  isHighRisk,
			  CreatedAt: now,
		  })
	  }
	   // 按 ExecutionOrder 排序保证一致落库顺序。
	  sort.Slice(legs, func(i, j int) bool {
		   return legs[i].ExecutionOrder < legs[j].ExecutionOrder
	   })
	   return legs, res
   }

   func extractRowIDs(rows []reorgacct.ImpactRow) []string {
	   ids := make([]string, len(rows))
	  for i, r := range rows {
		  ids[i] = r.ID
	  }
	  return ids
   }

   func batchIdemKey(eventID string, rowIDs []string) string {
	   sorted := make([]string, len(rowIDs))
	   copy(sorted, rowIDs)
	  sort.Strings(sorted)
	  raw := eventID + "|" + strings.Join(sorted, ",")
	  h := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(h[:16])
   }

   func legIdemKey(batchID, impactRowID, legKind string) string {
	  raw := batchID + "|" + impactRowID + "|" + legKind
	   h := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(h[:])
  }

// newUUID 生成 UUID v4 字符串（供 leg.go 引用）。
  func newUUID() string {
	   return uuid.New().String()
   }

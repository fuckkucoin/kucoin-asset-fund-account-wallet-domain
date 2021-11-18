package reorgorch

import (
	"context"
	"fmt"
	"time"
	"github.com/rs/zerolog"
)

// ────────────────────────────────────────────────────────────────────────────
// 外部服务接口（由调用方注入）
// ────────────────────────────────────────────────────────────────────────────
// LedgerReverser 账本借方冲正接口（对应 KindCredited 腿）。
// 实现方负责幂等；idemKey 为腿 IdemKey。
type LedgerReverser interface {
	ReverseCredit(ctx context.Context, idemKey, transferID, userID, asset, amount string) error
}

// OccupancyReleaser r4 风险占用释放接口（对应 KindOccupied 腿）。
type OccupancyReleaser interface {
	ReleaseOccupancy(ctx context.Context, idemKey, occupancyID, userID, asset, amount string) error
}

// FreezeRollbacker 出金冻结回滚接口（对应 KindFreeze 腿，仅非 live-hash）。
type FreezeRollbacker interface {
	RollbackFreeze(ctx context.Context,
		idemKey, freezeID, userID, asset, amount string) error
}

// HopCloser 划转 hop 收口通知接口（对应 KindHopPartial 腿）。

type HopCloser interface {
	CloseHop(ctx context.Context, idemKey, hopID, userID, asset, amount string) error
}

// IncidentOpener 事故单开单接口（live-hash / 超时 / 失败腿）。

type IncidentOpener interface {
	OpenIncident(ctx context.Context,
		ticket *IncidentTicket) error
}

// ────────────────────────────────────────────────────────────────────────────
// LegExecutor
// ────────────────────────────────────────────────────────────────────────────
// LegExecutor 冲正腿执行器。
type LegExecutor struct {
	store *Store
	guard *GuardManager
	legFSM   *LegFSM
	batchFSM *BatchFSM
	reverser LedgerReverser
	occupancy OccupancyReleaser
	freeze    FreezeRollbacker
	hop       HopCloser
	incident   IncidentOpener
	legTimeout time.Duration
	log        zerolog.Logger
}

// NewLegExecutor 构造 LegExecutor。
   func NewLegExecutor(
	  store *Store,
	  guard *GuardManager,
	  legFSM *LegFSM,
	   batchFSM *BatchFSM,
	  reverser LedgerReverser,
	   occupancy OccupancyReleaser,
	  freeze FreezeRollbacker,
	   hop HopCloser,
	   incident IncidentOpener,
	   legTimeout time.Duration,
	   log zerolog.Logger,
   ) *LegExecutor {
	  if legTimeout <= 0 {
		   legTimeout = 30 * time.Second
	   }
	   return &LegExecutor{
		   store: store, guard: guard, legFSM: legFSM, batchFSM: batchFSM,
		   reverser: reverser, occupancy: occupancy, freeze: freeze,
		   hop: hop, incident: incident, legTimeout: legTimeout, log: log,
	  }
  }

// ExecuteBatch 顺序执行批次中所有 pending 腿（按 ExecutionOrder 排序）。
// 遇到 failed 腿时，对后续依赖腿调用 Skip；不中断整批执行流程。
  func (ex *LegExecutor) ExecuteBatch(ctx context.Context, batchID string) (ExecuteResult, error) {
	   res := ExecuteResult{BatchID: batchID}
	  legs, err := ex.store.ListLegs(ctx, batchID)
	   if err != nil {
		   return res, err
	   }
	   // 按 ExecutionOrder 排序（store 已按 ORDER BY 返回，此处作防御排序）。
	  sortLegs(legs)
	  hasPriorFailed := false
	  for i := range legs {
		   leg := &legs[i]
		  if leg.State != LegStatePending {
			  // 已完成（done/skipped/failed/compensated）或等待人工确认，跳过本轮。
			   if leg.State == LegStateManualPending {
				  res.Manual++
			   }
			  continue
		   }
		   res.Executed++
		   // 前序失败 → 跳过此腿（依赖顺序保护）。
		   if hasPriorFailed && leg.Kind != LegKindIncident {
			   _ = ex.legFSM.Skip(ctx, leg.ID, "prior leg failed; skipped by dependency order")
			  res.Skipped++
			  continue
		   }
		   // 高危腿需人工确认。
		   if leg.HighRisk {
			  _ = ex.legFSM.MarkManualPending(ctx, leg.ID, "high-risk leg; awaiting manual gate")
			  res.Manual++
			  continue
		  }
		   if err2 := ex.executeOne(ctx, leg, batchID); err2 != nil {
			  hasPriorFailed = true
			   res.Failed++
			   // 开事故单。
			   ex.openIncident(ctx, batchID, leg.ID, "leg_failed_manual", err2.Error())
			  res.Incidents++
		   } else {
			   if leg.Kind == LegKindIncident {
				   res.Incidents++
			  } else {
				  res.Done++
			   }
		   }
	  }
	  // 尝试推进批次状态。
	  newState, _ := ex.batchFSM.AdvanceBatchFromLegResults(ctx, batchID)
	  if newState == "" {
		   // 读取当前批次状态。
		  batch, _ := ex.store.GetBatch(ctx, batchID)
		  if batch != nil {
			   newState = batch.State
		  }
	   }
	  res.BatchState = newState
	  return res, nil
  }

// executeOne 执行单条腿。

   func (ex *LegExecutor) executeOne(ctx context.Context, leg *ReverseLeg, batchID string) error {
	  // live-hash 腿只开事故单，不执行任何解冻。
	  if leg.Kind == LegKindIncident {
		  return ex.executeIncidentLeg(ctx, leg, batchID)
	  }
	   // 护栏检查（原子写入）。
	  if err := ex.guard.Lock(ctx, leg.ImpactRowID, leg.Kind, batchID, leg.ID); err != nil {
		   if err == ErrGuardConflict {
			   // 护栏命中 → 跳过（已冲正）。
			  _ = ex.legFSM.Skip(ctx, leg.ID, "guard conflict: already reversed")
			  return nil
		  }
		  _ = ex.legFSM.Transition(ctx, leg.ID, LegStateFailed, "guard lock error: "+err.Error())
		  return err
	   }
	  // 带超时的 context。
	  execCtx,
		  cancel := context.WithTimeout(ctx, ex.legTimeout)
	  defer cancel()
	  now := time.Now().UTC()
	   _ = ex.store.MarkLegStarted(execCtx, leg.ID, now)
	   var execErr error
	  switch leg.Kind {
	  case LegKindDebitReversal:
		   execErr = ex.executeDebitReversal(execCtx, leg)
	   case LegKindOccupancyRelease:
		  execErr = ex.executeOccupancyRelease(execCtx, leg)
	  case LegKindFreezeRollback:
		  execErr = ex.executeFreezeRollback(execCtx, leg)
	  case LegKindHopClose:
		   execErr = ex.executeHopClose(execCtx, leg)
	  default:
		   execErr = fmt.Errorf("unknown leg kind: %s", leg.Kind)
	  }
	   fin := time.Now().UTC()
	  if execErr != nil {
		  _ = ex.store.MarkLegFinished(ctx, leg.ID, fin)
		   _ = ex.legFSM.Transition(ctx, leg.ID, LegStateFailed, execErr.Error())
		   ex.log.Error().Err(execErr).Str("leg_id", leg.ID).Str("kind", leg.Kind).Msg("reorgorch: leg failed")
		  return execErr
	   }
	   _ = ex.store.MarkLegFinished(ctx, leg.ID, fin)
	   _ = ex.legFSM.Transition(ctx, leg.ID, LegStateDone, "ok")
	  ex.log.Info().Str("leg_id", leg.ID).Str("kind", leg.Kind).Msg("reorgorch: leg done")
	   return nil
   }

// executeIncidentLeg live-hash 事故腿：仅开事故单，不解冻。
  func (ex *LegExecutor) executeIncidentLeg(ctx context.Context, leg *ReverseLeg, batchID string) error {
	  desc := fmt.Sprintf("live-hash withdraw leg user=%s asset=%s amount=%s ref=%s; no auto-unfreeze",
		  leg.UserID, leg.Asset, leg.Amount, leg.ReferenceID)
	  ex.openIncident(ctx, batchID, leg.ID, "live_hash_manual", desc)
	  _ = ex.legFSM.Transition(ctx, leg.ID, LegStateDone, "incident opened; manual review required")
	   return nil
  }

  func (ex *LegExecutor) executeDebitReversal(ctx context.Context, leg *ReverseLeg) error {
	   if ex.reverser == nil {
		   return fmt.Errorf("LedgerReverser not configured")
	  }
	   return ex.reverser.ReverseCredit(ctx, leg.IdemKey, leg.ReferenceID, leg.UserID, leg.Asset, leg.Amount)
   }

  func (ex *LegExecutor) executeOccupancyRelease(ctx context.Context, leg *ReverseLeg) error {
	  if ex.occupancy == nil {
		   return fmt.Errorf("OccupancyReleaser not configured")
	  }
	   return ex.occupancy.ReleaseOccupancy(ctx, leg.IdemKey, leg.ReferenceID, leg.UserID, leg.Asset, leg.Amount)
  }

  func (ex *LegExecutor) executeFreezeRollback(ctx context.Context, leg *ReverseLeg) error {
	  if ex.freeze == nil {
		   return fmt.Errorf("FreezeRollbacker not configured")
	  }
	  return ex.freeze.RollbackFreeze(ctx, leg.IdemKey, leg.ReferenceID, leg.UserID, leg.Asset, leg.Amount)
   }

  func (ex *LegExecutor) executeHopClose(ctx context.Context, leg *ReverseLeg) error {
	  if ex.hop == nil {
		   return fmt.Errorf("HopCloser not configured")
	  }
	   return ex.hop.CloseHop(ctx, leg.IdemKey, leg.ReferenceID, leg.UserID, leg.Asset, leg.Amount)
   }

// openIncident 写入事故单（忽略失败；告警由指标兜底）。

   func (ex *LegExecutor) openIncident(ctx context.Context, batchID, legID, kind, desc string) {
	  if ex.incident == nil {
		  return
	  }
	  now := time.Now().UTC()
	   ticket := &IncidentTicket{
		  ID: newUUID(), BatchID: batchID, LegID: legID,
		   Kind: kind, Description: desc, State: "open",
		  CreatedAt: now, UpdatedAt: now,
	  }
	  if err := ex.incident.OpenIncident(ctx, ticket); err != nil {
		  ex.log.Error().Err(err).Str("batch_id", batchID).Str("leg_id", legID).
			  Msg("reorgorch: open incident failed")
	  }
   }

// ────────────────────────────────────────────────────────────────────────────
// sortLegs 按 ExecutionOrder 稳定排序（避免 import sort 繁琐）
// ────────────────────────────────────────────────────────────────────────────

   func sortLegs(legs []ReverseLeg) {
	  // 简单插入排序（腿数量通常 < 20，线性成本可接受）。
	   for i := 1; i < len(legs); i++ {
		   for j := i; j > 0 && legs[j].ExecutionOrder < legs[j-1].ExecutionOrder; j-- {
			   legs[j], legs[j-1] = legs[j-1], legs[j]
		   }
	   }
  }

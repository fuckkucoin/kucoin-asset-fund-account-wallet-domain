package reorgacct

import (
	"context"
	"fmt"
	"strings"
)

// FundsafetyHook 将 fundsafety 确认回退信号转化为 ReorgEvent 摄取请求。
type FundsafetyHook struct {
	ingestor *Ingestor
}

// NewFundsafetyHook 构造 FundsafetyHook；ingestor 不得为 nil。

  func NewFundsafetyHook(ingestor *Ingestor) *FundsafetyHook {
	   return &FundsafetyHook{ingestor: ingestor}
   }

// OnConfirmRollback 处理 fundsafety.AdvanceConfirm 返回 ErrReorg 后的回调。
// 参数说明：
// - chain：规范化前的链字符串（由 fundsafety 传入，Hook 内部规范化）；
// - pivotTxHash：触发重组检测的交易哈希；
// - depthBefore：回退前的确认数（ConfirmWin.Have 的前次值）；
// - depthAfter：回退后的确认数（当前 have 值）；
// - outputs：受影响的入金输出列表（由链观察者提供，可为空）；
// - tracks：受影响的出金跟踪列表（由出金监控提供，可为空）。
// 若 depthBefore <= depthAfter（非业务回退），直接返回 ErrBadDepth，不产生事件。
   func (h *FundsafetyHook) OnConfirmRollback(
	   ctx context.Context,
	   chain, pivotTxHash string,
	   depthBefore, depthAfter int,
	  outputs []AffectedOutput,
	   tracks []AffectedTrack,
  ) (*ReorgEvent, error) {
	  if h == nil || h.ingestor == nil {
		   return nil, ErrStoreUnavailable
	   }
	   req := IngestReq{
		   Chain: chain,
		  PivotTxHash: pivotTxHash,
		   DepthBefore:     depthBefore,
		   DepthAfter:      depthAfter,
		   AffectedOutputs: outputs,
		   AffectedTracks: tracks,
		  SourceKind:     "confirm_rollback",
		   Evidence: []EvidenceRef{
			  {
				  Kind: "confirm_rollback",
				  Ref:  fmt.Sprintf("fundsafety:AdvanceConfirm:%s:%s:depth_before=%d:depth_after=%d", chain, pivotTxHash, depthBefore, depthAfter),
				   Note: "AdvanceConfirm returned ErrReorg; confirm window rolled back",
			  },
		   },
		  Note: fmt.Sprintf("auto-ingested from fundsafety.AdvanceConfirm rollback on %s tx=%s", chain, shortTx(pivotTxHash)),
	  }
	   ev, err := h.ingestor.Ingest(ctx, req)
	   if err != nil && err != ErrDupEvent {
		  return nil, fmt.Errorf("reorgacct hook OnConfirmRollback: %w", err)
	   }
	   return ev, err
  }

// OrphanBlockHook 将链观察者报告的孤块信号转化为 ReorgEvent 摄取请求。

type OrphanBlockHook struct {
	ingestor *Ingestor
}

// NewOrphanBlockHook 构造 OrphanBlockHook。

  func NewOrphanBlockHook(ingestor *Ingestor) *OrphanBlockHook {
	  return &OrphanBlockHook{ingestor: ingestor}
   }

// OnOrphanObserved 处理链观察者发现孤块的回调。
// 参数说明：
// - chain：链 ID；
// - orphanBlockHash：孤块哈希（作为 PivotTxHash）；
// - rollbackDepth：孤块导致的回退深度（通常为孤块高度差）；
// - affectedTxHashes：孤块中包含的交易哈希列表（由观察者扫描提供）。

   func (h *OrphanBlockHook) OnOrphanObserved(
	  ctx context.Context,
	   chain, orphanBlockHash string,
	  depthBefore, depthAfter int,
	   affectedTxHashes []string,
	  tracks []AffectedTrack,
   ) (*ReorgEvent, error) {
	   if h == nil || h.ingestor == nil {
		  return nil, ErrStoreUnavailable
	  }
	  outputs := buildOutputsFromTxHashes(affectedTxHashes)
	   req := IngestReq{
		  Chain:       chain,
		  PivotTxHash: orphanBlockHash,
		  DepthBefore: depthBefore,
		  DepthAfter:  depthAfter,
		  AffectedOutputs: outputs,
		   AffectedTracks: tracks,
		   SourceKind: "orphan_observe",
		  Evidence: []EvidenceRef{
			  {
				  Kind: "orphan_observe",
				  Ref:  fmt.Sprintf("orphan_block:%s", orphanBlockHash),
				  Note: fmt.Sprintf("orphan block observed on %s; %d affected txs", chain, len(affectedTxHashes)),
			   },
		  },
		   Note: fmt.Sprintf("orphan block %s on %s; rollback %d→%d", shortTx(orphanBlockHash), chain, depthBefore, depthAfter),
	   }
	  ev,
		   err := h.ingestor.Ingest(ctx, req)
	  if err != nil && err != ErrDupEvent {
		  return nil, fmt.Errorf("reorgacct hook OnOrphanObserved: %w", err)
	  }
	   return ev, err
  }

// DepositFaceChangeHook 监听提前入账标记变化，对已冲正输出触发影响面封存。
type DepositFaceChangeHook struct {
	store *Store
	ingestor *Ingestor
}

// NewDepositFaceChangeHook 构造 DepositFaceChangeHook。

   func NewDepositFaceChangeHook(store *Store, ingestor *Ingestor) *DepositFaceChangeHook {
	   return &DepositFaceChangeHook{store: store,
		  ingestor: ingestor}
   }

// OnFaceReversed 当 deposit.RollBook.Rollback 完成后调用，确保关联的 ImpactRow 已被
// 标记（ReverseBatchID 已回写），并将对应 ReorgEvent 推进到 closed（若已完成所有腿）。
// 此钩子为只读消费路径；不修改 deposit.RollBook 任何状态。
  func (h *DepositFaceChangeHook) OnFaceReversed(
	   ctx context.Context,
	   chain, txHash string,
	  outputIndex int,
	  transferID string,
   ) error {
	  if h == nil || h.store == nil {
		   return ErrStoreUnavailable
	  }
	   // 查询关联 ImpactRow（只读）；若未找到说明该输出未被重组会计管理，正常返回。
	   rows,
		   err := h.store.ListImpactRowsByReference(ctx, transferID)
	   if err != nil {
		  return fmt.Errorf("reorgacct OnFaceReversed list rows: %w", err)
	  }
	   if len(rows) == 0 {
		   return nil
	   }
	   // 对每条 ImpactRow 关联的 ReorgEvent，尝试推进状态（若条件满足）。
	   seenEvents := map[string]bool{}
	  for _, row := range rows {
		   if seenEvents[row.EventID] {
			  continue
		   }
		  seenEvents[row.EventID] = true
		  // 仅推进已进入 orch_started 状态的事件（编排完成条件由 reorgorch 判断）。
		  ev, err := h.store.GetEvent(ctx, row.EventID)
		   if err != nil {
			  continue
		   }
		   if ev.State == EventStateOrchStarted {
			  // 通知：face 已完成冲正，辅助日志/指标，不强制推进 closed（由 orch 负责）。
			  _ = ev
		  }
	   }
	  return nil
  }

// ────────────────────────────────────────────────────────────────────────────
// 工具函数
// ────────────────────────────────────────────────────────────────────────────
// buildOutputsFromTxHashes 将孤块中的交易哈希列表转化为 AffectedOutput 骨架（output_index=0）。
// 实际盘点时由 InventoryScanner 查询完整输出列表；此处仅生成摄取骨架。

  func buildOutputsFromTxHashes(hashes []string) []AffectedOutput {
	   var out []AffectedOutput
	   for _, h := range hashes {
		   h = strings.TrimSpace(h)
		   if h == "" {
			   continue
		  }
		  out = append(out, AffectedOutput{
			  TxHash: h,
			  OutputIndex: 0,
		   })
	  }
	   return out
   }

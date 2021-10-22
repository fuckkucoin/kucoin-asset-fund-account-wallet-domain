package reorgacct

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ────────────────────────────────────────────────────────────────────────────
// FacePhase 与 deposit.Face 兼容的相位常量
// ────────────────────────────────────────────────────────────────────────────

const (
	// FacePhaseConfirming 确认中 / 待冲正（与 deposit.Face.Confirming = true 对应）。
	FacePhaseConfirming = "confirming"
	// FacePhaseReversed 已冲正完成（与 deposit.Face.Reversed = true 对应）。
	FacePhaseReversed = "reversed"
	// FacePhaseLiveHashHold live-hash 人工持有，禁止自动冲正。
	FacePhaseLiveHashHold = "live_hash_hold"
	// FacePhaseProcessing 其他资金位处理中（非 credited）。
	FacePhaseProcessing = "processing"
)

// ────────────────────────────────────────────────────────────────────────────
// ImpactFaceView 冲正影响面对应的读面视图
// ────────────────────────────────────────────────────────────────────────────
// ImpactFaceView 兼容 deposit.Face 字段语义的只读视图。

type ImpactFaceView struct {
	// Chain 链 ID。
	Chain string `json:"chain"`
	// ReferenceID 被影响对象 ID（TransferID / OccupancyID 等）。
	ReferenceID string `json:"reference_id"`
	// UserID 用户 ID。
	UserID string `json:"user_id"`
	// Asset 资产代码。
	Asset string `json:"asset"`
	// ImpactKind 资金位类型。
	ImpactKind string `json:"impact_kind"`
	// Phase 读面相位（与 deposit.Face.Phase 字段兼容）。
	Phase string `json:"phase"`
	// PhaseLabel 中文相位说明。
	PhaseLabel string `json:"phase_label"`
	// Confirming 是否为「确认中」状态（与 deposit.Face.Confirming 兼容）。
	Confirming bool `json:"confirming"`
	// Reversed 是否已冲正完成（与 deposit.Face.Reversed 兼容）。
	Reversed bool `json:"reversed"`
	// AffectedAmount 影响金额。
	AffectedAmount string `json:"affected_amount"`
	// ReverseBatchID 关联批次（空表示尚未规划）。
	ReverseBatchID string `json:"reverse_batch_id,omitempty"`
	// BatchState 批次状态（空表示尚未规划）。
	BatchState string `json:"batch_state,omitempty"`
	// EventID 关联重组事件 ID。
	EventID string `json:"event_id"`
	// Note 说明。
	Note string `json:"note,omitempty"`
	// UpdatedAt 最近更新时间。
	UpdatedAt string `json:"updated_at,omitempty"`
}

// FaceLabel 返回中文相位说明（与 deposit 包的 FaceLabel 函数逻辑一致）。
  func FaceLabel(phase string) string {
	   switch phase {
	  case FacePhaseConfirming:
		  return "确认中（待冲正）"
	   case FacePhaseReversed:
		   return "已冲正"
	  case FacePhaseLiveHashHold:
		  return "live-hash 人工持有"
	  case FacePhaseProcessing:
		  return "处理中"
	  default:
		  return phase
	  }
   }

// ────────────────────────────────────────────────────────────────────────────
// FaceCompat 兼容层
// ────────────────────────────────────────────────────────────────────────────
// BatchStateReader 只读批次状态查询接口（避免直接依赖 reorgorch）。

type BatchStateReader interface {
	// GetBatchState 按 batchID 查询批次状态；不存在时返回空字符串。
	GetBatchState(ctx context.Context, batchID string) (string, error)
}

// FaceCompat 将 ImpactRow 映射为兼容 deposit.Face 语义的读面视图。

type FaceCompat struct {
	batchReader BatchStateReader
}

// NewFaceCompat 构造 FaceCompat；batchReader 可为 nil（批次状态列显示为空）。

   func NewFaceCompat(batchReader BatchStateReader) *FaceCompat {
	   return &FaceCompat{batchReader: batchReader}
   }

// ToFaceView 将 ImpactRow 转换为 ImpactFaceView（只读）。
// chain 由调用方从关联 ReorgEvent 中传入。
  func (fc *FaceCompat) ToFaceView(ctx context.Context, row ImpactRow, chain string) (ImpactFaceView, error) {
	   view := ImpactFaceView{
		  Chain: chain,
		  ReferenceID: row.ReferenceID,
		  UserID: row.UserID,
		  Asset: row.Asset,
		  ImpactKind: row.Kind,
		   AffectedAmount: row.AffectedAmount,
		   ReverseBatchID: row.ReverseBatchID,
		  EventID:        row.EventID,
		  Note:           row.Note,
		   UpdatedAt: row.CreatedAt.UTC().Format(time.RFC3339),
	  }
	  // 若有批次，查询批次状态。
	  if row.ReverseBatchID != "" && fc.batchReader != nil {
		   batchState, err := fc.batchReader.GetBatchState(ctx, row.ReverseBatchID)
		   if err == nil {
			  view.BatchState = batchState
		  }
	  }
	   // 按 Kind + BatchState 派生相位。
	   view.Phase, view.Confirming, view.Reversed = derivePhase(row.Kind, view.BatchState)
	   view.PhaseLabel = FaceLabel(view.Phase)
	  return view, nil
  }

// ToFaceViews 批量转换 ImpactRow 列表。
   func (fc *FaceCompat) ToFaceViews(ctx context.Context, rows []ImpactRow, chain string) ([]ImpactFaceView, error) {
	  views := make([]ImpactFaceView, 0, len(rows))
	  for _, row := range rows {
		  v,
			   err := fc.ToFaceView(ctx, row, chain)
		  if err != nil {
			   return nil, fmt.Errorf("ToFaceView row %s: %w", row.ID, err)
		  }
		   views = append(views, v)
	  }
	  return views, nil
  }

// derivePhase 根据 Kind 和 BatchState 派生读面相位。
   func derivePhase(kind, batchState string) (phase string, confirming, reversed bool) {
	  switch kind {
	  case KindCredited:
		  switch batchState {
		  case "committed":
			   return FacePhaseReversed, false, true
		   case "executing", "planning":
			   return FacePhaseConfirming, true, false
		   default:
			  // open / 空（尚未规划）→ 确认中
			   return FacePhaseConfirming, true, false
		  }
	   case KindLiveHash:
		   return FacePhaseLiveHashHold, false, false
	   default:
		  // occupied / freeze / hop_partial → processing
		  if batchState == "committed" {
			  return FacePhaseReversed, false, true
		  }
		  return FacePhaseProcessing, false, false
	  }
  }

// ────────────────────────────────────────────────────────────────────────────
// FaceCompatStore SQL 辅助（读 reverse_batches.state）
// ────────────────────────────────────────────────────────────────────────────
// FaceCompatStore 实现 BatchStateReader，通过 SQL 查询批次状态。

type FaceCompatStore struct {
	store *Store
}

// NewFaceCompatStore 构造 FaceCompatStore。
   func NewFaceCompatStore(store *Store) *FaceCompatStore {
	  return &FaceCompatStore{store: store}
   }

// GetBatchState 从 reorgorch 的 reverse_batches 表查询批次状态（只读）。
// 注意：此查询跨包（reorgacct 查询 reorgorch 的表），需确保同一 DB 连接。
   func (fcs *FaceCompatStore) GetBatchState(ctx context.Context, batchID string) (string, error) {
	  if fcs == nil || fcs.store == nil || fcs.store.db == nil {
		  return "", ErrStoreUnavailable
	  }
	  var state string
	   q := `SELECT state FROM reverse_batches WHERE id = ? LIMIT 1`
	  err := fcs.store.db.QueryRowContext(ctx, q, batchID).Scan(&state)
	   if err != nil {
		  return "", err
	  }
	  return strings.TrimSpace(state), nil
   }

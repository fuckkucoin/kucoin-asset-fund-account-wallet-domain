package reorgacct

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ────────────────────────────────────────────────────────────────────────────
// DepositFaceLookup 只读入金相位查询（由调用方注入，解耦 deposit 包）
// ────────────────────────────────────────────────────────────────────────────
// DepositFace 只读入金相位（与 deposit.Face 结构一致但不直接依赖）。
type DepositFace struct {
	Phase      string // confirming / credited / reversed
	Reversed   bool
	Confirming bool
	Amount     string
	UserID     string
	Asset      string
	TransferID string
}

// DepositFaceLookup 由调用方实现；查询不得修改 deposit 包任何状态。
type DepositFaceLookup interface {
	// LookupFace 按 (chain, txHash, outputIndex) 查询只读入金相位。
	// 未找到时返回 (zero, false, nil)。
	LookupFace(ctx context.Context, chain, txHash string, outputIndex int) (DepositFace, bool, error)
}

// OccupancyLookup 只读 r4 风险占用查询（由调用方注入）。
type OccupancyLookup interface {
	// LookupOccupancy 按 transferID 查询关联的风险占用单 ID 与金额。
	// 未找到时返回 ("", "", nil)。
	LookupOccupancy(ctx context.Context, transferID string) (occupancyID, amount string, err error)
}

// HopLookup 只读 hop 状态查询（划转 hop 半截检测）。

type HopLookup interface {
	// LookupHop 按 transferID 查询 hop 记录；若存在半截 hop 返回 (hopID, amount, true, nil)。
	LookupHop(ctx context.Context, transferID string) (hopID, amount string, partial bool, err error)
}

// ────────────────────────────────────────────────────────────────────────────
// InventoryScanner
// ────────────────────────────────────────────────────────────────────────────
// InventoryScanner 对 ReorgEvent 进行影响面盘点。

type InventoryScanner struct {
	store    *Store
	ingestor *Ingestor
	deposit DepositFaceLookup
	occupancy OccupancyLookup
	hop       HopLookup
}

// NewInventoryScanner 构造 InventoryScanner。deposit/occupancy/hop 可为 nil（对应类型的盘点将跳过）。
   func NewInventoryScanner(
	   store *Store,
	  ingestor *Ingestor,
	   deposit DepositFaceLookup,
	   occupancy OccupancyLookup,
	  hop HopLookup,
   ) *InventoryScanner {
	  return &InventoryScanner{
		  store: store,
		   ingestor: ingestor,
		   deposit:  deposit,
		   occupancy: occupancy,
		   hop: hop,
	   }
  }

// Scan 对 event 执行影响面盘点，产出并持久化 ImpactRow 列表。
// 若 event.State != pending，返回错误（已盘点的事件不重入）。
  func (sc *InventoryScanner) Scan(ctx context.Context, event *ReorgEvent) ([]ImpactRow, error) {
	  if sc == nil || sc.store == nil {
		   return nil, ErrStoreUnavailable
	  }
	  if event.State != EventStatePending {
		  if event.State == EventStateScanned {
			   // 已盘点，返回已有行。
			   return sc.store.ListImpactRows(ctx,
				  event.ID)
		   }
		   return nil, ErrImpactSealed
	   }
	   var rows []ImpactRow
	  now := time.Now().UTC()
	   // ── 步骤1：盘点入金输出（已贷记 + 风险占用）──────────────────────────
	  for _, out := range event.AffectedOutputs {
		  creditRows, err := sc.scanOutput(ctx, event.ID, event.Chain, out, now)
		  if err != nil {
			   _ = sc.ingestor.Transition(ctx, event.ID, EventStateError)
			   return nil, fmt.Errorf("reorgacct scan output %s#%d: %w", out.TxHash, out.OutputIndex, err)
		  }
		   rows = append(rows, creditRows...)
	  }
	   // ── 步骤2：盘点出金跟踪（冻结 + live-hash）────────────────────────────
	   for _, track := range event.AffectedTracks {
		   trackRows := sc.scanTrack(event.ID, track, now)
		   rows = append(rows, trackRows...)
	   }
	   // ── 步骤3：盘点 hop 半截（若有 deposit 依赖）──────────────────────────
	   if sc.hop != nil {
		   hopRows, err := sc.scanHops(ctx, event.ID, event.AffectedOutputs, now)
		  if err != nil {
			   _ = sc.ingestor.Transition(ctx, event.ID, EventStateError)
			  return nil, fmt.Errorf("reorgacct scan hops: %w", err)
		  }
		  rows = append(rows, hopRows...)
	   }
	  // ── 落库 ImpactRow ──────────────────────────────────────────────────────
	   if len(rows) > 0 {
		   if err := sc.store.SaveImpactRows(ctx, rows); err != nil {
			   _ = sc.ingestor.Transition(ctx, event.ID, EventStateError)
			   return nil, fmt.Errorf("reorgacct scan save rows: %w", err)
		  }
	  }
	   // ── 推进事件状态 → scanned ──────────────────────────────────────────────
	  if err := sc.ingestor.Transition(ctx, event.ID, EventStateScanned); err != nil {
		  return nil, fmt.Errorf("reorgacct scan transition: %w", err)
	   }
	   return rows, nil
   }

// scanOutput 盘点单条入金输出的影响面。

  func (sc *InventoryScanner) scanOutput(
	  ctx context.Context,
	  eventID, chain string,
	  out AffectedOutput,
	   now time.Time,
  ) ([]ImpactRow, error) {
	   var rows []ImpactRow
	  // 查询 deposit Face（只读）。
	   if sc.deposit == nil {
		   return nil, nil
	   }
	   face, found, err := sc.deposit.LookupFace(ctx, chain, out.TxHash, out.OutputIndex)
	   if err != nil {
		  return nil, err
	  }
	  if !found {
		   // 未找到贷记记录；该输出可能从未被认领，无需冲正。
		  return nil, nil
	   }
	  amount := normalizeAmount(firstNonEmptyStr(out.Amount, face.Amount))
	  if amount == "" || amount == "0" {
		  return nil, nil
	   }
	   userID := firstNonEmptyStr(out.UserID,
		  face.UserID)
	   asset := firstNonEmptyStr(out.Asset, face.Asset)
	  transferID := firstNonEmptyStr(out.TransferID, face.TransferID)
	   // 已贷记（confirmed 或 confirming 均算入影响面）。
	  if !face.Reversed {
		  rows = append(rows, ImpactRow{
			   ID: uuid.New().String(),
			   EventID: eventID,
			   UserID:  userID,
			   Asset:   asset,
			   Kind: KindCredited,
			   ReferenceID:    transferID,
			  AffectedAmount: amount,
			   Note:           fmt.Sprintf("deposit face phase=%s tx=%s#%d", face.Phase, shortTx(out.TxHash), out.OutputIndex),
			  CreatedAt:      now,
		   })
	   }
	  // r4 风险占用（若有）。
	   if sc.occupancy != nil && transferID != "" {
		   occID, occAmt, occErr := sc.occupancy.LookupOccupancy(ctx, transferID)
		  if occErr == nil && occID != "" && occAmt != "" && occAmt != "0" {
			   rows = append(rows, ImpactRow{
				  ID:      uuid.New().String(),
				  EventID: eventID,
				   UserID: userID,
				   Asset:  asset,
				  Kind:   KindOccupied,
				   ReferenceID:    occID,
				   AffectedAmount: normalizeAmount(occAmt),
				  Note:           fmt.Sprintf("r4 occupancy id=%s transfer=%s", occID, transferID),
				  CreatedAt: now,
			  })
		   }
	   }
	  return rows, nil
  }

// scanTrack 盘点单条出金跟踪的影响面（冻结 / live-hash）。
  func (sc *InventoryScanner) scanTrack(eventID string, track AffectedTrack, now time.Time) []ImpactRow {
	   var rows []ImpactRow
	  if track.LiveHash {
		  // live-hash 出金：禁止自动解冻，仅记录 KindLiveHash 行，供 reorgorch 开事故单。
		  rows = append(rows, ImpactRow{
			   ID:      uuid.New().String(),
			   EventID: eventID,
			   UserID:  track.UserID,
			   Asset:   track.Asset,
			   Kind: KindLiveHash,
			   ReferenceID: track.WithdrawID,
			   AffectedAmount: normalizeAmount(track.FreezeAmount),
			  Note: fmt.Sprintf("live-hash broadcast_id=%s; manual review required", track.BroadcastID),
			   CreatedAt: now,
		  })
		   return rows
	   }
	  // 普通冻结（非 live-hash）。
	   if track.FreezeID != "" && track.FreezeAmount != "" && track.FreezeAmount != "0" {
		   rows = append(rows, ImpactRow{
			   ID: uuid.New().String(),
			   EventID: eventID,
			  UserID:  track.UserID,
			  Asset:   track.Asset,
			  Kind:    KindFreeze,
			   ReferenceID:    track.FreezeID,
			  AffectedAmount: normalizeAmount(track.FreezeAmount),
			   Note:           fmt.Sprintf("withdraw freeze withdraw_id=%s", track.WithdrawID),
			  CreatedAt:      now,
		   })
	  }
	   return rows
  }

// scanHops 对已受影响入金的 TransferID 查询 hop 半截。

  func (sc *InventoryScanner) scanHops(
	   ctx context.Context,
	  eventID string,
	  outputs []AffectedOutput,
	   now time.Time,
  ) ([]ImpactRow, error) {
	   var rows []ImpactRow
	   seenTransfer := map[string]bool{}
	  for _, out := range outputs {
		  tid := strings.TrimSpace(out.TransferID)
		   if tid == "" || seenTransfer[tid] {
			   continue
		   }
		   seenTransfer[tid] = true
		   hopID, hopAmt, partial, err := sc.hop.LookupHop(ctx, tid)
		   if err != nil || !partial || hopID == "" {
			   continue
		  }
		   rows = append(rows, ImpactRow{
			  ID: uuid.New().String(),
			  EventID: eventID,
			   UserID:         out.UserID,
			  Asset:          out.Asset,
			  Kind:           KindHopPartial,
			   ReferenceID:    hopID,
			  AffectedAmount: normalizeAmount(hopAmt),
			   Note: fmt.Sprintf("transfer hop partial transfer_id=%s hop_id=%s", tid, hopID),
			  CreatedAt: now,
		   })
	  }
	  return rows, nil
  }

// ────────────────────────────────────────────────────────────────────────────
// Summarize 汇总影响面
// ────────────────────────────────────────────────────────────────────────────
// Summarize 对 ImpactRow 列表产出 ImpactSummary（仅做加法，不改任何状态）。

  func Summarize(eventID string, rows []ImpactRow) ImpactSummary {
	   sum := ImpactSummary{
		  EventID: eventID,
		   ByAsset: map[string]string{},
	  }
	   totals := map[string]decimal.Decimal{
		   KindCredited: decimal.Zero,
		   KindOccupied: decimal.Zero,
		  KindFreeze:     decimal.Zero,
		   KindHopPartial: decimal.Zero,
	  }
	   byAsset := map[string]decimal.Decimal{}
	  for _, r := range rows {
		   amt, err := decimal.NewFromString(r.AffectedAmount)
		   if err != nil || amt.IsNegative() {
			   continue
		   }
		  if r.Kind == KindLiveHash {
			  sum.LiveHashCount++
			   continue
		   }
		   if _, ok := totals[r.Kind]; ok {
			  totals[r.Kind] = totals[r.Kind].Add(amt)
		   }
		  byAsset[r.Asset] = byAsset[r.Asset].Add(amt)
	  }
	   sum.TotalCredited = totals[KindCredited].String()
	   sum.TotalOccupied = totals[KindOccupied].String()
	   sum.TotalFreeze = totals[KindFreeze].String()
	  sum.TotalHopPartial = totals[KindHopPartial].String()
	   sum.RowCount = len(rows)
	   for asset, v := range byAsset {
		   sum.ByAsset[asset] = v.String()
	  }
	  return sum
   }

// ────────────────────────────────────────────────────────────────────────────
// 工具函数（包内）
// ────────────────────────────────────────────────────────────────────────────

   func normalizeAmount(s string) string {
	  s = strings.TrimSpace(s)
	   if s == "" {
		  return "0"
	   }
	  d, err := decimal.NewFromString(s)
	  if err != nil || d.IsNegative() {
		   return "0"
	   }
	  return d.String()
   }

   func firstNonEmptyStr(a, b string) string {
	   if strings.TrimSpace(a) != "" {
		   return strings.TrimSpace(a)
	  }
	  return strings.TrimSpace(b)
   }

   func shortTx(h string) string {
	   if len(h) <= 12 {
		  return h
	   }
	  return h[:6] + "…" + h[len(h)-4:]
  }

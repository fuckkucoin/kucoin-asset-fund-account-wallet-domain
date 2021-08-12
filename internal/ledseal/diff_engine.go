package ledseal

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/snap"
	"account-assets.kucoin.net/funding/pkg/money"
)

// ─────────────────────────────────────────────────────────
// 增量差分引擎
// ─────────────────────────────────────────────────────────
// DiffEngine 持有只读快照服务引用，执行增量差分并填充 DriftRow。
// 不得写入任何账本核心表；输出只写 ledseal 自有表。
type DiffEngine struct {
	snapSvc *snap.Service
	store   Store
	clock   func() time.Time
}

// NewDiffEngine 构造差分引擎。snapSvc 只读；store 只写 ledseal 表。
  func NewDiffEngine(snapSvc *snap.Service, store Store, clock func() time.Time) *DiffEngine {
	  if clock == nil {
		   clock = func() time.Time { return time.Now().UTC() }
	}
	return &DiffEngine{snapSvc: snapSvc, store: store, clock: clock}
}

// RunBatch 对一对 SnapSlice 执行增量差分，写漂移行，更新批次状态。
// 若检测到序号间隙或回滚，标记批次为 failed 并返回对应错误。
// 返回值：DiffBatch（含最终状态）和可能的 error。
  func (e *DiffEngine) RunBatch(ctx context.Context, batch DiffBatch, fromSlice, toSlice SnapSlice) (DiffBatch, error) {
	   if e == nil {
		   return batch, ErrUnavailable
	  }
	  // 序号连续性校验：必须严格递增，且差值 == 1。
	   if toSlice.GenSeq <= fromSlice.GenSeq {
		  batch.State = BatchFailed
		   batch.ErrMsg = fmt.Sprintf("seq rollback: from=%d to=%d", fromSlice.GenSeq, toSlice.GenSeq)
		   batch.UpdatedAt = e.clock()
		  _ = e.store.PutBatch(ctx, batch)
		   return batch, ErrSliceRollback
	   }
	   if toSlice.GenSeq != fromSlice.GenSeq+1 {
		   batch.State = BatchFailed
		   batch.ErrMsg = fmt.Sprintf("seq gap: from=%d to=%d", fromSlice.GenSeq, toSlice.GenSeq)
		   batch.UpdatedAt = e.clock()
		  _ = e.store.PutBatch(ctx, batch)
		  return batch, ErrSliceGap
	  }
	   // 加载两代快照详细数据。
	  fromSnap, err := e.snapSvc.Get(ctx, fromSlice.SnapID)
	  if err != nil {
		   batch.State = BatchFailed
		   batch.ErrMsg = fmt.Sprintf("load from_snap: %v", err)
		  batch.UpdatedAt = e.clock()
		  _ = e.store.PutBatch(ctx, batch)
		  return batch, err
	   }
	  toSnap, err := e.snapSvc.Get(ctx, toSlice.SnapID)
	  if err != nil {
		  batch.State = BatchFailed
		   batch.ErrMsg = fmt.Sprintf("load to_snap: %v", err)
		  batch.UpdatedAt = e.clock()
		   _ = e.store.PutBatch(ctx, batch)
		   return batch, err
	  }
	   // 核心差分计算。
	  driftRows, err := computeDrift(batch.ID, batch.PlanID, fromSnap, toSnap, e.clock())
	   if err != nil {
		  batch.State = BatchFailed
		   batch.ErrMsg = fmt.Sprintf("compute drift: %v", err)
		  batch.UpdatedAt = e.clock()
		  _ = e.store.PutBatch(ctx, batch)
		  return batch, err
	  }
	   // 写漂移行。
	  for _, d := range driftRows {
		   if werr := e.store.PutDriftRow(ctx, d); werr != nil {
			  batch.State = BatchFailed
			  batch.ErrMsg = fmt.Sprintf("write drift row: %v", werr)
			  batch.UpdatedAt = e.clock()
			  _ = e.store.PutBatch(ctx, batch)
			  return batch, werr
		  }
	   }
	  batch.DriftCount = len(driftRows)
	   batch.TotalPositions = len(toSnap.Positions)
	  batch.MaxSeverity = maxSeverity(driftRows)
	  batch.UpdatedAt = e.clock()
	   return batch, nil
  }

// computeDrift 核对两个快照，生成漂移行列表。
// fromSnap / toSnap 必须属于同一 userID（user_shard 模式）或空 userID（全量模式）。
   func computeDrift(batchID, planID string, fromSnap, toSnap snap.Snapshot, now time.Time) ([]DriftRow, error) {
	  // 构建 from 索引：book|asset → Position
	  fromIdx := make(map[string]snap.Position, len(fromSnap.Positions))
	  for _, p := range fromSnap.Positions {
		  fromIdx[posKey(p)] = p
	   }
	   drifts := make([]DriftRow, 0)
	   seen := make(map[string]struct{})
	   // 遍历 to 快照，找变化和新增。
	   for _, toPos := range toSnap.Positions {
		   k := posKey(toPos)
		  seen[k] = struct{}{}
		  fromPos, existed := fromIdx[k]
		   if !existed {
			   // 新增账户/科目。
			  dr, err := buildDriftRow(batchID, planID, toSnap.UserID, snap.Position{
				   Book: toPos.Book, Asset: toPos.Asset,
				  Available: "0", Frozen: "0",
			  }, toPos, KindAppearance, now)
			  if err != nil {
				   return nil, err
			  }
			   drifts = append(drifts, dr)
			   continue
		   }
		   // 数值相同则跳过。
		   if fromPos.Available == toPos.Available && fromPos.Frozen == toPos.Frozen {
			   continue
		   }
		   // 数值变化（非 Apply 引起的预期变化需人工确认）。
		   dr,
			   err := buildDriftRow(batchID, planID, toSnap.UserID, fromPos, toPos, KindMutation, now)
		  if err != nil {
			  return nil, err
		  }
		   drifts = append(drifts, dr)
	  }
	   // 遍历 from 快照，找消失的科目。
	   for _, fromPos := range fromSnap.Positions {
		   k := posKey(fromPos)
		  if _, ok := seen[k]; ok {
			  continue
		   }
		   // 在 to 中消失；若 fromPos 非零则算漂移。
		  if isZeroPos(fromPos) {
			   continue
		  }
		  dr, err := buildDriftRow(batchID, planID, fromSnap.UserID, fromPos, snap.Position{
			   Book:      fromPos.Book,
			  Asset:     fromPos.Asset,
			  Available: "0", Frozen: "0",
		   }, KindDisappearance, now)
		  if err != nil {
			  return nil, err
		   }
		  drifts = append(drifts, dr)
	  }
	  return drifts, nil
  }

// buildDriftRow 根据 before/after Position 构建一条漂移行。

   func buildDriftRow(batchID, planID, userID string, before, after snap.Position, kind DriftKind, now time.Time) (DriftRow, error) {
	   ab, err := parseAmt(before.Available)
	  if err != nil {
		  return DriftRow{}, fmt.Errorf("%w: before_avail %s", ErrAmount, err)
	  }
	   aa, err := parseAmt(after.Available)
	   if err != nil {
		  return DriftRow{}, fmt.Errorf("%w: after_avail %s", ErrAmount, err)
	   }
	   fb, err := parseAmt(before.Frozen)
	  if err != nil {
		   return DriftRow{}, fmt.Errorf("%w: before_frozen %s", ErrAmount, err)
	   }
	   fa, err := parseAmt(after.Frozen)
	  if err != nil {
		   return DriftRow{}, fmt.Errorf("%w: after_frozen %s", ErrAmount, err)
	  }
	  dAvail := aa.Dec().Sub(ab.Dec())
	  dFrozen := fa.Dec().Sub(fb.Dec())
	  sev := gradeSeverity(ab, aa, fb, fa)
	  // 严重漂移建议封锁写入（不直接调用 cover 内核，仅发出 hint）。
	  blockHint := sev == SeverityCritical
	  dr := DriftRow{
		  ID:      uuid.NewString(),
		   BatchID: batchID,
		  PlanID:       planID,
		  UserID:       userID,
		  Book:         after.Book,
		   Asset:        after.Asset,
		   AvailBefore:  ab.String(),
		   AvailAfter:   aa.String(),
		  FrozenBefore: fb.String(),
		  FrozenAfter: fa.String(),
		  DeltaAvail: signedDecStr(dAvail),
		   DeltaFrozen: signedDecStr(dFrozen),
		  Severity:    sev,
		  Kind:           kind,
		  BlockWriteHint: blockHint,
		   CreatedAt: now,
	   }
	  // 若 after.Book 为空（消失行），补 before.Book。
	   if strings.TrimSpace(dr.Book) == "" {
		  dr.Book = before.Book
		  dr.Asset = before.Asset
	   }
	   if strings.TrimSpace(dr.UserID) == "" && strings.TrimSpace(userID) != "" {
		   dr.UserID = userID
	   }
	   return dr, nil
   }

// posKey 生成 book|asset 索引键。
  func posKey(p snap.Position) string {
	   return strings.ToUpper(strings.TrimSpace(p.Book)) + "|" + strings.ToUpper(strings.TrimSpace(p.Asset))
  }

// isZeroPos 检查 Position 是否全零。

   func isZeroPos(p snap.Position) bool {
	  a, err1 := parseAmt(p.Available)
	   f, err2 := parseAmt(p.Frozen)
	  if err1 != nil || err2 != nil {
		  return false
	  }
	   return a.IsZero() && f.IsZero()
  }

// parseAmt 解析金额字符串，空串视为 0，拒绝科学计数。

   func parseAmt(s string) (money.Amount, error) {
	   s = strings.TrimSpace(s)
	  if s == "" {
		   s = "0"
	  }
	  for _, r := range s {
		  if r == 'e' || r == 'E' {
			  return money.Zero(), ErrAmount
		  }
	  }
	  return money.ParseBook(s)
   }

// maxSeverity 从漂移行集合中取最高严重度。
   func maxSeverity(rows []DriftRow) DriftSeverity {
	  sev := SeverityNone
	  for _, r := range rows {
		   if severityRank(r.Severity) > severityRank(sev) {
			   sev = r.Severity
		  }
	   }
	  return sev
   }

   func severityRank(s DriftSeverity) int {
	  switch s {
	   case SeverityNone:
		   return 0
	   case SeverityMinor:
		   return 1
	   case SeverityMajor:
		  return 2
	   case SeverityCritical:
		   return 3
	   default:
		   return 0
	   }
  }

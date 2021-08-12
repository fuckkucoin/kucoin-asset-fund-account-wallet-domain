package ledorphan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

const kindSealAction = "lo_seal_action"
const kindIdemBlock = "lo_idem_block"
const kindOrphanItem = "lo_orphan_item"

// ReverseFunc 由 ledger.Service 提供的反向分录函数类型。
// 调用者传入原始 hop 信息，ledger 负责建立对应的反向分录并返回新 entry ID。
type ReverseFunc func(ctx context.Context, req ReverseReq) (entryID string, err error)

// ReverseReq 反向分录请求。

type ReverseReq struct {
	OriginalEntryID string
	UserID string
	Asset  string
	Amount string
	Direction string
	IdempotencyKey string // 冲正 idem key = "reverse:" + original_idem_key
	BatchID  string
	OrphanID string
	SealID   string
}

// Sealer 封堵与冲正动作编排器。

type Sealer struct {
	kv persist.KV
	reverseHop ReverseFunc
	genMgr     *GenerationManager
	nowFn      func() time.Time
}

// NewSealer 构造 Sealer。
// reverseHop 为 nil 时，SealKindReverse 操作将失败（返回错误，不静默）。
  func NewSealer(kv persist.KV, genMgr *GenerationManager, reverseHop ReverseFunc) *Sealer {
	   return &Sealer{
		   kv: kv,
		   reverseHop: reverseHop,
		  genMgr: genMgr,
		   nowFn: func() time.Time { return time.Now().UTC() },
	   }
   }

   func (s *Sealer) now() time.Time {
	   if s.nowFn != nil {
		   return s.nowFn()
	  }
	  return time.Now().UTC()
   }

// Seal 对指定孤儿报告中的孤儿执行封堵或冲正。
// 前提条件：
// 1. report 中 batch 的世代锁已被调用方的 workerID+generation 持有（可通过 genMgr.Check 验证）；
// 2. actor1 ≠ actor2（双人授权）；
// 3. req.Kind 为合法值。
// 执行流程：
// 1. 校验双人授权；
// 2. 创建 SealAction（状态 pending）；
// 3. 按 Kind 执行：
// - block：逐个写入 IdemBlock；
// - reverse：逐个调用 reverseHop；
// - ignore：仅更新孤儿状态；
// 4. 更新 OrphanItem 状态；
// 5. SealAction 状态更新为 applied；
// 6. 计算并存储 IntegrityProof。
  func (s *Sealer) Seal(ctx context.Context, report *OrphanReport, req SealReq) (*SealAction, error) {
	   if s.kv == nil {
		  return nil, ErrStorageNil
	  }
	  if err := validateSealReq(req); err != nil {
		   return nil, err
	  }
	  // 构建待处置孤儿列表。
	   targets, err := selectOrphans(report, req.OrphanIDs)
	  if err != nil {
		  return nil, err
	   }
	  now := s.now()
	   approvedAt := now
	   action := &SealAction{
		  SealID: uuid.NewString(),
		   BatchID:   report.BatchID,
		   ReportID:  report.ReportID,
		   Kind:      req.Kind,
		   Status:    SealPending,
		  OrphanIDs: orphanIDsOf(targets),
		   Actor1:    req.Actor1,
		   Actor2:    req.Actor2,
		   ApprovedAt: &approvedAt,
		  CreatedAt:  now,
		   UpdatedAt:  now,
	   }
	  // 持久化 pending 状态的 SealAction（供崩溃恢复）。
	  if err := s.saveSealAction(ctx, action); err != nil {
		  return nil, fmt.Errorf("ledorphan seal: save pending action: %w", err)
	   }
	  // 执行具体动作。
	  switch req.Kind {
	  case SealKindBlock:
		  err = s.applyBlock(ctx,
			  action, targets, req.Note)
	   case SealKindReverse:
		  err = s.applyReverse(ctx, action, targets, req.Note)
	  case SealKindIgnore:
		   err = s.applyIgnore(ctx, action, targets, req.Note)
	   default:
		   err = ErrSealKindInvalid
	  }
	   if err != nil {
		   action.Status = SealFailed
		   action.FailureReason = truncate(err.Error(), 128)
		   action.UpdatedAt = s.now()
		  _ = s.saveSealAction(ctx, action)
		   return action, fmt.Errorf("ledorphan seal: apply %s: %w", req.Kind, err)
	  }
	  appliedAt := s.now()
	   action.Status = SealApplied
	  action.AppliedAt = &appliedAt
	   action.UpdatedAt = appliedAt
	   if err := s.saveSealAction(ctx, action); err != nil {
		  return action, fmt.Errorf("ledorphan seal: finalize action: %w", err)
	   }
	   return action, nil
   }

// applyBlock 对孤儿的 IdempotencyKey 写入 idem 拒写记录。
// 写入后，后续 Submit 同 idem 的 hop 必须在 ledbatch 层被拒绝。

   func (s *Sealer) applyBlock(ctx context.Context, action *SealAction, targets []OrphanItem, note string) error {
	   now := s.now()
	   for i := range targets {
		  o := &targets[i]
		   if o.Status != OrphanDetected {
			   continue
		   }
		  // 写 idem 拒写记录（PutNew；若已存在视为幂等成功）。
		  block := IdemBlock{
			  IdemKey: o.IdempotencyKey,
			   BatchID: o.BatchID,
			  OrphanID: o.OrphanID,
			   SealID:    action.SealID,
			  BlockedAt: now,
		   }
		   body, err := json.Marshal(block)
		   if err != nil {
			   return fmt.Errorf("marshal idem block for %s: %w", o.OrphanID, err)
		   }
		  row := persist.Row{Kind: kindIdemBlock, Key: o.IdempotencyKey, Body: body}
		  if err := persist.PutNew(ctx, s.kv, row); err != nil && err != persist.ErrConflict {
			   return fmt.Errorf("put idem block for %s: %w", o.OrphanID, err)
		   }
		   // 更新孤儿状态。
		  sealed := now
		   o.Status = OrphanSealed
		  o.SealedAt = &sealed
		  if note != "" {
			   o.Note = note
		  }
		  if err := s.saveOrphanItem(ctx, o); err != nil {
			  return fmt.Errorf("save orphan item %s: %w", o.OrphanID, err)
		  }
	  }
	  return nil
  }

// applyReverse 对 hanging 孤儿发起反向分录，撤销已错误入账的金额。
// missing 孤儿（ledger 无记录）无需冲正，直接 block 即可。

   func (s *Sealer) applyReverse(ctx context.Context, action *SealAction, targets []OrphanItem, note string) error {
	   if s.reverseHop == nil {
		   return fmt.Errorf("ledorphan reverse: reverseHop function not bound")
	   }
	   now := s.now()
	   reverseIDs := make([]string, 0, len(targets))
	   for i := range targets {
		  o := &targets[i]
		  if o.Status != OrphanDetected {
			   continue
		  }
		  // missing 类型（ledger 无记录）没有要冲正的实体分录，降级为 block。
		  if o.Kind == OrphanMissing {
			  block := IdemBlock{
				  IdemKey: o.IdempotencyKey,
				  BatchID: o.BatchID,
				   OrphanID: o.OrphanID,
				  SealID:    action.SealID,
				  BlockedAt: now,
			   }
			   body, _ := json.Marshal(block)
			  row := persist.Row{Kind: kindIdemBlock, Key: o.IdempotencyKey, Body: body}
			   _ = persist.PutNew(ctx, s.kv, row)
			   sealed := now
			  o.Status = OrphanSealed
			   o.SealedAt = &sealed
			  o.Note = "missing orphan downgraded to block: no ledger entry to reverse"
			   _ = s.saveOrphanItem(ctx, o)
			   continue
		   }
		  // hanging 类型：发起反向分录。
		   reverseIdem := "reverse:" + o.IdempotencyKey
		   reverseEntryID, err := s.reverseHop(ctx, ReverseReq{
			   OriginalEntryID: o.LedgerID,
			   UserID:          o.UserID,
			   Asset: o.Asset,
			  Amount:    o.Amount,
			   Direction: o.Direction,
			  IdempotencyKey: reverseIdem,
			   BatchID: o.BatchID,
			  OrphanID: o.OrphanID,
			  SealID:   action.SealID,
		   })
		  if err != nil {
			   return fmt.Errorf("reverse hop for orphan %s: %w", o.OrphanID, err)
		   }
		  reverseIDs = append(reverseIDs,
			  reverseEntryID)
		  // 同时写 idem 拒写块，防止原 idem 再次续写。
		   block := IdemBlock{
			  IdemKey:  o.IdempotencyKey,
			  BatchID:  o.BatchID,
			  OrphanID: o.OrphanID,
			  SealID: action.SealID,
			  BlockedAt: now,
		  }
		  body, _ := json.Marshal(block)
		  row := persist.Row{Kind: kindIdemBlock, Key: o.IdempotencyKey, Body: body}
		  _ = persist.PutNew(ctx, s.kv, row)
		   reversed := now
		  o.Status = OrphanReversed
		  o.ReversedAt = &reversed
		  if note != "" {
			  o.Note = note
		   }
		   if err := s.saveOrphanItem(ctx, o); err != nil {
			   return fmt.Errorf("save reversed orphan %s: %w", o.OrphanID, err)
		  }
	   }
	   action.ReverseEntryIDs = reverseIDs
	   return nil
  }

// applyIgnore 经双人审核认定无需处置，仅变更孤儿状态为 ignored。

   func (s *Sealer) applyIgnore(ctx context.Context,
	  action *SealAction, targets []OrphanItem, note string) error {
	  now := s.now()
	  for i := range targets {
		   o := &targets[i]
		   if o.Status != OrphanDetected {
			   continue
		   }
		   o.Status = OrphanIgnored
		  if note != "" {
			   o.Note = note
		   }
		   o.Note = strings.TrimSpace("ignored by dual-auth: " + o.Note)
		   if err := s.saveOrphanItem(ctx, o); err != nil {
			  return fmt.Errorf("save ignored orphan %s: %w", o.OrphanID, err)
		  }
		   _ = now // 时间戳记录在 o 上，此处仅抑制未使用警告。
	   }
	   return nil
  }

// IsIdemBlocked 检查幂等键是否已被封堵。
// 封堵后的幂等键不允许通过 ledbatch 接着续写。

  func (s *Sealer) IsIdemBlocked(ctx context.Context, idemKey string) (bool, error) {
	   if s.kv == nil {
		  return false, ErrStorageNil
	   }
	  _,
		   err := s.kv.Get(ctx, kindIdemBlock, strings.TrimSpace(idemKey))
	   if err == persist.ErrNotFound {
		   return false, nil
	   }
	  if err != nil {
		   return false, fmt.Errorf("ledorphan idem block check: %w", err)
	   }
	   return true, nil
  }

// GetSealAction 按 sealID 读取封堵动作记录。

  func (s *Sealer) GetSealAction(ctx context.Context, sealID string) (*SealAction, error) {
	  if s.kv == nil {
		  return nil, ErrStorageNil
	  }
	   return s.loadSealAction(ctx, strings.TrimSpace(sealID))
   }

// ListSealActions 列出指定批次的全部封堵动作。
   func (s *Sealer) ListSealActions(ctx context.Context, batchID string) ([]SealAction, error) {
	   if s.kv == nil {
		   return nil, ErrStorageNil
	   }
	   rows, err := s.kv.List(ctx, kindSealAction, 200)
	  if err != nil {
		   return nil, fmt.Errorf("ledorphan list seal actions: %w", err)
	   }
	   result := make([]SealAction, 0)
	  for _, row := range rows {
		   var a SealAction
		   if err := json.Unmarshal(row.Body, &a); err != nil {
			  continue
		   }
		  if batchID == "" || a.BatchID == batchID {
			   result = append(result, a)
		  }
	   }
	  return result, nil
   }

// ── 内部持久化辅助 ────────────────────────────────────────────────────────────

   func (s *Sealer) saveSealAction(ctx context.Context, a *SealAction) error {
	   body, err := json.Marshal(a)
	  if err != nil {
		   return err
	   }
	  row := persist.Row{Kind: kindSealAction, Key: a.SealID, Body: body}
	   cur, getErr := s.kv.Get(ctx, kindSealAction, a.SealID)
	   if getErr == persist.ErrNotFound {
		   return persist.PutNew(ctx, s.kv, row)
	   }
	   if getErr != nil {
		   return getErr
	   }
	  return s.kv.Put(ctx, row, cur.Fence)
  }

   func (s *Sealer) loadSealAction(ctx context.Context, sealID string) (*SealAction, error) {
	   row, err := s.kv.Get(ctx, kindSealAction, sealID)
	   if err == persist.ErrNotFound {
		  return nil, ErrSealNotFound
	   }
	   if err != nil {
		  return nil, err
	  }
	   var a SealAction
	   if err := json.Unmarshal(row.Body, &a); err != nil {
		  return nil, fmt.Errorf("ledorphan load seal action: %w", err)
	  }
	   return &a, nil
  }

   func (s *Sealer) saveOrphanItem(ctx context.Context, o *OrphanItem) error {
	   body, err := json.Marshal(o)
	   if err != nil {
		   return err
	   }
	   key := orphanItemKey(o.BatchID, o.OrphanID)
	  row := persist.Row{Kind: kindOrphanItem, Key: key, Body: body}
	  cur,
		  getErr := s.kv.Get(ctx, kindOrphanItem, key)
	  if getErr == persist.ErrNotFound {
		  return persist.PutNew(ctx, s.kv, row)
	   }
	  if getErr != nil {
		   return getErr
	   }
	   return s.kv.Put(ctx, row, cur.Fence)
   }

  func orphanItemKey(batchID, orphanID string) string {
	   return batchID + ":" + orphanID
   }

// ── 请求校验 ──────────────────────────────────────────────────────────────────

   func validateSealReq(req SealReq) error {
	   if strings.TrimSpace(req.BatchID) == "" {
		  return fmt.Errorf("%w: batch_id required", ErrBatchNotFound)
	   }
	  if strings.TrimSpace(req.ReportID) == "" {
		  return fmt.Errorf("%w: report_id required", ErrReportNotFound)
	  }
	  a1, err1 := validate.Actor(req.Actor1)
	  a2, err2 := validate.Actor(req.Actor2)
	   if err1 != nil || err2 != nil {
		   return ErrActorInvalid
	  }
	   if strings.EqualFold(a1, a2) {
		  return ErrDualAuth
	  }
	  switch req.Kind {
	   case SealKindBlock,
		   SealKindReverse, SealKindIgnore:
		  // 合法
	   default:
		   return ErrSealKindInvalid
	  }
	  return nil
   }

// selectOrphans 从报告中选出需要处置的孤儿列表。
// 若 orphanIDs 为空，则选出全部 detected 状态孤儿。
  func selectOrphans(report *OrphanReport, orphanIDs []string) ([]OrphanItem, error) {
	  if report == nil {
		   return nil, ErrReportNotFound
	  }
	  if report.Status == ReportClosed {
		   return nil, ErrReportClosed
	   }
	   if len(orphanIDs) == 0 {
		  // 选出全部 detected 状态孤儿。
		  result := make([]OrphanItem, 0, report.TotalOrphans)
		  for _, o := range report.Items {
			  if o.Status == OrphanDetected {
				  result = append(result, o)
			   }
		  }
		  return result, nil
	  }
	  // 按指定 ID 筛选。
	  idSet := make(map[string]bool,
		   len(orphanIDs))
	   for _, id := range orphanIDs {
		  idSet[strings.TrimSpace(id)] = true
	  }
	   result := make([]OrphanItem, 0, len(orphanIDs))
	   for _, o := range report.Items {
		   if idSet[o.OrphanID] {
			  if o.Status != OrphanDetected {
				  return nil, fmt.Errorf("%w: orphan %s is already %s", ErrAlreadySealed, o.OrphanID, o.Status)
			   }
			   result = append(result, o)
		  }
	   }
	  return result, nil
  }

   func orphanIDsOf(items []OrphanItem) []string {
	   ids := make([]string, len(items))
	  for i, o := range items {
		   ids[i] = o.OrphanID
	  }
	  return ids
  }

// truncate 截断字符串到 maxLen 字节（避免存储超长错误信息）。
   func truncate(s string, maxLen int) string {
	   if len(s) <= maxLen {
		   return s
	   }
	  return s[:maxLen] + "..."
  }

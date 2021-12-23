package wdpartial

import (
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// CloseoutBook BatchCloseout 存储接口。

type CloseoutBook interface {
	// ExistsByBatch 检查指定批次是否已有收口单。
	ExistsByBatch(wdbatchID string) bool
	// Save 保存收口单（只写一次，幂等）。
	Save(c *BatchCloseout) error
	// GetByBatch 获取指定批次的收口单。
	GetByBatch(wdbatchID string) (*BatchCloseout, error)
}

// ConserveGuard Conserve 守卫。

type ConserveGuard struct {
	book     ResidueBook
	proofs   ProofBook
	closeout CloseoutBook
	mutex    *MutexManager
	clock func() time.Time
}

// NewConserveGuard 构造 ConserveGuard。
   func NewConserveGuard(book ResidueBook, proofs ProofBook, closeout CloseoutBook, mutex *MutexManager) *ConserveGuard {
	   return &ConserveGuard{
		  book: book,
		   proofs: proofs,
		   closeout: closeout,
		  mutex:    mutex,
		  clock:    func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟。

   func (g *ConserveGuard) WithClock(c func() time.Time) *ConserveGuard {
	  if g != nil && c != nil {
		  g.clock = c
	   }
	   return g
  }

   // BatchSummary wdbatch 批次汇总（只读消费，不修改 wdbatch）。
  type BatchSummary struct {
	   WDBatchID string
	  Requested string // 原批次总请求额
	  Submitted string // 已提交额
	  Skipped   string // 跳过额
	  Failed string // 失败额
  }

   // CheckConserve 计算 Conserve 等式是否成立，返回 ConserveResult（不写收口单）。
   // 外部可用此方法做 dry-run 校验，无副作用。
  func (g *ConserveGuard) CheckConserve(summary BatchSummary) (*ConserveResult, error) {
	  if g == nil {
		  return nil, ErrUnavailable
	   }
	   items,
		   err := g.book.ListByBatch(summary.WDBatchID)
	   if err != nil {
		   return nil, err
	   }
	  result := &ConserveResult{
		   WDBatchID: summary.WDBatchID,
		   Requested: summary.Requested,
		  Submitted: summary.Submitted,
	  }
	  skippedRel := decimal.Zero
	   failedRel := decimal.Zero
	   escalated := decimal.Zero
	  for _, it := range items {
		  if !it.Terminal() {
			  result.Balanced = false
			   result.Note = fmt.Sprintf("residue_id=%s seq=%d not in terminal state", it.ID, it.Seq)
			   return result, nil
		   }
		   amt, err := decimal.NewFromString(it.Amount)
		   if err != nil {
			   return nil, fmt.Errorf("parse amount residue_id=%s: %w", it.ID, err)
		   }
		  switch it.Origin {
		  case ResidueOriginSkipped:
			   skippedRel = skippedRel.Add(amt)
			   if it.Status == ResidueEscalated {
				   escalated = escalated.Add(amt)
			  }
		  case ResidueOriginFailed:
			  failedRel = failedRel.Add(amt)
			  if it.Status == ResidueEscalated {
				   escalated = escalated.Add(amt)
			   }
		   }
	  }
	  // 计算 Conserve 等式右侧
	   sub, err := decimal.NewFromString(orEmpty(summary.Submitted))
	  if err != nil {
		  return nil, fmt.Errorf("parse submitted: %w", err)
	   }
	  right := sub.Add(skippedRel).Add(failedRel)
	   req, err := decimal.NewFromString(orEmpty(summary.Requested))
	   if err != nil {
		   return nil, fmt.Errorf("parse requested: %w", err)
	  }
	   result.SkippedReleased = skippedRel.String()
	  result.FailedReleased = failedRel.String()
	   result.EscalatedAmount = escalated.String()
	   result.Balanced = req.Equal(right)
	  if !result.Balanced {
		  result.Note = fmt.Sprintf("requested=%s != submitted=%s + skipped_rel=%s + failed_rel=%s (right=%s)",
			   req.String(), sub.String(), skippedRel.String(), failedRel.String(), right.String())
	   }
	   return result, nil
   }

// CloseOutReq 收口请求参数。

type CloseOutReq struct {
	Summary     BatchSummary
	Actor       string
	LeaseHolder string
	LeaseGen int64
}

// CloseOut 执行批次收口：校验全部残留已处置、证明齐全、Conserve 成立，写入 BatchCloseout。
// 调用者须持有有效 ResumeLease，且 Gen 未变（由 MutexManager.Verify 校验）。
   func (g *ConserveGuard) CloseOut(req CloseOutReq) (*BatchCloseout, error) {
	   if g == nil {
		  return nil, ErrUnavailable
	   }
	   req.Summary.WDBatchID = strings.TrimSpace(req.Summary.WDBatchID)
	   req.Actor = strings.TrimSpace(req.Actor)
	  if req.Summary.WDBatchID == "" {
		   return nil, ErrBatchID
	  }
	   if req.Actor == "" {
		  return nil, ErrActor
	   }
	   // 幂等：已有收口单
	  if g.closeout.ExistsByBatch(req.Summary.WDBatchID) {
		  return g.closeout.GetByBatch(req.Summary.WDBatchID)
	   }
	  // 校验 ResumeLease 世代
	   if g.mutex != nil {
		  if err := g.mutex.Verify(req.Summary.WDBatchID, req.LeaseHolder, req.LeaseGen); err != nil {
			  return nil, err
		  }
	   }
	  // 证明齐全校验
	  items,
		  err := g.book.ListByBatch(req.Summary.WDBatchID)
	  if err != nil {
		  return nil, err
	  }
	  for _, it := range items {
		  if !it.Terminal() {
			  return nil, fmt.Errorf("%w: residue_id=%s seq=%d",
				  ErrProofMissing, it.ID, it.Seq)
		   }
		  if it.Status == ResidueReleased || it.Status == ResidueEscalated {
			  if !g.proofs.ExistsByResidue(it.ID) {
				  return nil, fmt.Errorf("%w: residue_id=%s",
					  ErrProofMissing, it.ID)
			   }
		   }
	   }
	  // Conserve 等式校验
	   cr, err := g.CheckConserve(req.Summary)
	  if err != nil {
		   return nil, err
	  }
	  if !cr.Balanced {
		   return nil, fmt.Errorf("%w: %s", ErrConserve, cr.Note)
	   }
	  // 判断批次收口状态（有升账则 escalated，否则 closed）
	  bStatus := BatchResidueClosed
	   escalatedAmt, _ := decimal.NewFromString(cr.EscalatedAmount)
	   if escalatedAmt.IsPositive() {
		   bStatus = BatchResidueEscalated
	   }
	  proofList,
		  _ := g.proofs.ListByBatch(req.Summary.WDBatchID)
	  closeout := &BatchCloseout{
		   ID: uuid.NewString(),
		  WDBatchID: req.Summary.WDBatchID,
		   Status:    bStatus,
		  Requested:       cr.Requested,
		  Submitted:       cr.Submitted,
		   SkippedReleased: cr.SkippedReleased,
		  FailedReleased: cr.FailedReleased,
		   EscalatedAmount: cr.EscalatedAmount,
		  ConserveOK:   cr.Balanced,
		   ConserveNote: cr.Note,
		   ProofCount:   len(proofList),
		   ResidueCount: len(items),
		  Actor: req.Actor,
		  ClosedAt: g.clock(),
	   }
	  if err := g.closeout.Save(closeout); err != nil {
		   return nil, err
	  }
	  return closeout, nil
   }

// AssertNoDoubleFreezeOnRetry 校验重提新批次中的 item 不得与已释放的残留项重复冻结。
// 设计：通过比较新批次 items 的 idempotency_key 与已记录残留项的 IdemKey：
// - 若新 idem_key 与旧 idem_key 相同（未加后缀区分），且旧项已 released，则报 ErrConserve。
// - 新 idem_key 须是旧 idem_key + "-retry-N" 形态（约定）。
// 调用时机：wdbatch Retry 产生新批次，Submit 前由调用者主动校验。

  func AssertNoDoubleFreezeOnRetry(retryItems []BatchItemView, existingResidues []*ResidueItem) error {
	   releasedKeys := make(map[string]bool, len(existingResidues))
	  for _, r := range existingResidues {
		   if r.Status == ResidueReleased {
			  releasedKeys[r.IdemKey] = true
		   }
	   }
	   for _, it := range retryItems {
		   if releasedKeys[it.IdempotencyKey] {
			   return fmt.Errorf("%w: idem_key=%s already released in prior batch",
				  ErrConserve, it.IdempotencyKey)
		  }
	   }
	   return nil
  }

// orEmpty 处理空字符串为 "0"。

  func orEmpty(s string) string {
	   s = strings.TrimSpace(s)
	  if s == "" {
		   return "0"
	   }
	  return s
   }

// ─── 内存 CloseoutBook 实现 ───────────────────────────────────────────────────
// MemCloseoutBook 内存 CloseoutBook，仅供使用。
type MemCloseoutBook struct {
	byBatch map[string]*BatchCloseout
}

// NewMemCloseoutBook 构造内存 CloseoutBook。

  func NewMemCloseoutBook() *MemCloseoutBook {
	   return &MemCloseoutBook{byBatch: make(map[string]*BatchCloseout)}
   }

  func (b *MemCloseoutBook) ExistsByBatch(wdbatchID string) bool {
	  if b == nil {
		   return false
	   }
	  _, ok := b.byBatch[wdbatchID]
	   return ok
   }

  func (b *MemCloseoutBook) Save(c *BatchCloseout) error {
	  if b == nil {
		  return ErrUnavailable
	   }
	   if _, dup := b.byBatch[c.WDBatchID]; dup {
		  return ErrCloseoutExists
	   }
	  cp := *c
	   b.byBatch[c.WDBatchID] = &cp
	   return nil
   }

   func (b *MemCloseoutBook) GetByBatch(wdbatchID string) (*BatchCloseout, error) {
	  if b == nil {
		   return nil, ErrUnavailable
	  }
	   c, ok := b.byBatch[wdbatchID]
	   if !ok {
		   return nil, ErrNotFound
	   }
	  cp := *c
	   return &cp, nil
   }

// ProveConserveCase 守恒案件证明固化（open→proven）。
   func (s *Service) ProveConserveCase(req ProveConserveCaseReq) (*ConserveCase, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	   if strings.TrimSpace(req.CaseID) == "" {
		   return nil, ErrConserveCaseNotFound
	  }
	   if strings.TrimSpace(req.ProofPayload) == "" {
		   return nil, ErrConserveCaseProofRequired
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  c, ok := s.remConserves[req.CaseID]
	  if !ok {
		   return nil, ErrConserveCaseNotFound
	   }
	   if c.Status != ConserveCaseOpen {
		   return nil, fmt.Errorf("%w: case %s is %s, expected open", ErrConserveCaseNotFound, c.ID, c.Status)
	  }
	   now := s.now()
	  c.Status = ConserveCaseProven
	   c.ProofPayload = req.ProofPayload
	   c.Approver = req.Approver
	   c.Approver2 = req.Approver2
	   t := now
	  c.ProvenAt = &t
	   c.UpdatedAt = now
	  cp := *c
	   return &cp, nil
  }

// CloseConserveCase 守恒案件收口（proven→closed）。

   func (s *Service) CloseConserveCase(req CloseConserveCaseReq) (*ConserveCase, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   c, ok := s.remConserves[req.CaseID]
	   if !ok {
		  return nil, ErrConserveCaseNotFound
	  }
	   if c.Status == ConserveCaseClosed {
		   return nil, ErrConserveCaseAlreadyClosed
	   }
	  if c.Status != ConserveCaseProven {
		   return nil, fmt.Errorf("%w: case %s is %s", ErrConserveCaseNotProven, c.ID, c.Status)
	  }
	   now := s.now()
	   c.Status = ConserveCaseClosed
	  if req.Approver != "" {
		   c.Approver = req.Approver
	   }
	  t := now
	   c.ClosedAt = &t
	   c.UpdatedAt = now
	  cp := *c
	  return &cp, nil
  }

// ForceCloseConserveCase 双人确认强制收口（open/proven→closed）。
// 须提供 proof_payload；Approver 与 Approver2 必须不同。

   func (s *Service) ForceCloseConserveCase(planID, caseID, proof, approver, approver2 string) (*ConserveCase, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   if strings.TrimSpace(proof) == "" {
		   return nil, ErrConserveCaseProofRequired
	  }
	  if strings.TrimSpace(approver) == "" || strings.TrimSpace(approver2) == "" {
		  return nil,
			   ErrForceReleaseDenied
	   }
	  if approver == approver2 {
		  return nil, fmt.Errorf("%w: approver and approver2 must be different", ErrForceReleaseDenied)
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   var c *ConserveCase
	   if caseID != "" {
		  c = s.remConserves[caseID]
	   }
	   if c == nil {
		  // 按 planID 找 open case。
		   for _, cc := range s.remConserves {
			  if cc.PlanID == planID && !cc.Status.IsTerminal() {
				   c = cc
				  break
			  }
		  }
	  }
	  if c == nil {
		   return nil, ErrConserveCaseNotFound
	   }
	  if c.Status == ConserveCaseClosed {
		   return nil, ErrConserveCaseAlreadyClosed
	  }
	   now := s.now()
	  c.Status = ConserveCaseClosed
	   c.ProofPayload = proof
	  c.Approver = approver
	  c.Approver2 = approver2
	  t := now
	  c.ClosedAt = &t
	   c.UpdatedAt = now
	  cp := *c
	   return &cp, nil
   }

// GetConserveCase 查询守恒案件（只读）。

   func (s *Service) GetConserveCase(id string) (*ConserveCase, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  c, ok := s.remConserves[id]
	   if !ok {
		   return nil,
			   ErrConserveCaseNotFound
	  }
	  cp := *c
	  return &cp, nil
  }

// ListOpenConserveCases 查询 open 守恒案件列表（Worker 巡检用）。
   func (s *Service) ListOpenConserveCases(limit int) []*ConserveCase {
	  if s == nil {
		   return nil
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  var out []*ConserveCase
	   for _, c := range s.remConserves {
		   if c.Status == ConserveCaseOpen {
			  cp := *c
			  out = append(out, &cp)
			  if limit > 0 && len(out) >= limit {
				   break
			   }
		  }
	  }
	  return out
   }

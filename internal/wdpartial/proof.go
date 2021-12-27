package wdpartial

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// ProofBook 证明存储接口。
type ProofBook interface {
	// ExistsByResidue 检查指定 residue_id 是否已有证明。
	ExistsByResidue(residueID string) bool
	// Save 追加一条证明记录（幂等：同 ID 不重复写）。
	Save(proof *ReleaseProof) error
	// ListByBatch 返回指定批次所有证明。
	ListByBatch(wdbatchID string) ([]*ReleaseProof, error)
	// Get 按 ID 获取证明。
	Get(id string) (*ReleaseProof, error)
}

// ProofManager Hold Release Proof 管理器。
type ProofManager struct {
	proofs ProofBook
	book   ResidueBook
	mutex *MutexManager
	clock func() time.Time
}

// NewProofManager 构造 ProofManager。
  func NewProofManager(proofs ProofBook, book ResidueBook, mutex *MutexManager) *ProofManager {
	   return &ProofManager{
		  proofs: proofs,
		   book: book,
		  mutex: mutex,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

// WithClock 注入时钟。
  func (p *ProofManager) WithClock(c func() time.Time) *ProofManager {
	   if p != nil && c != nil {
		  p.clock = c
	   }
	   return p
   }

   // RecordProofReq 记录证明请求参数。
   type RecordProofReq struct {
	   WDBatchID string
	  ResidueID string
	  Kind ReleaseProofKind
	  Actor string
	   LedgerRef string // 账本操作参考 ID（可选）
	   Summary   string
	   LeaseHolder string
	  LeaseGen    int64
  }

  // RecordProof 为指定残留项记录释放/升级证明。
   // 调用前须已通过 MutexManager.Verify 校验持有有效 ResumeLease。
  // 同一 ResidueItem 重复调用返回 ErrProofDuplicate。
  func (p *ProofManager) RecordProof(req RecordProofReq) (*ReleaseProof, error) {
	   if p == nil {
		   return nil, ErrUnavailable
	   }
	   req.WDBatchID = strings.TrimSpace(req.WDBatchID)
	   req.ResidueID = strings.TrimSpace(req.ResidueID)
	  req.Actor = strings.TrimSpace(req.Actor)
	  if req.WDBatchID == "" {
		  return nil, ErrBatchID
	   }
	  if req.ResidueID == "" {
		   return nil, ErrResidueID
	  }
	   if req.Actor == "" {
		   return nil, ErrActor
	   }
	   if req.Kind == "" {
		  return nil, ErrUnavailable
	   }
	   // 校验 ResumeLease 世代
	  if p.mutex != nil {
		   if err := p.mutex.Verify(req.WDBatchID, req.LeaseHolder, req.LeaseGen); err != nil {
			  return nil, err
		  }
	   }
	   // 幂等检查
	   if p.proofs.ExistsByResidue(req.ResidueID) {
		   return nil, ErrProofDuplicate
	   }
	   // 获取残留项，验证状态
	  item, err := p.book.Get(req.ResidueID)
	  if err != nil {
		   return nil, err
	  }
	  if item.WDBatchID != req.WDBatchID {
		   return nil, ErrBatchID
	   }
	  if item.Terminal() {
		   return nil, ErrTerminal
	  }
	   if !item.Open() {
		   return nil, ErrNotOpen
	   }
	  now := p.clock()
	   proof := &ReleaseProof{
		   ID: uuid.NewString(),
		  WDBatchID: req.WDBatchID,
		  ResidueID: req.ResidueID,
		   UserID: item.UserID,
		   Asset:      item.Asset,
		   Amount:     item.Amount,
		  Kind:       req.Kind,
		  Actor:      req.Actor,
		   LedgerRef:  req.LedgerRef,
		  Summary:    req.Summary,
		  LeaseGen:   req.LeaseGen,
		   RecordedAt: now,
	   }
	   proof.Checksum = computeProofChecksum(proof)
	   if err := p.proofs.Save(proof); err != nil {
		   return nil, err
	  }
	   // 更新残留项状态
	  switch req.Kind {
	   case ProofKindFreezeRelease, ProofKindLedgerCancel, ProofKindNoOccupy:
		   item.Status = ResidueReleased
	   case ProofKindEscalateSusp:
		  item.Status = ResidueEscalated
		  item.EscalateLedgerID = req.LedgerRef
	  }
	  item.ProofID = proof.ID
	   item.ResolvedAt = now
	   item.UpdatedAt = now
	   if err := p.book.Update(item); err != nil {
		   return nil, err
	   }
	  return proof, nil
   }

// MarkRetried 将残留项标记为 retried（新批次中重提成功）。
// 调用者须持有有效 ResumeLease 且通过 Verify。
   func (p *ProofManager) MarkRetried(residueID, retryBatchID, leaseHolder string, leaseGen int64) error {
	  if p == nil {
		  return ErrUnavailable
	   }
	   item,
		  err := p.book.Get(strings.TrimSpace(residueID))
	  if err != nil {
		  return err
	  }
	  if item.Terminal() {
		   return ErrTerminal
	   }
	   if p.mutex != nil {
		   if err := p.mutex.Verify(item.WDBatchID, leaseHolder, leaseGen); err != nil {
			  return err
		  }
	  }
	   now := p.clock()
	  item.Status = ResidueRetried
	   item.RetryBatchID = strings.TrimSpace(retryBatchID)
	  item.ResolvedAt = now
	   item.UpdatedAt = now
	  return p.book.Update(item)
   }

// AssertAllProved 校验指定批次所有 open/terminal 残留项是否均有证明。
// 若任一 open 项缺少证明，返回 ErrProofMissing。
// 用于 BatchCloseout 前的必要性校验。

   func (p *ProofManager) AssertAllProved(wdbatchID string) error {
	  if p == nil {
		   return ErrUnavailable
	   }
	  items, err := p.book.ListByBatch(strings.TrimSpace(wdbatchID))
	  if err != nil {
		  return err
	   }
	  for _, it := range items {
		  if !it.Terminal() {
			   // 存在非终态的残留项
			  if !p.proofs.ExistsByResidue(it.ID) {
				  return fmt.Errorf("%w: residue_id=%s (batch=%s, seq=%d)",
					   ErrProofMissing, it.ID, wdbatchID, it.Seq)
			   }
		   }
	   }
	  return nil
  }

// VerifyChecksum 验证证明 Checksum 完整性。

   func VerifyChecksum(proof *ReleaseProof) error {
	   if proof == nil {
		  return ErrProofStore
	   }
	  want := computeProofChecksum(proof)
	   if proof.Checksum != want {
		   return fmt.Errorf("%w: id=%s want=%s got=%s",
			   ErrProofChecksum, proof.ID, want, proof.Checksum)
	   }
	   return nil
  }

// computeProofChecksum 计算证明校验和。
// SHA-256(kind|wd_batch_id|residue_id|user_id|asset|amount|actor)

  func computeProofChecksum(p *ReleaseProof) string {
	   raw := strings.Join([]string{
		  string(p.Kind),
		  p.WDBatchID,
		   p.ResidueID,
		   p.UserID,
		   p.Asset,
		  p.Amount,
		   p.Actor,
	   }, "|")
	  h := sha256.Sum256([]byte(raw))
	  return fmt.Sprintf("%x", h)
  }

// ─── 内存 ProofBook 实现 ──────────────────────────────────────────────────────
// MemProofBook 内存 ProofBook，仅供使用。
type MemProofBook struct {
	byID map[string]*ReleaseProof
	byResidue map[string]string   // residueID → proofID
	byBatch   map[string][]string // wdbatchID → []proofID
}

// NewMemProofBook 构造内存 ProofBook。

   func NewMemProofBook() *MemProofBook {
	   return &MemProofBook{
		  byID:      make(map[string]*ReleaseProof),
		  byResidue: make(map[string]string),
		  byBatch:   make(map[string][]string),
	  }
   }

  func (b *MemProofBook) ExistsByResidue(residueID string) bool {
	   if b == nil {
		   return false
	  }
	   _, ok := b.byResidue[residueID]
	   return ok
   }

  func (b *MemProofBook) Save(proof *ReleaseProof) error {
	   if b == nil {
		   return ErrProofStore
	   }
	   if _, dup := b.byID[proof.ID]; dup {
		   return nil // 幂等
	   }
	   cp := *proof
	  b.byID[proof.ID] = &cp
	  b.byResidue[proof.ResidueID] = proof.ID
	   b.byBatch[proof.WDBatchID] = append(b.byBatch[proof.WDBatchID], proof.ID)
	   return nil
   }

  func (b *MemProofBook) ListByBatch(wdbatchID string) ([]*ReleaseProof, error) {
	  if b == nil {
		   return nil, ErrProofStore
	  }
	  ids := b.byBatch[wdbatchID]
	   out := make([]*ReleaseProof, 0, len(ids))
	  for _, id := range ids {
		   if p, ok := b.byID[id]; ok {
			   cp := *p
			   out = append(out, &cp)
		   }
	   }
	   return out, nil
  }

   func (b *MemProofBook) Get(id string) (*ReleaseProof, error) {
	   if b == nil {
		  return nil, ErrProofStore
	  }
	  p, ok := b.byID[id]
	  if !ok {
		  return nil, ErrNotFound
	  }
	   cp := *p
	  return &cp, nil
  }

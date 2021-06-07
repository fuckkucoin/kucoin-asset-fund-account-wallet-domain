package fiatbreak

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// ProofInput 生成双边证明所需的原始数据。
type ProofInput struct {
	BookingID string
	BreakID   string
	PSPReceiptID string
	PSPAmount string
	BookAmount string
	LedgerID   string
	Currency   string
}

// ProofService 双边证明服务。
// 证明追加只写；生成后 ProofHash 不可覆盖。
type ProofService struct {
	mu sync.RWMutex
	// proofs 按 ID 存储证明。
	proofs map[string]*BilateralProof
	// byBooking 按 booking_id 索引证明 ID 列表（一个预约可有多次证明）。
	byBooking map[string][]string
	// byBreak 按 break_id 索引证明 ID 列表。
	byBreak map[string][]string
	clock func() time.Time
}

// NewProofService 创建证明服务实例。

  func NewProofService() *ProofService {
	   return &ProofService{
		  proofs: map[string]*BilateralProof{},
		   byBooking: map[string][]string{},
		   byBreak: map[string][]string{},
		   clock:   func() time.Time { return time.Now().UTC() },
	   }
   }

// now 可注入时钟。
   func (p *ProofService) now() time.Time {
	   if p == nil || p.clock == nil {
		   return time.Now().UTC()
	   }
	  return p.clock()
   }

// Generate 生成双边证明。
// 计算轨道侧摘要（覆盖预约+PSP收据+金额）和账本侧摘要（覆盖账本凭证），
// 再生成联合哈希。追加只写，不修改任何现有证明。
  func (p *ProofService) Generate(_ context.Context, in ProofInput) (BilateralProof, error) {
	  if p == nil {
		  return BilateralProof{},
			  ErrUnavailable
	   }
	   bookingID := strings.TrimSpace(in.BookingID)
	  if bookingID == "" {
		  return BilateralProof{}, fmt.Errorf("%w: booking_id required", ErrProof)
	   }
	   now := p.now()
	  // 轨道侧摘要：booking_id + psp_receipt_id + psp_amount + book_amount + currency
	  railRaw := strings.Join([]string{
		   bookingID,
		  strings.TrimSpace(in.PSPReceiptID),
		   strings.TrimSpace(in.PSPAmount),
		   strings.TrimSpace(in.BookAmount),
		   strings.ToUpper(strings.TrimSpace(in.Currency)),
	   }, "|")
	   railDigest := sha256Hex(railRaw)
	   // 账本侧摘要：ledger_id + booking_id + book_amount（无账本时退化为预约侧冗余）
	  ledgerRaw := strings.Join([]string{
		  strings.TrimSpace(in.LedgerID),
		  bookingID,
		   strings.TrimSpace(in.BookAmount),
	  }, "|")
	   ledgerDigest := sha256Hex(ledgerRaw)
	   // 联合哈希：rail_digest || ledger_digest || timestamp（纳秒）
	   proofRaw := fmt.Sprintf("%s|%s|%d", railDigest, ledgerDigest, now.UnixNano())
	  proofHash := sha256Hex(proofRaw)
	  proof := &BilateralProof{
		   ID:           uuid.NewString(),
		   BookingID:    bookingID,
		   BreakID:      strings.TrimSpace(in.BreakID),
		   RailDigest:   railDigest,
		   LedgerDigest: ledgerDigest,
		  ProofHash: proofHash,
		  Sealed:    false,
		  CreatedAt: now,
	   }
	  p.mu.Lock()
	   p.proofs[proof.ID] = proof
	  p.byBooking[bookingID] = append(p.byBooking[bookingID], proof.ID)
	   if proof.BreakID != "" {
		   p.byBreak[proof.BreakID] = append(p.byBreak[proof.BreakID], proof.ID)
	  }
	  p.mu.Unlock()
	   return proof.copy(), nil
   }

// Seal 封印证明（封印后不得追加新摘要到此证明）。
   func (p *ProofService) Seal(id, sealedBy string) (BilateralProof, error) {
	   if p == nil {
		   return BilateralProof{}, ErrUnavailable
	  }
	  id = strings.TrimSpace(id)
	  sealedBy = strings.TrimSpace(sealedBy)
	   if sealedBy == "" {
		  return BilateralProof{}, fmt.Errorf("%w: sealed_by required", ErrProof)
	   }
	  p.mu.Lock()
	   defer p.mu.Unlock()
	  proof, ok := p.proofs[id]
	  if !ok {
		  return BilateralProof{}, ErrNotFound
	  }
	  if proof.Sealed {
		   return proof.copy(), nil
	   }
	   now := p.now()
	  proof.Sealed = true
	   proof.SealedBy = sealedBy
	   proof.SealedAt = &now
	  return proof.copy(), nil
  }

// Get 按 ID 获取证明。
  func (p *ProofService) Get(id string) (BilateralProof, error) {
	  if p == nil {
		  return BilateralProof{}, ErrUnavailable
	  }
	   p.mu.RLock()
	   proof, ok := p.proofs[strings.TrimSpace(id)]
	  p.mu.RUnlock()
	   if !ok {
		   return BilateralProof{}, ErrNotFound
	  }
	   return proof.copy(), nil
   }

// Query 按条件查询证明列表。
   func (p *ProofService) Query(req ProofQueryReq) []BilateralProof {
	   if p == nil {
		   return nil
	   }
	   limit := req.Limit
	  if limit <= 0 {
		  limit = 50
	  }
	  p.mu.RLock()
	   defer p.mu.RUnlock()
	   if req.BookingID != "" {
		   ids := p.byBooking[strings.TrimSpace(req.BookingID)]
		   out := make([]BilateralProof, 0, len(ids))
		  for _, id := range ids {
			  if pf, ok := p.proofs[id]; ok {
				   out = append(out, pf.copy())
			  }
			   if len(out) >= limit {
				   break
			  }
		  }
		  return out
	   }
	  if req.BreakID != "" {
		   ids := p.byBreak[strings.TrimSpace(req.BreakID)]
		   out := make([]BilateralProof, 0, len(ids))
		   for _, id := range ids {
			  if pf, ok := p.proofs[id]; ok {
				  out = append(out, pf.copy())
			   }
			   if len(out) >= limit {
				   break
			   }
		  }
		   return out
	  }
	  // 全量（分页）
	   out := make([]BilateralProof, 0, limit)
	  for _, pf := range p.proofs {
		   out = append(out, pf.copy())
		   if len(out) >= limit {
			  break
		  }
	  }
	  return out
   }

// Verify 校验证明完整性（重算哈希核对）。
// 返回 true 表示证明哈希与原始摘要一致。

  func (p *ProofService) Verify(id string) (bool, error) {
	   if p == nil {
		   return false, ErrUnavailable
	   }
	  proof, err := p.Get(id)
	  if err != nil {
		   return false, err
	  }
	  proofRaw := fmt.Sprintf("%s|%s|%d", proof.RailDigest, proof.LedgerDigest, proof.CreatedAt.UnixNano())
	  expected := sha256Hex(proofRaw)
	  return expected == proof.ProofHash,
		   nil
   }

// copy 深拷贝（BilateralProof 含指针字段 SealedAt）。

  func (pf *BilateralProof) copy() BilateralProof {
	   if pf == nil {
		   return BilateralProof{}
	  }
	   cp := *pf
	   if pf.SealedAt != nil {
		   t := *pf.SealedAt
		   cp.SealedAt = &t
	   }
	   return cp
   }

// sha256Hex 计算 SHA-256 并返回小写十六进制字符串。
   func sha256Hex(s string) string {
	  h := sha256.Sum256([]byte(s))
	   return hex.EncodeToString(h[:])
   }

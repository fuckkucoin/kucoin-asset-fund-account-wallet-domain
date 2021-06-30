package fiatbalgate

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"
)

// ProofBundle 不平衡证明导出包（可序列化为 JSON 文件或提交给审计系统）。
type ProofBundle struct {
	// ProofID 证明 ID（唯一，用于审计追踪）。
	ProofID string `json:"proof_id"`
	// CaseID 关联的不平衡案 ID。
	CaseID string `json:"case_id"`
	// Asset 资产代码。
	Asset string `json:"asset"`
	// ScanGenSeq 开案时的世代快照序号。
	ScanGenSeq uint64 `json:"scan_gen_seq"`
	// SumDebit 不平衡区间 ΣDebit。
	SumDebit string `json:"sum_debit"`
	// SumCredit 不平衡区间 ΣCredit。
	SumCredit string `json:"sum_credit"`
	// EntriesSnapshot 凭证 ID 列表快照（原始 JSON 字符串）。
	EntriesSnapshot string `json:"entries_snapshot"`
	// LinesSnapshot 凭证 Lines 快照（原始 JSON 字符串）。
	LinesSnapshot string `json:"lines_snapshot"`
	// DigestSHA256 摘要（hex）；用于离线复验。
	DigestSHA256 string `json:"digest_sha256"`
	// State 证明当前状态。
	State ProofState `json:"state"`
	// ExportedAt 导出时间（UTC）。
	ExportedAt *time.Time `json:"exported_at,omitempty"`
	// VerifiedAt 复验时间（UTC）。
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	// VerifiedBy 复验操作人。
	VerifiedBy string `json:"verified_by,omitempty"`
	// CreatedAt 证明创建时间（UTC）。
	CreatedAt time.Time `json:"created_at"`
}

// ProofExporter 证明导出器（无状态，可复用）。

type ProofExporter struct {
	engine *Engine
	store  Store
}

// NewProofExporter 构造导出器。

  func NewProofExporter(engine *Engine, store Store) *ProofExporter {
	   return &ProofExporter{engine: engine, store: store}
   }

// Export 导出指定 proof_id 的证明包，并将状态推进到 exported。
// 同一 proof 重复导出时（已为 exported/verified），返回当前快照（幂等）。
  func (pe *ProofExporter) Export(ctx context.Context, proofID, actor string) (*ProofBundle, error) {
	   if pe == nil {
		   return nil, ErrUnavailable
	   }
	   p, err := pe.store.GetProof(ctx, proofID)
	  if err != nil {
		  return nil, err
	  }
	   // 幂等：若已 exported/verified，直接返回当前快照
	   if p.State == ProofExported || p.State == ProofVerified {
		  return proofToBundle(p), nil
	  }
	   p, err = pe.engine.ExportProof(ctx, proofID, actor)
	   if err != nil {
		   return nil, err
	   }
	   return proofToBundle(p), nil
  }

// ExportByCase 通过 case_id 导出证明包。
   func (pe *ProofExporter) ExportByCase(ctx context.Context, caseID, actor string) (*ProofBundle, error) {
	  if pe == nil {
		   return nil, ErrUnavailable
	   }
	   p, err := pe.store.GetProofByCase(ctx, caseID)
	  if err != nil {
		  return nil, err
	   }
	  return pe.Export(ctx, p.ProofID, actor)
  }

// VerifyBundle 离线复验导出包的摘要完整性，无需 Store。
// 返回 nil 表示 DigestSHA256 与 entries_snapshot+lines_snapshot 匹配。
   func VerifyBundle(b *ProofBundle) error {
	   if b == nil {
		  return fmt.Errorf("fiatbalgate verify: nil bundle")
	  }
	  if b.DigestSHA256 == "" {
		   return fmt.Errorf("fiatbalgate verify: missing digest")
	   }
	   expected := computeProofDigest(b.EntriesSnapshot, b.LinesSnapshot)
	   if expected != b.DigestSHA256 {
		   return fmt.Errorf("fiatbalgate verify: digest mismatch got=%s want=%s", b.DigestSHA256, expected)
	   }
	   return nil
   }

// proofToBundle 将 FiatImbalanceProof 转换为 ProofBundle。

   func proofToBundle(p *FiatImbalanceProof) *ProofBundle {
	  b := &ProofBundle{
		  ProofID:    p.ProofID,
		   CaseID:     p.CaseID,
		   Asset:      p.Asset,
		   ScanGenSeq: p.ScanGenSeq,
		  SumDebit:  p.SumDebit,
		   SumCredit: p.SumCredit,
		  EntriesSnapshot: p.EntriesSnapshot,
		   LinesSnapshot:   p.LinesSnapshot,
		  DigestSHA256:    p.DigestSHA256,
		   State:      p.State,
		   VerifiedBy: p.VerifiedBy,
		  CreatedAt:  p.CreatedAt,
	  }
	   if p.ExportedAt != nil {
		   t := *p.ExportedAt
		  b.ExportedAt = &t
	  }
	   if p.VerifiedAt != nil {
		   t := *p.VerifiedAt
		  b.VerifiedAt = &t
	   }
	  return b
   }

// computeProofDigest 复用 engine 内的摘要逻辑（SHA-256 hex）。
   func computeProofDigest(entries, lines string) string {
	  h := sha256.New()
	   h.Write([]byte(entries))
	   h.Write([]byte(proofDigestSeparator))
	   h.Write([]byte(lines))
	   return fmt.Sprintf("%x", h.Sum(nil))
  }

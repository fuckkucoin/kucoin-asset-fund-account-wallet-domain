package viptiergate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
	"github.com/google/uuid"
)

// DriftProofVault 管理 DriftProof 的构建与验证。

type DriftProofVault struct {
	store Store
	clock func() time.Time
}

  func newDriftProofVault(store Store, clock func() time.Time) *DriftProofVault {
	   return &DriftProofVault{store: store, clock: clock}
  }

   // Build 为指定 DriftCase 构建漂移证明。
   // 若 DriftCase 已有关联证明（ProofID 非空），直接返回已有证明（幂等）。
  // 若 DriftCase 状态不是 open，返回 ErrLeaseStale。
  func (v *DriftProofVault) Build(ctx context.Context, caseID string) (*DriftProof, error) {
	   dc,
		   err := v.store.GetDriftCase(ctx, caseID)
	   if err != nil {
		   return nil, ErrDriftCaseNotFound
	   }
	   // 幂等：已有证明直接返回。
	  if dc.ProofID != "" {
		   return v.store.GetDriftProof(ctx, dc.ProofID)
	  }
	  if dc.Status != DriftOpen {
		  return nil, ErrLeaseStale
	  }
	   now := v.clock()
	  p := &DriftProof{
		  ID:     uuid.NewString(),
		   CaseID: caseID,
		  UserID: dc.UserID,
		  Asset:       dc.Asset,
		   ObservedBPS: dc.ObservedBPS,
		   ExpectedBPS: dc.ExpectedBPS,
		   Amount:      dc.Amount,
		  QuoteBindID: dc.QuoteBindID,
		  LeaseID:     dc.LeaseID,
		   Generation: dc.Generation,
		  Status:     DriftProofPending,
		  CreatedAt: now,
	  }
	  p.Digest = computeProofDigest(p)
	  if err := v.store.SaveDriftProof(ctx, p); err != nil {
		   return nil, err
	   }
	  return p, nil
  }

// Verify 重计算摘要并；通过则推进状态至 verified，同时触发 DriftDetector.AttachProof。

   func (v *DriftProofVault) Verify(ctx context.Context, proofID string) (*DriftProof, error) {
	  p, err := v.store.GetDriftProof(ctx, proofID)
	  if err != nil {
		  return nil, ErrProofNotFound
	   }
	  if p.Status == DriftProofVerified {
		   return p, nil
	   }
	  // 重计算摘要。
	  computed := computeProofDigest(p)
	   if computed != p.Digest {
		  p.Status = DriftProofFailed
		  _ = v.store.SaveDriftProof(ctx,
			  p)
		  return p, ErrProofDigestMismatch
	   }
	  // 业务验证：ObservedBPS 与 ExpectedBPS 必须不等（漂移存在）。
	   if p.ObservedBPS == p.ExpectedBPS {
		   p.Status = DriftProofFailed
		  _ = v.store.SaveDriftProof(ctx, p)
		   return p, ErrProofMissing
	   }
	  now := v.clock()
	   p.Status = DriftProofVerified
	  p.VerifiedAt = &now
	   if err := v.store.SaveDriftProof(ctx, p); err != nil {
		   return nil, err
	   }
	  return p, nil
  }

// Export 导出指定证明的完整记录，并执行在线验签。
// 用于核验存档或 ops 审查。

   func (v *DriftProofVault) Export(ctx context.Context, proofID string) (*DriftProof, error) {
	   p, err := v.store.GetDriftProof(ctx, proofID)
	  if err != nil {
		   return nil, ErrProofNotFound
	  }
	  // 在线验签。
	   computed := computeProofDigest(p)
	  if computed != p.Digest {
		   return p, ErrProofDigestMismatch
	   }
	  return p, nil
   }

// GetByID 按 proof_id 查询。
   func (v *DriftProofVault) GetByID(ctx context.Context, proofID string) (*DriftProof, error) {
	  return v.store.GetDriftProof(ctx, proofID)
   }

// computeProofDigest 计算 DriftProof 的 SHA-256 摘要。
// 输入形态（NUL 分隔）：
// case_id \x00 user_id \x00 asset \x00 observed_bps \x00 expected_bps
// \x00 amount \x00 quote_bind_id \x00 lease_id \x00 generation
   func computeProofDigest(p *DriftProof) string {
	  raw := fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%d\x00%s\x00%s\x00%s\x00%d",
		  p.CaseID, p.UserID, p.Asset,
		  p.ObservedBPS, p.ExpectedBPS,
		  p.Amount, p.QuoteBindID, p.LeaseID,
		  p.Generation,
	  )
	   h := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(h[:])
   }

// VerifyDigestOnly 仅验签摘要（不修改状态），用于外部审计工具。
   func VerifyDigestOnly(p *DriftProof) bool {
	   if p == nil {
		   return false
	  }
	  return computeProofDigest(p) == p.Digest
  }

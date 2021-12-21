package utamodegate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
	"github.com/google/uuid"
)

// DriftProofVault 管理 ModeDriftProof 的构建、验证与导出。

type DriftProofVault struct {
	store Store
	clock func() time.Time
}

   func newDriftProofVault(store Store, clock func() time.Time) *DriftProofVault {
	   return &DriftProofVault{store: store, clock: clock}
   }

   // Build 构建并持久化漂移证明。
   // 流程：
   // 1. 验证 DriftCase 存在且处于 open 状态。
   // 2. 计算 DeltaDigest = sha256(OldKeySnapshot + "|" + NewAggSnapshot + "|" + CaseID)。
   // 3. 持久化 ModeDriftProof。
  // DriftDetector.AttachProof 由 Service 层在 Build 之后调用，将案件推进至 proven。

  func (v *DriftProofVault) Build(ctx context.Context, req BuildProofReq) (*ModeDriftProof, error) {
	   caseID := strings.TrimSpace(req.DriftCaseID)
	  if caseID == "" {
		   return nil, ErrDriftCaseNotFound
	   }
	  if strings.TrimSpace(req.BuiltBy) == "" {
		   return nil, ErrOperatorInvalid
	  }
	   // 防止同一案件重复构建证明（幂等）。
	  existing,
		  err := v.store.GetProofByCaseID(ctx, caseID)
	  if err == nil && existing != nil {
		  // 已有证明，验证摘要后直接返回
		  if verr := v.Verify(existing); verr != nil {
			   return nil, verr
		   }
		   return existing, nil
	  }
	   digest := computeDigest(req.OldKeySnapshot, req.NewAggSnapshot, caseID)
	   now := v.clock()
	  proof := &ModeDriftProof{
		  ID:             uuid.NewString(),
		   DriftCaseID:    caseID,
		  OldKeySnapshot: req.OldKeySnapshot,
		  NewAggSnapshot: req.NewAggSnapshot,
		  DeltaDigest: digest,
		  BuiltBy:     strings.TrimSpace(req.BuiltBy),
		   VerifiedAt:  now,
		   CreatedAt: now,
	   }
	   if err := v.store.SaveProof(ctx, proof); err != nil {
		   return nil, err
	  }
	  return proof, nil
   }

// Verify 验证证明摘要完整性。
// 重计算 sha256 与 DeltaDigest ；不一致返回 ErrProofDigestMismatch。

  func (v *DriftProofVault) Verify(proof *ModeDriftProof) error {
	  if proof == nil {
		  return ErrProofNotFound
	  }
	  expected := computeDigest(proof.OldKeySnapshot,
		   proof.NewAggSnapshot, proof.DriftCaseID)
	   if expected != proof.DeltaDigest {
		  return ErrProofDigestMismatch
	  }
	  return nil
   }

// BuildAndVerify 构建证明并立即验证（原子化便捷方法）。
  func (v *DriftProofVault) BuildAndVerify(ctx context.Context, req BuildProofReq) (*ModeDriftProof, error) {
	  proof, err := v.Build(ctx, req)
	   if err != nil {
		  return nil, err
	  }
	  if err := v.Verify(proof); err != nil {
		  return nil, err
	   }
	  return proof, nil
   }

// GetProof 按 proof_id 查询证明。
  func (v *DriftProofVault) GetProof(ctx context.Context, proofID string) (*ModeDriftProof, error) {
	  return v.store.GetProof(ctx, proofID)
   }

// GetProofByCaseID 按 drift_case_id 查询证明。

   func (v *DriftProofVault) GetProofByCaseID(ctx context.Context, caseID string) (*ModeDriftProof, error) {
	  return v.store.GetProofByCaseID(ctx,
		  caseID)
  }

// Export 返回证明的可导出摘要（核验存档用）。
// 不包含内部快照原文（避免泄漏业务敏感数据）；仅返回摘要与元数据。

  func (v *DriftProofVault) Export(ctx context.Context, proofID string) (map[string]any, error) {
	   proof, err := v.store.GetProof(ctx, proofID)
	  if err != nil {
		  return nil, err
	  }
	   if err := v.Verify(proof); err != nil {
		  return nil, err
	   }
	  return map[string]any{
		  "proof_id":     proof.ID,
		  "case_id":      proof.DriftCaseID,
		   "delta_digest": proof.DeltaDigest,
		  "built_by":     proof.BuiltBy,
		  "verified_at": proof.VerifiedAt.Format(time.RFC3339Nano),
		   "created_at":  proof.CreatedAt.Format(time.RFC3339Nano),
		  // 注：OldKeySnapshot / NewAggSnapshot 不含在导出响应中
	   }, nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// 内部工具
// ─────────────────────────────────────────────────────────────────────────────
// computeDigest 计算漂移证明摘要。
// 形态：sha256(oldKey + "|" + newAgg + "|" + caseID)，十六进制小写。
   func computeDigest(oldKey, newAgg, caseID string) string {
	   h := sha256.New()
	  h.Write([]byte(oldKey))
	   h.Write([]byte("|"))
	  h.Write([]byte(newAgg))
	  h.Write([]byte("|"))
	   h.Write([]byte(caseID))
	   return hex.EncodeToString(h.Sum(nil))
   }

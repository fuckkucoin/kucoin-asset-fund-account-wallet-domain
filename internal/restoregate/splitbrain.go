package restoregate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

const requiredApprovals = 2 // 四眼：至少 2 名不同角色审批人

// SplitBrainGuard 脑裂证明管理器。
// 回灌后（verifying 阶段） balanceRootPost 与 plan.BalanceRootPre / TargetBalanceRoot：
// - 若不符 → 创建 SplitBrainProof，状态 open，阻断写路径
// - 四眼审批通过（2 名不同角色）→ 转 cleared，解除挡写
// - 运维强制 override（带 reason）→ 转 override

type SplitBrainGuard struct {
	store PlanStore
	clock func() time.Time
}

// NewSplitBrainGuard 构造脑裂管理器。
   func NewSplitBrainGuard(store PlanStore) *SplitBrainGuard {
	  return &SplitBrainGuard{
		  store: store,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟（用）。
  func (sb *SplitBrainGuard) WithClock(c func() time.Time) *SplitBrainGuard {
	   sb.clock = c
	  return sb
   }

  // CheckAndOpen 回灌后余额根校验。
  // targetRoot plan 声明的目标余额根（由调用方从 RestorePlan 读取）
   // postRoot 回灌后实测余额根
   // diffSummary 差异摘要（由调用方提供，如科目名称列表）
   // 若 postRoot == targetRoot → 返回 nil（无脑裂）
  // 若不符 → 创建 SplitBrainProof，返回 ErrChecksumMismatch

  func (sb *SplitBrainGuard) CheckAndOpen(
	   ctx context.Context,
	  planID, balRootPre, targetRoot, postRoot, diffSummary string,
   ) error {
	   if sb == nil || sb.store == nil {
		  return ErrUnavailable
	   }
	   planID = strings.TrimSpace(planID)
	  if planID == "" {
		  return ErrEmpty
	  }
	   if postRoot == targetRoot {
		  return nil // 余额根一致，无脑裂
	   }
	   now := sb.clock()
	   proof := SplitBrainProof{
		   ID: uuid.New().String(),
		   PlanID:          planID,
		   BalanceRootPre:  balRootPre,
		   BalanceRootPost: postRoot,
		  BalanceRootTarget: targetRoot,
		   DiffSummary: diffSummary,
		   State:       SplitBrainOpen,
		   OpenedAt:    now,
		   Approvals: []ProofApproval{},
		  CreatedAt: now,
		   UpdatedAt: now,
	   }
	  if err := sb.store.CreateSplitBrainProof(ctx, proof); err != nil {
		  return fmt.Errorf("restoregate: create split-brain proof: %w", err)
	  }
	   return ErrChecksumMismatch
   }

// CheckWriteBlocked 检查是否存在 open 状态的脑裂证明阻断写路径。
// 返回 nil → 写路径畅通；
// 返回 ErrSplitBrainOpen → 脑裂未解除，写被阻断。
   func (sb *SplitBrainGuard) CheckWriteBlocked(ctx context.Context, planID string) error {
	  if sb == nil || sb.store == nil {
		  return nil
	   }
	  _, err := sb.store.GetOpenProofByPlan(ctx, planID)
	  if errors.Is(err, ErrSplitBrainNotFound) {
		   return nil // 无 open 证明，放行
	  }
	  if err != nil {
		  return nil // 查询失败时放行（fail-open）
	   }
	   return ErrSplitBrainOpen
   }

// CheckAnyOpenProof 全局扫描是否存在任意 open/clearing 证明（用于写路径全局拦截）。

  func (sb *SplitBrainGuard) CheckAnyOpenProof(ctx context.Context) error {
	  if sb == nil || sb.store == nil {
		  return nil
	  }
	   proofs, err := sb.store.ListOpenProofs(ctx)
	  if err != nil {
		  return nil
	  }
	  if len(proofs) > 0 {
		  return fmt.Errorf("%w: proof_id=%s plan_id=%s", ErrSplitBrainOpen, proofs[0].ID, proofs[0].PlanID)
	   }
	   return nil
   }

// Approve 四眼审批：添加一条 ProofApproval，角色不可重复。
// 当累计审批人 ≥ requiredApprovals（且角色互不相同）时，自动转 cleared。

   func (sb *SplitBrainGuard) Approve(ctx context.Context, proofID, approver, role, comment string) error {
	  if sb == nil || sb.store == nil {
		   return ErrUnavailable
	   }
	   proofID = strings.TrimSpace(proofID)
	  approver = strings.TrimSpace(approver)
	   role = strings.TrimSpace(role)
	   if proofID == "" || approver == "" || role == "" {
		   return ErrEmpty
	   }
	   proof,
		  err := sb.store.GetSplitBrainProof(ctx, proofID)
	  if err != nil {
		  return err
	   }
	   if proof.State == SplitBrainCleared || proof.State == SplitBrainOverride {
		   return nil // 幂等
	   }
	  if proof.State != SplitBrainOpen && proof.State != SplitBrainClearing {
		  return fmt.Errorf("%w: state=%s", ErrInvalidState, proof.State)
	  }
	  // 角色唯一性检查
	   for _, a := range proof.Approvals {
		   if a.Role == role {
			  return fmt.Errorf("%w: role %q already approved", ErrProofConflict, role)
		   }
		  if a.Approver == approver {
			   return fmt.Errorf("%w: approver %q already approved", ErrProofConflict, approver)
		  }
	  }
	   now := sb.clock()
	   approval := ProofApproval{
		   ProofID: proofID,
		   Approver: approver,
		  Role:    role,
		   Comment: comment,
		   CreatedAt: now,
	   }
	  if err := sb.store.AppendProofApproval(ctx, approval); err != nil {
		   return err
	   }
	  // 重新读取（AppendProofApproval 已更新内存）
	   proof, err = sb.store.GetSplitBrainProof(ctx, proofID)
	   if err != nil {
		  return err
	   }
	   // 推进状态
	   if len(proof.Approvals) >= requiredApprovals {
		   proof.State = SplitBrainCleared
		  proof.ClearedAt = &now
		  proof.UpdatedAt = now
		   return sb.store.UpdateSplitBrainProof(ctx, proof)
	   }
	  if proof.State == SplitBrainOpen {
		   proof.State = SplitBrainClearing
		  proof.UpdatedAt = now
		   return sb.store.UpdateSplitBrainProof(ctx, proof)
	  }
	  return nil
   }

// Override 运维强制解除脑裂（带 reason，仍需至少一名审批人）。
  func (sb *SplitBrainGuard) Override(ctx context.Context, proofID, actor, reason string) error {
	  if sb == nil || sb.store == nil {
		   return ErrUnavailable
	   }
	   proofID = strings.TrimSpace(proofID)
	  actor = strings.TrimSpace(actor)
	   reason = strings.TrimSpace(reason)
	   if proofID == "" || actor == "" || reason == "" {
		  return ErrEmpty
	   }
	  proof, err := sb.store.GetSplitBrainProof(ctx, proofID)
	  if err != nil {
		  return err
	  }
	   if proof.State == SplitBrainCleared || proof.State == SplitBrainOverride {
		  return nil
	   }
	   if len(proof.Approvals) < 1 {
		  return fmt.Errorf("%w: at least one approval required before override", ErrProofConflict)
	  }
	   now := sb.clock()
	  proof.State = SplitBrainOverride
	  proof.ClearedAt = &now
	  proof.DiffSummary = proof.DiffSummary + " | override by " + actor + ": " + reason
	   proof.UpdatedAt = now
	  return sb.store.UpdateSplitBrainProof(ctx, proof)
  }

// ExportProof 导出证明（用于 ops API 与四眼审计页面）。
   func (sb *SplitBrainGuard) ExportProof(ctx context.Context, proofID string) (SplitBrainProof, error) {
	  if sb == nil || sb.store == nil {
		  return SplitBrainProof{}, ErrUnavailable
	   }
	  return sb.store.GetSplitBrainProof(ctx, proofID)
  }

// ListOpenProofs 列出所有 open/clearing 证明（用于脑裂巡检 worker）。
  func (sb *SplitBrainGuard) ListOpenProofs(ctx context.Context) ([]SplitBrainProof, error) {
	   if sb == nil || sb.store == nil {
		   return nil, ErrUnavailable
	  }
	  return sb.store.ListOpenProofs(ctx)
  }

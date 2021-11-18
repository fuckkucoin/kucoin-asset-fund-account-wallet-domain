// 功能：领域服务：业务用例编排与校验。
// 作者：Orin
// 日期：2021.10.21

package restoregate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"github.com/google/uuid"
)

// Service 灾备回灌世代闸核心服务。
// 协调 RestorePlan FSM、GenerationGate、WriteGuard、HalfSealManager、SplitBrainGuard。
// 部署时注入 SQLStore；单测注入 MemStore。
type Service struct {
	store PlanStore
	gate  *GenerationGate
	guard *WriteGuard
	seal *HalfSealManager
	sbGuard *SplitBrainGuard
	clock   func() time.Time
}

// NewService 构造 Service。

   func NewService(store PlanStore) *Service {
	   return &Service{
		  store: store,
		  gate: NewGenerationGate(store),
		   guard: NewWriteGuard(store),
		  seal:  NewHalfSealManager(store),
		  sbGuard: NewSplitBrainGuard(store),
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

// WithClock 注入时钟（用）。

  func (s *Service) WithClock(c func() time.Time) *Service {
	   s.clock = c
	   s.guard.WithClock(c)
	   s.seal.WithClock(c)
	  s.sbGuard.WithClock(c)
	   return s
   }

   // WriteGuard 暴露写互斥锁（供写路径 hook 调用）。
  func (s *Service) WriteGuard() *WriteGuard { return s.guard }

// SplitBrainGuard 暴露脑裂管理器（供 ops API 调用）。
  func (s *Service) SplitBrainGuard() *SplitBrainGuard { return s.sbGuard }

// HalfSealMgr 暴露半封印管理器。
   func (s *Service) HalfSealMgr() *HalfSealManager { return s.seal }

// CreatePlan 创建 RestorePlan，初始状态 planned。
// 若活账水位 liveWatermark > targetGen，拒绝创建（世代冲突）。

  func (s *Service) CreatePlan(ctx context.Context, req CreatePlanRequest) (RestorePlan, error) {
	   if s == nil || s.store == nil {
		   return RestorePlan{}, ErrUnavailable
	  }
	   req.BlobID = strings.TrimSpace(req.BlobID)
	   req.Creator = strings.TrimSpace(req.Creator)
	  if req.BlobID == "" || req.Creator == "" {
		   return RestorePlan{}, ErrEmpty
	  }
	   if req.TargetGen <= 0 {
		  return RestorePlan{}, fmt.Errorf("%w: target_gen must be positive", ErrEmpty)
	  }
	   if req.LiveWatermark > req.TargetGen {
		  return RestorePlan{},
			   ErrGenConflict
	  }
	  now := s.clock()
	  p := RestorePlan{
		  ID: uuid.New().String(),
		  BlobID:    req.BlobID,
		   SnapID:    strings.TrimSpace(req.SnapID),
		  TargetGen: req.TargetGen,
		   LiveWatermark:  req.LiveWatermark,
		   BalanceRootPre: strings.TrimSpace(req.BalanceRootPre),
		  State:          StatePlanned,
		   Creator: req.Creator,
		   Reason:  strings.TrimSpace(req.Reason),
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	   if err := s.store.CreatePlan(ctx, p); err != nil {
		   return RestorePlan{}, err
	   }
	   return p, nil
   }

// FreezePlan 推进 plan 到 freezing_writes，并获取写互斥锁。

   func (s *Service) FreezePlan(ctx context.Context, planID string) (RestorePlan, error) {
	  p, err := s.store.GetPlan(ctx, planID)
	   if err != nil {
		   return RestorePlan{}, err
	   }
	  if err := AssertTransition(p.State, StateFreezing); err != nil {
		  return RestorePlan{}, err
	  }
	   // 申请写互斥锁
	  if _, err := s.guard.Acquire(ctx, planID, p.TargetGen); err != nil {
		   return RestorePlan{}, fmt.Errorf("restoregate: acquire write mutex: %w", err)
	   }
	  now := s.clock()
	   p.State = StateFreezing
	   p.FrozenAt = &now
	   p.UpdatedAt = now
	  if err := s.store.UpdatePlan(ctx, p); err != nil {
		  return RestorePlan{}, err
	  }
	   return p, nil
   }

// StartImport 推进 plan 到 importing，并执行世代闸校验。
// 世代冲突时 plan 自动中止（fail-closed）。
  func (s *Service) StartImport(ctx context.Context, planID string, liveWatermark int64) (RestorePlan, error) {
	   p, err := s.store.GetPlan(ctx, planID)
	  if err != nil {
		  return RestorePlan{}, err
	  }
	   if err := AssertTransition(p.State, StateImporting); err != nil {
		  return RestorePlan{}, err
	  }
	   // 检查封印（重复 Import 同计划）
	  if err := s.seal.CheckResumable(ctx, planID); err != nil {
		   return RestorePlan{}, err
	   }
	  // 世代闸校验
	  if err := s.gate.CheckAndRecord(ctx, planID, p.TargetGen, liveWatermark); err != nil {
		   // 世代冲突：自动中止
		   abortErr := s.abortPlan(ctx, &p, "generation gate failed: "+err.Error())
		   if abortErr != nil {
			  return p, fmt.Errorf("restoregate: generation gate failed and abort failed: gen=%w abort=%v", err, abortErr)
		  }
		   return p, err
	  }
	  now := s.clock()
	  p.State = StateImporting
	  p.ImportStartAt = &now
	   p.UpdatedAt = now
	   if err := s.store.UpdatePlan(ctx, p); err != nil {
		   return RestorePlan{}, err
	  }
	  return p, nil
  }

// RecordHalfSeal 记录 Import 中途失败的封印。
  func (s *Service) RecordHalfSeal(ctx context.Context, planID string, writtenKeys []string, failReason string) error {
	  return s.seal.Seal(ctx, planID, writtenKeys, failReason)
   }

// StartVerifying 推进 plan 到 verifying。
   func (s *Service) StartVerifying(ctx context.Context, planID string) (RestorePlan, error) {
	   p, err := s.store.GetPlan(ctx, planID)
	   if err != nil {
		  return RestorePlan{}, err
	  }
	   if err := AssertTransition(p.State, StateVerifying); err != nil {
		   return RestorePlan{}, err
	  }
	  now := s.clock()
	   p.State = StateVerifying
	   p.VerifyStartAt = &now
	  p.UpdatedAt = now
	   if err := s.store.UpdatePlan(ctx, p); err != nil {
		  return RestorePlan{}, err
	  }
	  return p, nil
  }

// VerifyAndSeal 验证余额根并封印（或打开脑裂证明）。
// balanceRootPost 回灌后实测余额根
// diffSummary 差异摘要
// 若余额根与 plan.BalanceRootPre 声明目标一致 → 封印（sealed）
// 若不一致 → 创建 SplitBrainProof，plan 保持 verifying（等待四眼解除后由 ops 手动 seal）
  func (s *Service) VerifyAndSeal(ctx context.Context, planID, balanceRootPost, diffSummary string) (RestorePlan, error) {
	  p, err := s.store.GetPlan(ctx, planID)
	   if err != nil {
		   return RestorePlan{}, err
	   }
	  if p.State != StateVerifying {
		  return RestorePlan{}, fmt.Errorf("%w: expected verifying, got %s", ErrInvalidState, p.State)
	   }
	  p.BalanceRootPost = balanceRootPost
	  p.UpdatedAt = s.clock()
	   // 余额根校验
	   sbErr := s.sbGuard.CheckAndOpen(ctx, planID, p.BalanceRootPre, p.BalanceRootPre, balanceRootPost, diffSummary)
	  if sbErr != nil {
		   // 脑裂检出：保存 post root，保持 verifying 状态，挡写
		   if updErr := s.store.UpdatePlan(ctx, p); updErr != nil {
			   return p, fmt.Errorf("restoregate: update plan after split-brain: %w", updErr)
		  }
		  return p, sbErr
	   }
	  // 无脑裂：封印
	   now := s.clock()
	   p.State = StateSealed
	  p.SealedAt = &now
	   p.UpdatedAt = now
	   if err := s.store.UpdatePlan(ctx, p); err != nil {
		  return p, err
	  }
	   // 推进系统世代
	  _ = s.gate.AdvanceSystemGen(ctx, p.TargetGen)
	   // 释放写互斥锁
	  _ = s.guard.Release(ctx, planID)
	   return p, nil
  }

// ForceSeal 脑裂四眼解除后由 ops 强制封印（proof 已 cleared/override）。

  func (s *Service) ForceSeal(ctx context.Context, planID, actor string) (RestorePlan, error) {
	   p, err := s.store.GetPlan(ctx, planID)
	  if err != nil {
		   return RestorePlan{}, err
	  }
	  if p.State != StateVerifying {
		  return RestorePlan{}, fmt.Errorf("%w: expected verifying for force-seal", ErrInvalidState)
	   }
	   // 确认脑裂证明已解除
	  proof, proofErr := s.store.GetOpenProofByPlan(ctx, planID)
	  if proofErr == nil && (proof.State == SplitBrainOpen || proof.State == SplitBrainClearing) {
		   return p, fmt.Errorf("%w: split-brain proof not yet cleared", ErrSplitBrainOpen)
	   }
	  now := s.clock()
	   p.State = StateSealed
	   p.SealedAt = &now
	  p.UpdatedAt = now
	  if err := s.store.UpdatePlan(ctx, p); err != nil {
		   return p, err
	  }
	  _ = s.gate.AdvanceSystemGen(ctx, p.TargetGen)
	  _ = s.guard.Release(ctx, planID)
	   return p, nil
   }

// AbortPlan 强制中止计划（任意非 sealed 状态）。
  func (s *Service) AbortPlan(ctx context.Context, planID, reason string) (RestorePlan, error) {
	   p, err := s.store.GetPlan(ctx, planID)
	  if err != nil {
		  return RestorePlan{}, err
	   }
	   return p,
		  s.abortPlan(ctx, &p, reason)
   }

   func (s *Service) abortPlan(ctx context.Context, p *RestorePlan, reason string) error {
	   if p.State == StateSealed {
		   return ErrPlanSealed
	   }
	   if p.State == StateAborted {
		   return ErrPlanAborted
	   }
	   now := s.clock()
	  p.State = StateAborted
	   p.AbortedAt = &now
	   p.AbortReason = reason
	  p.UpdatedAt = now
	  if err := s.store.UpdatePlan(ctx, *p); err != nil {
		  return err
	   }
	   _ = s.guard.Release(ctx, p.ID)
	   return nil
   }

// GetPlan 查询计划详情。
  func (s *Service) GetPlan(ctx context.Context, planID string) (RestorePlan, error) {
	  if s == nil || s.store == nil {
		  return RestorePlan{},
			   ErrUnavailable
	  }
	   return s.store.GetPlan(ctx, planID)
  }

// ListPlans 列出计划摘要。
  func (s *Service) ListPlans(ctx context.Context, limit int) ([]PlanSummary, error) {
	   if s == nil || s.store == nil {
		  return nil, ErrUnavailable
	  }
	  return s.store.ListPlans(ctx,
		   limit)
  }

// CheckWritePath 供资金写路径（贷记/冻结/划转意图）调用的综合检查。
// 按序执行：
// 1. WriteMutex 检查
// 2. SplitBrainProof 全局检查
// 其中任一阻断则返回对应错误。
   func (s *Service) CheckWritePath(ctx context.Context) error {
	   if s == nil {
		  return nil
	  }
	  if err := s.guard.CheckWrite(ctx); err != nil {
		   return err
	  }
	   return s.sbGuard.CheckAnyOpenProof(ctx)
   }

// ResumeHalfSeal 续封印（允许重 Import 同计划）。
  func (s *Service) ResumeHalfSeal(ctx context.Context, planID string) error {
	   return s.seal.Resume(ctx, planID)
  }

// ApproveProof 四眼审批脑裂证明。
  func (s *Service) ApproveProof(ctx context.Context, proofID, approver, role, comment string) error {
	  return s.sbGuard.Approve(ctx, proofID, approver, role, comment)
   }

// OverrideProof 运维强制解除脑裂。
   func (s *Service) OverrideProof(ctx context.Context, proofID, actor, reason string) error {
	   return s.sbGuard.Override(ctx, proofID, actor, reason)
   }

// ExportProof 导出脑裂证明供审计。

   func (s *Service) ExportProof(ctx context.Context, proofID string) (SplitBrainProof, error) {
	   return s.sbGuard.ExportProof(ctx, proofID)
  }

// BalanceRoot 计算余额根 checksum（给 VerifyAndSeal 调用方使用）。
// entries 形态：key=userID|book|asset，value=available|frozen（均为字符串）
// 排序后 SHA-256，十六进制编码。

   func BalanceRoot(entries map[string]string) string {
	  if len(entries) == 0 {
		  return ""
	  }
	  keys := make([]string, 0, len(entries))
	  for k := range entries {
		  keys = append(keys, k)
	   }
	   sort.Strings(keys)
	   h := sha256.New()
	   for _, k := range keys {
		  _, _ = h.Write([]byte(k))
		  _, _ = h.Write([]byte{0})
		  _,
			   _ = h.Write([]byte(entries[k]))
		  _, _ = h.Write([]byte{0})
	   }
	  return hex.EncodeToString(h.Sum(nil))
   }

// CreatePlanRequest CreatePlan 入参。

type CreatePlanRequest struct {
	BlobID         string `json:"blob_id"`
	SnapID         string `json:"snap_id,omitempty"`
	TargetGen      int64  `json:"target_gen"`
	LiveWatermark  int64  `json:"live_watermark"`
	BalanceRootPre string `json:"balance_root_pre"`
	Creator string `json:"creator"`
	Reason string `json:"reason"`
}

// GenGates 列出世代闸审计记录。

  func (s *Service) GenGates(ctx context.Context, planID string) ([]GenGate, error) {
	   if s == nil || s.store == nil {
		   return nil, ErrUnavailable
	  }
	  return s.store.ListGenGates(ctx, planID)
   }

// BlockedWrites 列出被阻断的写意图审计日志。
   func (s *Service) BlockedWrites(ctx context.Context, planID string, limit int) ([]BlockedWrite, error) {
	   if s == nil || s.store == nil {
		   return nil, ErrUnavailable
	   }
	   m, err := s.store.GetMutexByPlan(ctx, planID)
	  if err != nil {
		   if errors.Is(err, ErrMutexNotFound) {
			  return nil, nil
		   }
		  return nil, err
	   }
	   return s.store.ListBlockedWrites(ctx, m.ID, limit)
  }

// ListOpenProofs 列出所有 open/clearing 脑裂证明。

  func (s *Service) ListOpenProofs(ctx context.Context) ([]SplitBrainProof, error) {
	   return s.sbGuard.ListOpenProofs(ctx)
  }

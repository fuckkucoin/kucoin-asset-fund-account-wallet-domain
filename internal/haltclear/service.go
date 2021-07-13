// 功能：haltclear Service：业务外观，串联四眼、误清检测、再熔断；供 HTTP 层和 cmd 调用。
// 所有操作均通过 Service 入口，不暴露底层 Repo 给传输层。

// 作者：Cason
// 日期：2021.07.10

package haltclear

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
)

// ---- Service -------------------------------------------------------
// Service haltclear 业务外观。
// 功能：
// 1. 清闸四眼：ProposePlan / FirstApprove / SecondApprove / RejectPlan；
// 2. 强制清闸升级：ForcedClearWithAudit；
// 3. 误清查询：ListMisClearCases / GetMisClearCase；
// 4. 再熔断调度：ScheduleReEngage / ConfirmReEngage（人工确认后执行）；
// 5. 案例关闭：CloseCase。
// 不写级联冻结计划；不写对账工单；不暴露 Gate 给调用方。

type Service struct {
	gate        *custodyhalt.Gate
	quorum      *custodyhalt.QuorumStore
	forcedAudit *custodyhalt.ForcedAuditStore
	detector *FalseClearDetector
	rollback *ReEngageRollback
	caseRepo CaseRepo
	legRepo  LegRepo
}

// ServiceConfig Service 构造参数。

type ServiceConfig struct {
	Gate   *custodyhalt.Gate
	Quorum *custodyhalt.QuorumStore
	ForcedAudit *custodyhalt.ForcedAuditStore
	Detector *FalseClearDetector
	Rollback *ReEngageRollback
	CaseRepo CaseRepo
	LegRepo  LegRepo
}

  func NewService(cfg ServiceConfig) *Service {
	   return &Service{
		   gate:   cfg.Gate,
		  quorum: cfg.Quorum,
		   forcedAudit: cfg.ForcedAudit,
		   detector: cfg.Detector,
		   rollback: cfg.Rollback,
		   caseRepo: cfg.CaseRepo,
		   legRepo:  cfg.LegRepo,
	  }
  }

// ---- 清闸四眼 -------------------------------------------------------
// ProposePlanReq 清闸提案请求。

type ProposePlanReq struct {
	PlanID string // 由调用方生成（UUID）
	Scope string
	Chain string
	Asset string
	Proposer string
	Reason   string
	OpenDiffs int // 提案时由调用方统计当前未关差异
	DiffSummary         string
	FreezeWarRoomPlanID string // 只读关联，不写级联
}

// ProposePlan 提交清闸提案。proposer 已算第一参与人，但仍需两名不同操作人审批。
   func (s *Service) ProposePlan(ctx context.Context, req ProposePlanReq) (*custodyhalt.ClearPlan, error) {
	   if s.quorum == nil {
		  return nil, errors.New("haltclear service: quorum store not configured")
	   }
	  plan := custodyhalt.ClearPlan{
		   PlanID: strings.TrimSpace(req.PlanID),
		  Scope: req.Scope,
		   Chain: strings.ToUpper(strings.TrimSpace(req.Chain)),
		  Asset:               strings.ToUpper(strings.TrimSpace(req.Asset)),
		  Proposer:            strings.TrimSpace(req.Proposer),
		  Reason:              strings.TrimSpace(req.Reason),
		   OpenDiffCount:       req.OpenDiffs,
		   FreezeWarRoomPlanID: req.FreezeWarRoomPlanID,
		  ProposedAt:          time.Now().UTC(),
	   }
	   if err := s.quorum.ProposeClearPlan(ctx, plan); err != nil {
		   return nil, fmt.Errorf("propose plan: %w", err)
	   }
	   stored, err := s.quorum.GetPlan(ctx, plan.PlanID)
	   if err != nil {
		   return nil, err
	  }
	   return &stored, nil
  }

// FirstApprovePlan 第一个审批人（须 ≠ proposer）签字。

   func (s *Service) FirstApprovePlan(ctx context.Context, planID, approver string) (*custodyhalt.ClearPlan, error) {
	   if err := s.quorum.FirstApprove(ctx, planID, approver); err != nil {
		  return nil, fmt.Errorf("first approve: %w", err)
	  }
	   p,
		  err := s.quorum.GetPlan(ctx, planID)
	   return &p, err
  }

// SecondApprovePlan 第二个审批人完成四眼；成功后返回 CloseProof。
  func (s *Service) SecondApprovePlan(ctx context.Context, planID, approver string, openDiffs int, diffSummary string) (*custodyhalt.CloseProof, error) {
	  proof, err := s.quorum.SecondApprove(ctx, planID, approver, openDiffs, diffSummary)
	  if err != nil {
		  return nil, fmt.Errorf("second approve: %w", err)
	   }
	  return proof, nil
   }

// RejectPlan 否决提案。
  func (s *Service) RejectPlan(ctx context.Context, planID, rejector, rejectReason string) (*custodyhalt.ClearPlan, error) {
	  if err := s.quorum.RejectPlan(ctx, planID, rejector, rejectReason); err != nil {
		  return nil, fmt.Errorf("reject plan: %w", err)
	  }
	  p, err := s.quorum.GetPlan(ctx, planID)
	  return &p, err
   }

// GetPlan 读取清闸提案。

  func (s *Service) GetPlan(ctx context.Context, planID string) (*custodyhalt.ClearPlan, error) {
	  p, err := s.quorum.GetPlan(ctx, planID)
	  if err != nil {
		  return nil, err
	  }
	   return &p, nil
  }

// ListPlans 列出清闸提案。

   func (s *Service) ListPlans(ctx context.Context, limit int) ([]custodyhalt.ClearPlan, error) {
	  return s.quorum.ListPlans(ctx, limit)
  }

// GetProof 读取闭合证明。

  func (s *Service) GetProof(ctx context.Context, proofID string) (*custodyhalt.CloseProof, error) {
	  p,
		   err := s.quorum.GetProof(ctx, proofID)
	  if err != nil {
		  return nil, err
	  }
	  return &p, nil
   }

// ---- 强制清闸升级 -------------------------------------------------------
// ForcedClearReq 强制清闸请求；必须携带理由码。
type ForcedClearReq struct {
	Scope string
	Chain string
	Asset string
	Actor string
	Reason string
	ReasonCode    string // 枚举值
	PostTicketID  string // other 理由码时必填
	OpenDiffCount int    // 调用方提供当前差异数（供审计与误清检测）
	DiffKeys      []string
	FlagSnapshot string // 当前熔断快照 JSON（可选）
}

// ForcedClear 强制清闸：校验理由码 → 执行 Gate.ClearForced → 落审计 → 检测误清。
   func (s *Service) ForcedClear(ctx context.Context, req ForcedClearReq) (*custodyhalt.ForcedAuditRecord, *MisClearCase, error) {
	   scope := custodyhalt.Scope(strings.TrimSpace(req.Scope))
	  if scope == "" {
		  scope = custodyhalt.ScopeGlobal
	   }
	  if err := custodyhalt.ValidateForced(req.ReasonCode, req.PostTicketID); err != nil {
		  return nil, nil, err
	  }
	  // 执行强制清闸并落审计。
	  auditID := fmt.Sprintf("fa:%d:%s", time.Now().UnixNano(), req.Actor)
	  err := custodyhalt.ForcedClearWithAudit(ctx,
		   s.gate, s.forcedAudit,
		   scope, req.Chain, req.Asset, req.Actor, req.Reason,
		  req.ReasonCode, req.PostTicketID,
		  req.OpenDiffCount,
		  req.FlagSnapshot,
	  )
	  if err != nil {
		  return nil, nil, fmt.Errorf("forced clear: %w", err)
	  }
	  rec := &custodyhalt.ForcedAuditRecord{
		  AuditID: auditID,
		   Scope: string(scope),
		   Chain:      strings.ToUpper(req.Chain),
		  Asset:      strings.ToUpper(req.Asset),
		   Actor:      req.Actor,
		  Reason:     req.Reason,
		  ReasonCode: req.ReasonCode,
		   PostTicketID: req.PostTicketID,
		   OpenDiffCount: req.OpenDiffCount,
		   MisClearRisk: req.OpenDiffCount > 0,
		   IssuedAt:     time.Now().UTC(),
	   }
	   // 同步检测是否产出误清案例。
	  var mc *MisClearCase
	  if s.detector != nil && req.OpenDiffCount > 0 {
		   mc, err = s.detector.DetectFromForcedAudit(ctx, auditID,
			   string(scope), req.Chain, req.Asset, req.OpenDiffCount, req.DiffKeys)
		  if err != nil {
			  // 检测失败不阻断业务
			   err = nil
		  }
	  }
	   return rec, mc, nil
   }

// ---- 误清查询 -------------------------------------------------------
// ListMisClearCases 列出误清案例。
   func (s *Service) ListMisClearCases(ctx context.Context, f CaseFilter) ([]*MisClearCase, error) {
	  return s.caseRepo.List(ctx, f)
   }

// GetMisClearCase 读取单个误清案例。

   func (s *Service) GetMisClearCase(ctx context.Context, misclearID string) (*MisClearCase, error) {
	   return s.caseRepo.Get(ctx, misclearID)
  }

// ---- 再熔断调度 -------------------------------------------------------
// ScheduleReEngageReq 再熔断调度请求。
type ScheduleReEngageReq struct {
	MisclearID string
	ActorID string
	Reason string
	IsAuto bool
}

// ScheduleReEngage 调度再熔断腿（不立即执行）。
   func (s *Service) ScheduleReEngage(ctx context.Context, req ScheduleReEngageReq) (*ReEngageLeg, error) {
	   return s.rollback.ScheduleReEngage(ctx, req.MisclearID, req.ActorID, req.Reason, req.IsAuto)
   }

// ConfirmReEngage 人工确认后立即执行指定腿。
  func (s *Service) ConfirmReEngage(ctx context.Context, legID string) (*RollbackProof, error) {
	   return s.rollback.ExecuteLeg(ctx, legID)
   }

// CloseCase 人工关闭已 reengaged 案例。

  func (s *Service) CloseCase(ctx context.Context, misclearID, closedBy, closeNote string) error {
	   return s.rollback.CloseCase(ctx, misclearID, closedBy, closeNote)
  }

// GetLeg 读取再熔断腿。

   func (s *Service) GetLeg(ctx context.Context, legID string) (*ReEngageLeg, error) {
	  return s.legRepo.Get(ctx, legID)
  }

// ListLegs 列出案例的全部再熔断腿。

  func (s *Service) ListLegs(ctx context.Context, misclearID string, limit int) ([]*ReEngageLeg, error) {
	  return s.legRepo.ListByCase(ctx, misclearID, limit)
  }

// VerifyProof 校验回滚证明。

  func (s *Service) VerifyProof(legID string, proof *RollbackProof) bool {
	  return VerifyRollbackProof(proof)
  }

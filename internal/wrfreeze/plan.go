package wrfreeze

import (
	"context"
	"sort"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/dualctl"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

const defaultTTLHours = 24

// PlanService 级联冻结计划编排服务。

type PlanService struct {
	store Store
	dual  *dualctl.Book
	freezer LedgerFreezer
	clock func() time.Time
}

// NewPlanService 构造计划编排服务。
// freezer 用于实际调用账本冻结位；dual 提供四眼审批；store 提供持久化。
  func NewPlanService(store Store, dual *dualctl.Book, freezer LedgerFreezer) *PlanService {
	  return &PlanService{
		  store: store, dual: dual, freezer: freezer,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
  }

  func (ps *PlanService) SetClock(fn func() time.Time) {
	  if ps != nil && fn != nil {
		   ps.clock = fn
	   }
   }

  func (ps *PlanService) now() time.Time {
	   if ps == nil || ps.clock == nil {
		   return time.Now().UTC()
	   }
	  return ps.clock()
  }

// OpenPlan 创建草稿计划并持久化。
// 对 ScopeUserSet 必须提供非空 UserSet；其他 scope 可为空。
// 返回新计划及对应 dualctl 四眼申请 ID（供调用方组合投票）。

  func (ps *PlanService) OpenPlan(ctx context.Context, req OpenPlanReq) (*CascadePlan, string, error) {
	  if ps == nil || ps.store == nil {
		  return nil, "", ErrUnavailable
	  }
	   actor, err := requireActor(req.Actor)
	   if err != nil {
		  return nil, "", err
	  }
	  reason, err := requireReason(req.Reason)
	  if err != nil {
		  return nil, "", err
	   }
	  incRef := strings.TrimSpace(req.IncidentRef)
	  if incRef == "" {
		  return nil, "", ErrIncidentRef
	   }
	  scope, err := validateScope(req.Scope)
	  if err != nil {
		  return nil, "", err
	   }
	  if scope == ScopeUserSet && len(req.UserSet) == 0 {
		  return nil, "", ErrUserSet
	   }
	   ttl := req.TTLHours
	  if ttl <= 0 {
		  ttl = defaultTTLHours
	   }
	   now := ps.now()
	   plan := &CascadePlan{
		  ID:          uuid.NewString(),
		  IncidentRef: incRef,
		  Scope: scope,
		  Asset:     strings.ToUpper(strings.TrimSpace(req.Asset)),
		   CohortTag: strings.TrimSpace(req.CohortTag),
		  UserSet:   sanitizeUserSet(req.UserSet),
		   Status:    PlanDraft,
		   OpenedBy:  actor,
		   ExpiresAt: now.Add(time.Duration(ttl) * time.Hour),
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	  // 开四眼申请（需 2 票批准方可武装）
	  var dualID string
	   if ps.dual != nil {
		   dr, derr := ps.dual.Open(
			  "cascade_plan_arm",
			  actor,
			  "cascade_plan:"+plan.ID+" incident:"+incRef,
			   2,
			  reason,
		  )
		   if derr != nil {
			   return nil, "", derr
		  }
		   dualID = dr.ID
		  plan.DualReqID = dualID
	   }
	   if err := ps.store.SavePlan(ctx, plan); err != nil {
		   return nil, "", err
	   }
	  return plan, dualID, nil
   }

// ArmPlan 武装计划：四眼已 ready 后将 draft → armed。
// 武装人不得是开单人（四眼原则由 dualctl 保证，此处仅补充业务隔离）。

  func (ps *PlanService) ArmPlan(ctx context.Context, req ArmPlanReq) (*CascadePlan, error) {
	   if ps == nil || ps.store == nil {
		   return nil, ErrUnavailable
	   }
	   actor, err := requireActor(req.Actor)
	  if err != nil {
		  return nil, err
	  }
	  plan,
		   err := ps.store.GetPlan(ctx, req.PlanID)
	   if err != nil {
		  return nil, err
	   }
	   if plan.Status != PlanDraft {
		  return nil, ErrPlanState
	   }
	   if strings.EqualFold(actor, plan.OpenedBy) {
		   return nil, ErrFourEye
	  }
	   if ps.now().After(plan.ExpiresAt) {
		  return nil, ErrExpired
	   }
	  // 校验 dualctl 四眼已就绪
	  if ps.dual != nil && plan.DualReqID != "" {
		   dr, derr := ps.dual.Get(plan.DualReqID)
		   if derr != nil {
			  return nil, derr
		  }
		   if dr.State != dualctl.StateReady && dr.State != dualctl.StateExecuted {
			   return nil, ErrDualCtl
		   }
	  }
	   now := ps.now()
	   plan.Status = PlanArmed
	  plan.ArmedBy = actor
	   plan.UpdatedAt = now
	   if err := ps.store.SavePlan(ctx, plan); err != nil {
		  return nil, err
	  }
	  return plan, nil
   }

// ExecutePlan 触发执行：armed → executing，启动第一波次。
// 执行触发人可以与武装人相同（允许同一 ops 流程自动推进）；
// 但不得是开单人（额外安全隔离）。

   func (ps *PlanService) ExecutePlan(ctx context.Context, req ExecutePlanReq) (*CascadePlan, error) {
	   if ps == nil || ps.store == nil {
		   return nil, ErrUnavailable
	   }
	  actor,
		  err := requireActor(req.Actor)
	   if err != nil {
		  return nil, err
	   }
	  plan, err := ps.store.GetPlan(ctx, req.PlanID)
	  if err != nil {
		  return nil, err
	  }
	   if plan.Status != PlanArmed {
		  return nil, ErrPlanState
	   }
	   if strings.EqualFold(actor, plan.OpenedBy) {
		  return nil, ErrFourEye
	  }
	   if ps.now().After(plan.ExpiresAt) {
		  return nil, ErrExpired
	   }
	   now := ps.now()
	  plan.Status = PlanExecuting
	   plan.UpdatedAt = now
	   if err := ps.store.SavePlan(ctx, plan); err != nil {
		   return nil, err
	   }
	  return plan, nil
  }

// AbortPlan 中止计划（任意非终态均可中止）。
// 中止不自动解冻；已冻结用户须走独立解冻阶段或回滚腿。
   func (ps *PlanService) AbortPlan(ctx context.Context, req AbortPlanReq) (*CascadePlan, error) {
	  if ps == nil || ps.store == nil {
		   return nil, ErrUnavailable
	  }
	   actor, err := requireActor(req.Actor)
	   if err != nil {
		   return nil, err
	   }
	  reason,
		   err := requireReason(req.Reason)
	   if err != nil {
		   return nil, err
	  }
	   plan, err := ps.store.GetPlan(ctx, req.PlanID)
	  if err != nil {
		  return nil, err
	   }
	  if plan.Status.Terminal() {
		  return nil, ErrPlanState
	   }
	   now := ps.now()
	   plan.Status = PlanAborted
	   plan.AbortedBy = actor
	  plan.AbortReason = reason
	  plan.UpdatedAt = now
	   if err := ps.store.SavePlan(ctx, plan); err != nil {
		   return nil, err
	   }
	  return plan, nil
   }

// SealPlan 封存计划：执行态且覆盖率证明 100% 后封存。
// 差额不为空则拒绝封存。actor 可为 "system"（worker 自动封存）。
   func (ps *PlanService) SealPlan(ctx context.Context, planID, actor string) (*CascadePlan, error) {
	  if ps == nil || ps.store == nil {
		  return nil, ErrUnavailable
	  }
	   // 允许 worker 以 "system" 身份触发封存（不经 validate.Actor 严格校验）
	   if strings.TrimSpace(actor) == "" {
		  return nil, ErrActor
	   }
	  plan, err := ps.store.GetPlan(ctx, planID)
	  if err != nil {
		   return nil, err
	  }
	  if plan.Status != PlanExecuting {
		   return nil, ErrPlanState
	   }
	   // 覆盖率证明检查
	  proof, err := ps.store.LatestProof(ctx, planID)
	  if err != nil {
		  return nil, err
	  }
	  if proof == nil || proof.Verdict != VerdictPassed {
		   return nil, ErrCoverageGap
	  }
	   now := ps.now()
	  plan.Status = PlanSealed
	   plan.SealedAt = &now
	  plan.UpdatedAt = now
	  if err := ps.store.SavePlan(ctx, plan); err != nil {
		   return nil, err
	  }
	  return plan, nil
   }

// GetPlan 查询计划。

  func (ps *PlanService) GetPlan(ctx context.Context, planID string) (*CascadePlan, error) {
	  if ps == nil || ps.store == nil {
		   return nil, ErrUnavailable
	  }
	   return ps.store.GetPlan(ctx, planID)
   }

// ListPlans 列举计划（按 status 过滤）。

  func (ps *PlanService) ListPlans(ctx context.Context, status string, limit int) ([]*CascadePlan, error) {
	  if ps == nil || ps.store == nil {
		  return nil, ErrUnavailable
	   }
	  return ps.store.ListPlans(ctx, status, limit)
   }

// ExpireOverduePlans 将超时的 draft/armed/executing 计划标记为 aborted。
// 由 worker 定期调用；不解冻已冻用户。
  func (ps *PlanService) ExpireOverduePlans(ctx context.Context) (int, error) {
	   if ps == nil || ps.store == nil {
		  return 0, ErrUnavailable
	  }
	   plans, err := ps.store.ListOverduePlans(ctx, ps.now())
	   if err != nil {
		   return 0, err
	  }
	   n := 0
	   for _, p := range plans {
		  if p.Status.Terminal() {
			  continue
		   }
		  p.Status = PlanAborted
		  p.AbortReason = "expired"
		  p.UpdatedAt = ps.now()
		   if serr := ps.store.SavePlan(ctx, p); serr == nil {
			   n++
		  }
	   }
	  return n, nil
   }

// ──────────────────────────────────────────
// helpers
// ──────────────────────────────────────────

   func requireActor(s string) (string, error) {
	   a, err := validate.Actor(s)
	  if err != nil {
		  return "", ErrActor
	  }
	  return a, nil
   }

  func requireReason(s string) (string, error) {
	  r, err := validate.Reason(s)
	   if err != nil {
		  return "", ErrReason
	   }
	   return r, nil
  }

   func validateScope(s PlanScope) (PlanScope, error) {
	  switch s {
	   case ScopeGlobal, ScopeAsset, ScopeCohort, ScopeUserSet:
		  return s, nil
	  default:
		   return "", ErrScope
	   }
   }

  func sanitizeUserSet(users []string) []string {
	  seen := make(map[string]struct{}, len(users))
	  out := make([]string,
		  0, len(users))
	   for _, u := range users {
		   u = strings.TrimSpace(u)
		   if u == "" {
			  continue
		  }
		   if _, ok := seen[u]; ok {
			   continue
		  }
		  seen[u] = struct{}{}
		   out = append(out, u)
	  }
	   sort.Strings(out)
	   return out
   }

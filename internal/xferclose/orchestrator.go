// 功能：xferclose 模块 orchestrator。
// 收口编排器。
// 统一入口：给定 HalfHop 工单，根据 SLA 和当前状态选择并执行收口动作：
// retry_advance → 重新推进 hop，由注入的 AdvanceFunc 执行
// rollback_egress → 回滚出腿（解冻发起方），由注入的 RollbackEgressFunc 执行
// backfill_ingress → 补入腿（补记对手方到账），由注入的 BackfillIngressFunc 执行
// escalate_desk → 转人工，写入 CloseoutPlan 后标记腿为 skipped
// open_incident → 开事故单，写入 CloseoutPlan 后标记腿为 skipped
// 幂等设计：同一 HopID 的有效 plan 只有一个；每条腿以 idemKey 去重；
// backfill_ingress 腿已 done 后重入直接返回 ErrIdempotent（不得双补入）。

// 作者：Orin
// 日期：2021.12.30

package xferclose

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// AdvanceFunc 重新推进 hop；clientOid 为幂等键。
type AdvanceFunc func(ctx context.Context, clientOid string) error

// RollbackEgressFunc 回滚出腿（解冻发起方）；idem 为本次回滚的幂等键。
// 返回 entryRef（账本回滚记录 ID）。
type RollbackEgressFunc func(ctx context.Context,
	  hopID, idem string) (entryRef string, err error)

// BackfillIngressFunc 补入腿（补记对手方到账）；idem 为本次补入的幂等键。
// 返回 entryRef（账本补记 ID）。

type BackfillIngressFunc func(ctx context.Context, hopID, asset, toBook, userID, idem string) (entryRef string, err error)

// IncidentFunc 开事故工单。返回 incidentID。

type IncidentFunc func(ctx context.Context, hopID, reason string) (incidentID string, err error)

// Orchestrator 收口编排器。

type Orchestrator struct {
	store Store
	sla   *SLABoard
	plans    *PlanManager
	advance  AdvanceFunc
	rollback RollbackEgressFunc
	backfill BackfillIngressFunc
	incident IncidentFunc
}

// NewOrchestrator 构造编排器，注入各执行函数。
  func NewOrchestrator(
	  store Store,
	   sla *SLABoard,
	  advance AdvanceFunc,
	   rollback RollbackEgressFunc,
	  backfill BackfillIngressFunc,
	  incident IncidentFunc,
  ) *Orchestrator {
	   return &Orchestrator{
		   store:    store,
		  sla:      sla,
		  plans:    NewPlanManager(store),
		  advance:  advance,
		   rollback: rollback,
		  backfill: backfill,
		   incident: incident,
	   }
  }

// Closeout 对指定 HalfHop 执行收口。
// 流程：
// 1. 检查 hop 是否处于合法状态（未终态）。
// 2. 从 SLA 规则选定动作（尊重调用方传入的 forceAction）。
// 3. 获取或开启 CloseoutPlan（幂等）。
// 4. 将 plan 推进到 running。
// 5. 分发到具体执行函数，写收口腿，更新腿状态。
// 6. 根据腿结果更新 plan（done/abort）。
// 7. 根据 plan 结果更新 HalfHop（closed/escalated）。
  func (o *Orchestrator) Closeout(ctx context.Context, hop HalfHop, forceAction CloseoutAction, actor string) (CloseoutResult, error) {
	  if hop.Terminal() {
		   return CloseoutResult{},
			   ErrAlreadyTerminal
	   }
	   if strings.TrimSpace(actor) == "" {
		  actor = actorSystem
	  }
	  // 选定动作。
	  action := forceAction
	  if action == "" {
		  slaMatch := o.sla.MustMatch(hop.Asset, hop.FromBook, hop.ToBook, hop.Amount, hop.DetectedAt)
		  action = slaMatch.Rule.DefaultAction
	  }
	  // 幂等获取或开启计划。
	   plan, err := o.getOrOpenPlan(ctx, hop, action, actor)
	  if err != nil {
		   return CloseoutResult{}, err
	  }
	  if plan.State == PlanDone {
		  return CloseoutResult{PlanID: plan.ID, HopID: hop.HopID, Action: action, Success: true}, nil
	  }
	  if plan.State == PlanAborted {
		   return CloseoutResult{PlanID: plan.ID,
			  HopID: hop.HopID, Action: action, Success: false, Error: plan.AbortReason}, nil
	   }
	  // 推进 plan 到 running。
	  plan, err = o.plans.StartPlan(ctx, plan.ID)
	   if err != nil {
		   return CloseoutResult{}, err
	  }
	   // 执行收口腿。
	   result, execErr := o.execute(ctx, plan, hop)
	   // 记录尝试。
	  _ = o.plans.RecordAttempt(ctx, plan.ID, execErr)
	  if execErr != nil {
		   exhausted, _ := o.plans.ExhaustedAttempts(ctx, plan.ID)
		  if exhausted {
			  _, _ = o.plans.AbortPlan(ctx, plan.ID, fmt.Sprintf("max attempts: %v", execErr))
			   o.markHopEscalated(ctx, hop, fmt.Sprintf("closeout plan %s aborted: %v", plan.ID, execErr))
		   }
		   result.Error = execErr.Error()
		  return result, execErr
	  }
	  // 所有腿成功，标记计划 done。
	  if _, err2 := o.plans.MarkDone(ctx, plan.ID); err2 != nil {
		   return result, err2
	   }
	   o.markHopClosed(ctx, hop, plan.ID)
	   result.Success = true
	   return result, nil
  }

// ForceCloseout 运维强制收口，actor 必须非空。
   func (o *Orchestrator) ForceCloseout(ctx context.Context, hopID, actor string, action CloseoutAction) (CloseoutResult, error) {
	  if strings.TrimSpace(actor) == "" {
		  return CloseoutResult{},
			  ErrActorRequired
	   }
	   hop, err := o.store.GetByHopID(ctx, hopID)
	  if err != nil {
		  return CloseoutResult{}, err
	  }
	   return o.Closeout(ctx,
		  hop, action, actor)
  }

// getOrOpenPlan 获取现有计划或开新计划。若已有 running 计划则复用。
  func (o *Orchestrator) getOrOpenPlan(ctx context.Context, hop HalfHop, action CloseoutAction, actor string) (CloseoutPlan, error) {
	  existing, found, err := o.store.GetPlanByHopID(ctx, hop.HopID)
	  if err != nil {
		   return CloseoutPlan{}, err
	  }
	  if found && (existing.State == PlanOpen || existing.State == PlanRunning) {
		  return existing, nil
	  }
	  plan, err := o.plans.OpenPlan(ctx, hop, action, actor)
	   if errors.Is(err, ErrPlanRunning) {
		   p, _, _ := o.store.GetPlanByHopID(ctx, hop.HopID)
		  return p, nil
	  }
	  if errors.Is(err, ErrPlanDone) {
		  p, _, _ := o.store.GetPlanByHopID(ctx, hop.HopID)
		  return p, nil
	  }
	   return plan, err
   }

// execute 分发到具体执行函数并管理腿状态。
   func (o *Orchestrator) execute(ctx context.Context, plan CloseoutPlan, hop HalfHop) (CloseoutResult, error) {
	   result := CloseoutResult{PlanID: plan.ID, HopID: hop.HopID, Action: plan.SelectedAction}
	  idemKey := legIdemKey(plan.ID, plan.SelectedAction, plan.Attempts+1)
	   leg,
		   err := o.plans.BuildLeg(ctx, plan, plan.Attempts+1, plan.SelectedAction, idemKey)
	  if errors.Is(err, ErrIdempotent) {
		   // 腿已存在：检查状态。
		  leg, err = o.store.GetLegByIdem(ctx, idemKey)
		   if err != nil {
			  return result, err
		  }
		  if leg.State == LegDone {
			   result.Success = true
			   return result, nil
		   }
	   } else if err != nil {
		  return result, err
	   }
	  _ = o.plans.UpdateLegState(ctx, leg.ID, LegRunning, "", "")
	   var execErr error
	   var entryRef string
	  switch plan.SelectedAction {
	   case ActionRetryAdvance:
		   execErr = o.doRetryAdvance(ctx, hop)
		  if execErr == nil {
			   entryRef = "advance-ok"
		   }
	  case ActionRollbackEgress:
		   entryRef, execErr = o.doRollbackEgress(ctx, hop, idemKey)
	   case ActionBackfillIngress:
		  // 双补保护：检查是否已有 done 的 backfill 腿。
		  if alreadyBackfilled(o.store, ctx, plan.ID) {
			   _ = o.plans.UpdateLegState(ctx, leg.ID, LegSkipped, "", "already backfilled")
			   return result, ErrIdempotent
		  }
		   entryRef, execErr = o.doBackfillIngress(ctx, hop, idemKey)
	   case ActionEscalateDesk:
		  _ = o.plans.UpdateLegState(ctx, leg.ID, LegSkipped, "", "escalated to desk")
		  return result, nil
	  case ActionOpenIncident:
		   entryRef,
			  execErr = o.doOpenIncident(ctx, hop, plan.ID)
	  default:
		   execErr = fmt.Errorf("unknown action: %s", plan.SelectedAction)
	   }
	  if execErr != nil {
		  _ = o.plans.UpdateLegState(ctx, leg.ID, LegFailed, "", execErr.Error())
		  return result, execErr
	  }
	   _ = o.plans.UpdateLegState(ctx, leg.ID, LegDone, entryRef, "")
	   return result, nil
   }

   func (o *Orchestrator) doRetryAdvance(ctx context.Context, hop HalfHop) error {
	  if o.advance == nil {
		   return fmt.Errorf("advance func not configured")
	   }
	  return o.advance(ctx, hop.HopID)
   }

   func (o *Orchestrator) doRollbackEgress(ctx context.Context, hop HalfHop, idem string) (string, error) {
	   if o.rollback == nil {
		  return "", fmt.Errorf("rollback egress func not configured")
	  }
	  return o.rollback(ctx, hop.HopID, idem)
  }

  func (o *Orchestrator) doBackfillIngress(ctx context.Context, hop HalfHop, idem string) (string, error) {
	  if o.backfill == nil {
		  return "", fmt.Errorf("backfill ingress func not configured")
	   }
	   return o.backfill(ctx, hop.HopID, hop.Asset, hop.ToBook, hop.UserID, idem)
   }

  func (o *Orchestrator) doOpenIncident(ctx context.Context, hop HalfHop, planID string) (string, error) {
	  if o.incident == nil {
		   return "", fmt.Errorf("incident func not configured")
	  }
	  return o.incident(ctx, hop.HopID, fmt.Sprintf("closeout plan %s triggered incident for hop %s", planID, hop.HopID))
   }

  func (o *Orchestrator) markHopClosed(ctx context.Context, hop HalfHop, planID string) {
	   hop.State = HopClosed
	   hop.CloseoutPlanID = planID
	   now := time.Now().UTC()
	   hop.ClosedAt = &now
	  hop.UpdatedAt = now
	  _ = o.store.UpdateHop(ctx, hop)
  }

   func (o *Orchestrator) markHopEscalated(ctx context.Context, hop HalfHop, reason string) {
	  hop.State = HopEscalated
	  hop.EscalateReason = reason
	  hop.UpdatedAt = time.Now().UTC()
	   _ = o.store.UpdateHop(ctx, hop)
  }

// legIdemKey 构造腿的幂等键：planID + action + attemptNo。
   func legIdemKey(planID string, action CloseoutAction, attempt int) string {
	  return fmt.Sprintf("leg:%s:%s:%d", planID, action, attempt)
   }

// alreadyBackfilled 检查该计划是否已有成功的 backfill_ingress 腿（双补保护）。

  func alreadyBackfilled(store LegStore, ctx context.Context, planID string) bool {
	   legs, err := store.ListLegs(ctx, planID)
	  if err != nil {
		   return false
	  }
	  for _, l := range legs {
		  if l.Action == ActionBackfillIngress && l.State == LegDone {
			  return true
		   }
	  }
	  return false
  }

// RunBatchCloseout 批量执行已超 SLA 且 auto_closeout=true 的计划。
// 每轮最多执行 batchSize 条，避免长时间占用执行线程。
  func (o *Orchestrator) RunBatchCloseout(ctx context.Context, batchSize int) (int, int, error) {
	   if batchSize <= 0 {
		   batchSize = 20
	   }
	  hops, err := o.store.ListByState(ctx, HopDetected, 0, batchSize)
	  if err != nil {
		   return 0, 0, err
	   }
	  succeeded, failed := 0, 0
	   for _, hop := range hops {
		   if !hop.SLABreached {
			   continue
		  }
		   slaMatch := o.sla.MustMatch(hop.Asset,
			   hop.FromBook, hop.ToBook, hop.Amount, hop.DetectedAt)
		  if !slaMatch.Rule.AutoCloseout {
			  continue
		   }
		  // 跨分片 hop 不由本包处理。
		   if hop.XShard {
			  continue
		  }
		  _, execErr := o.Closeout(ctx, hop, "", actorSystem)
		  if execErr == nil || errors.Is(execErr, ErrIdempotent) || errors.Is(execErr, ErrPlanDone) {
			  succeeded++
		  } else {
			  failed++
		  }
	   }
	  return succeeded, failed, nil
  }

// GetPlanLegs 返回计划的所有腿（供 API 查询）。
  func (o *Orchestrator) GetPlanLegs(ctx context.Context, planID string) ([]CloseoutLeg, error) {
	   return o.store.ListLegs(ctx, planID)
  }

// GetHopCloseoutView 返回 HalfHop + 关联 Plan + Legs 的聚合视图。

   func (o *Orchestrator) GetHopCloseoutView(ctx context.Context, hopID string) (*HopCloseoutView, error) {
	   hop, err := o.store.GetByHopID(ctx, hopID)
	   if err != nil {
		  return nil, err
	  }
	  view := &HopCloseoutView{Hop: hop}
	   plan, found, err := o.store.GetPlanByHopID(ctx, hopID)
	   if err != nil {
		   return nil, err
	  }
	  if found {
		   view.Plan = &plan
		   legs, _ := o.store.ListLegs(ctx, plan.ID)
		  view.Legs = legs
	  }
	  return view, nil
  }

// HopCloseoutView 聚合视图：工单 + 收口计划 + 腿列表。
type HopCloseoutView struct {
	Hop  HalfHop       `json:"hop"`
	Plan *CloseoutPlan `json:"plan,omitempty"`
	Legs []CloseoutLeg `json:"legs,omitempty"`
}

// IdemKey 暴露给的辅助函数。

   func IdemKey(planID string, action CloseoutAction, attempt int) string {
	  return legIdemKey(planID, action, attempt)
   }

// GeneratePlanID 生成新计划 ID（仅使用）。

   func GeneratePlanID() string { return uuid.NewString() }

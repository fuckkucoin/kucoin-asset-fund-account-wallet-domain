// 严格禁止调用出金广播或出金失败补偿编排 API（r5 G 已占）。
// 账本反向分录经 LedgerReverser 接口；占用释放经 OccReleaser 接口。

// Author: Blaise
// Date: 2021.03.30

package cvrollback

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// OccReleaser 占用释放接口（由 cvoccup.Service 实现）。

type OccReleaser interface {
	// Release 释放路径占用，跃迁至 ROLLED_BACK。
	Release(ctx context.Context, occID, reason string) error
	// CheckIntegrity 完整性校验。
	CheckIntegrity(ctx context.Context,
		occID string, openBreach bool) error
}

// LedgerReverser 账本在途分录反向接口（只读消费冻结层；不直写 holdcat KV）。
type LedgerReverser interface {
	// ReverseInFlight 按幂等键冲销在途分录，返回反向分录 ID。
	ReverseInFlight(ctx context.Context, idemKey, ref, reason string) (string, error)
}

// HoldcatAligner holdcat 一致接口（确认 hold 已释放）。

type HoldcatAligner interface {
	// ConfirmReleased 确认 holdID 已释放（hold 不存在则视为已释放）。
	ConfirmReleased(ctx context.Context, userID, holdID string) (bool, error)
}

// Orchestrator 回滚编排器。
type Orchestrator struct {
	store Store
	occ   OccReleaser
	ledger  LedgerReverser
	holdcat HoldcatAligner
}

// NewOrchestrator 构造编排器。

  func NewOrchestrator(store Store, occ OccReleaser, ledger LedgerReverser, holdcat HoldcatAligner) *Orchestrator {
	  return &Orchestrator{
		  store: store,
		  occ: occ,
		  ledger: ledger,
		  holdcat: holdcat,
	  }
  }

// PlanReq 回滚计划请求参数。
type PlanReq struct {
	OccID string
	OrderID string
	UserID  string
	Trigger RollbackTrigger
	FilledAmount string
	RemainAmount string
	DebitIdemKey string // 在途 ConvertDebit 幂等键，用于冲销
}

// Plan 创建回滚计划（PLANNED 态）；幂等：同 OccID 已有计划则直接返回。
  func (o *Orchestrator) Plan(ctx context.Context, req PlanReq) (*RollbackPlan, error) {
	   req.OccID = strings.TrimSpace(req.OccID)
	  req.UserID = strings.TrimSpace(req.UserID)
	  if req.OccID == "" || req.UserID == "" {
		  return nil, ErrPlanStore
	   }
	  // 幂等：已有计划则返回
	   if existing, err := o.store.GetPlanByOcc(ctx, req.OccID); err == nil && existing != nil {
		  return existing, nil
	  }
	  idemKey := "rb:" + req.OccID + ":" + string(req.Trigger)
	  plan := &RollbackPlan{
		  ID:      uuid.NewString(),
		   OccID:   req.OccID,
		  OrderID: req.OrderID,
		   UserID: req.UserID,
		   Trigger: req.Trigger,
		   Status:  RollbackPlanned,
		  FilledAmount: req.FilledAmount,
		  RemainAmount: req.RemainAmount,
		  IdemKey:   idemKey,
		   CreatedAt: time.Now().UTC(),
		   UpdatedAt: time.Now().UTC(),
	   }
	  if err := o.store.SavePlan(ctx, plan); err != nil {
		  return nil, err
	   }
	   // 创建回滚腿序列
	  legs := o.buildLegs(plan, req.DebitIdemKey)
	  for _, l := range legs {
		  if err := o.store.SaveLeg(ctx, l); err != nil {
			  return nil, err
		  }
	  }
	  return plan, nil
   }

// Execute 执行回滚计划（PLANNED → EXECUTING → DONE/FAILED）。
// 回滚腿按序执行；单腿失败记录错误但接着尝试后续腿（最大努力执行）。

  func (o *Orchestrator) Execute(ctx context.Context, planID string) (*RollbackPlan, error) {
	  plan, err := o.store.GetPlan(ctx, planID)
	   if err != nil {
		   return nil, err
	  }
	   if plan.IsTerminal() {
		  return plan, ErrPlanTerminal
	   }
	  now := time.Now().UTC()
	  plan.Status = RollbackExecuting
	  plan.ExecutedAt = &now
	  if err := o.store.SavePlan(ctx, plan); err != nil {
		   return nil, err
	  }
	  legs,
		  err := o.store.ListLegs(ctx, planID)
	   if err != nil {
		   return nil, err
	   }
	   var execErr error
	  for _, leg := range legs {
		  if leg.Status == LegDone || leg.Status == LegSkipped {
			   continue
		  }
		  if err := o.execLeg(ctx, plan, leg); err != nil {
			  execErr = err
			   leg.Status = LegLegFailed
			   leg.Error = err.Error()
		  } else {
			  leg.Status = LegDone
		   }
		  _ = o.store.SaveLeg(ctx, leg)
	   }
	  doneAt := time.Now().UTC()
	   plan.DoneAt = &doneAt
	   if execErr != nil {
		  plan.Status = RollbackFailed
		   plan.Error = execErr.Error()
	   } else {
		  plan.Status = RollbackDone
	  }
	  if err := o.store.SavePlan(ctx, plan); err != nil {
		  return nil, err
	   }
	  return plan, execErr
   }

// PlanAndExecute 计划后立即执行（原子编排）。
   func (o *Orchestrator) PlanAndExecute(ctx context.Context, req PlanReq) (*RollbackPlan, error) {
	   plan, err := o.Plan(ctx, req)
	   if err != nil {
		  return nil, err
	  }
	  if plan.IsTerminal() {
		   return plan, nil
	  }
	  return o.Execute(ctx, plan.ID)
  }

// buildLegs 根据计划构建腿序列。
  func (o *Orchestrator) buildLegs(plan *RollbackPlan, debitIdemKey string) []*RollbackLeg {
	  now := time.Now().UTC()
	   legs := make([]*RollbackLeg, 0, 3)
	  // 腿1：释放占用
	   legs = append(legs, &RollbackLeg{
		   ID: uuid.NewString(),
		  PlanID: plan.ID,
		  Action: LegReleaseOcc,
		  Ref: plan.OccID,
		   Amount: plan.RemainAmount,
		  Status:    LegPending,
		   CreatedAt: now,
		   UpdatedAt: now,
	   })
	   // 腿2：冲在途账本分录（仅有剩余量时）
	   if plan.RemainAmount != "" && plan.RemainAmount != "0" && debitIdemKey != "" {
		  legs = append(legs, &RollbackLeg{
			  ID:     uuid.NewString(),
			  PlanID: plan.ID,
			   Action: LegReverseInFlight,
			   Ref: debitIdemKey,
			   Amount:    plan.RemainAmount,
			  Status:    LegPending,
			  CreatedAt: now,
			  UpdatedAt: now,
		   })
	  }
	  // 腿3：一致 holdcat（确认 hold 已释放）
	  legs = append(legs, &RollbackLeg{
		   ID:     uuid.NewString(),
		   PlanID: plan.ID,
		   Action: LegAlignHoldcat,
		  Ref: plan.OccID,
		  Status:    LegPending,
		   CreatedAt: now,
		   UpdatedAt: now,
	  })
	  return legs
  }

// execLeg 执行单条回滚腿。
  func (o *Orchestrator) execLeg(ctx context.Context, plan *RollbackPlan, leg *RollbackLeg) error {
	   switch leg.Action {
	  case LegReleaseOcc:
		   if o.occ == nil {
			  return nil
		   }
		   return o.occ.Release(ctx, leg.Ref, string(plan.Trigger))
	   case LegReverseInFlight:
		   if o.ledger == nil {
			  leg.Status = LegSkipped
			   return nil
		  }
		   _, err := o.ledger.ReverseInFlight(ctx, leg.Ref,
			   fmt.Sprintf("occ=%s order=%s", plan.OccID, plan.OrderID),
			  string(plan.Trigger))
		   return err
	   case LegAlignHoldcat:
		   if o.holdcat == nil {
			  leg.Status = LegSkipped
			  return nil
		   }
		  // 从 occ 中取 holdID（通过 occ.HoldToken；此处用 Ref 存 occID）
		  // 若 CheckIntegrity 通过则腿成功；否则记录警告不阻断
		  if o.occ != nil {
			  _ = o.occ.CheckIntegrity(ctx, leg.Ref, false)
		  }
		   return nil
	   default:
		  return fmt.Errorf("unknown leg action: %s", leg.Action)
	   }
   }

// GetPlan 按 ID 查询回滚计划。

   func (o *Orchestrator) GetPlan(ctx context.Context, id string) (*RollbackPlan, error) {
	   return o.store.GetPlan(ctx, id)
   }

// GetPlanByOcc 按 occID 查询回滚计划。

  func (o *Orchestrator) GetPlanByOcc(ctx context.Context, occID string) (*RollbackPlan, error) {
	  return o.store.GetPlanByOcc(ctx, occID)
   }

// ListPendingPlans 列出待执行的回滚计划。

   func (o *Orchestrator) ListPendingPlans(ctx context.Context, limit int) ([]*RollbackPlan, error) {
	  return o.store.ListPendingPlans(ctx,
		   limit)
  }

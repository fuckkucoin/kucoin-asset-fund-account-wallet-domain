package dustguard

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// PoolForceReleaser 提供强制释放（绕过冷却）的池操作接口。
// 由 addrpool.Pool 实现。

type PoolForceReleaser interface {
	ForceRelease(userID, chain string) error
	ByAddress(addr string) (userID, chain string, ok bool)
}

// InventoryReserver 库存预留接口。
// 由 addrpool.InventoryAdapter 实现，把 Inventory.Reserve/Bind 包一层。
type InventoryReserver interface {
	Reserve(chain, userID string) (addr string, err error)
	Bind(addr, userID string) error
}

// SharedMemoReg 共享地址注册接口（FamMEMO rotate 时更新共享地址）。
type SharedMemoReg interface {
	Register(chain, address string) error
	IsShared(addr string) (chain string, ok bool)
}

// RotateApproval 轮换双人复核接口（由 approve.go 封装 approvestore）。
type RotateApproval interface {
	OpenRotate(ctx context.Context, actor, planID, chain, oldAddr string) (approvalID string, err error)
	ApproveRotate(ctx context.Context, approvalID, actor string, allow bool, epoch int64, token string) error
	IsApproved(ctx context.Context, approvalID string) (bool, error)
	MarkExecuted(ctx context.Context, approvalID, actor string, epoch int64, token string) error
}

// Rotator 智能轮换编排器。
type Rotator struct {
	store Store
	fsm *QuarantineFSM
	policy *PolicyManager
	pool   PoolForceReleaser
	inv    InventoryReserver
	shared SharedMemoReg
	approval RotateApproval
	clock func() time.Time
}

// NewRotator 构造轮换编排器。
   func NewRotator(store Store, fsm *QuarantineFSM, policy *PolicyManager,
	   pool PoolForceReleaser, inv InventoryReserver, shared SharedMemoReg, approval RotateApproval,
  ) *Rotator {
	  return &Rotator{
		   store: store,
		   fsm: fsm,
		  policy: policy,
		   pool:   pool,
		   inv: inv,
		  shared:   shared,
		   approval: approval,
		  clock:    func() time.Time { return time.Now().UTC() },
	   }
  }

// RejectUserSelfRotate 拒绝用户自助轮换 FamMEMO 共享地址。
// 凡共享地址被检测为目标时调用；1:1 地址不拦截。
  func (r *Rotator) RejectUserSelfRotate(addr string) error {
	   if r.shared == nil {
		  return nil
	   }
	   if _, ok := r.shared.IsShared(addr); ok {
		  return ErrUserRotateRejected
	   }
	  return nil
  }

// OpenAutoRotate 因攻击分值或水位自动触发轮换（1:1 地址专用，FamMEMO 禁止自动）。
// AutoRotate=false 时只建计划但不执行，须人工介入。

   func (r *Rotator) OpenAutoRotate(ctx context.Context, chain, oldAddr, userID, caseID string, trigger TriggerKind) (*RotatePlan, error) {
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   // 共享地址不得走自动路径
	  if r.shared != nil {
		  if _, ok := r.shared.IsShared(oldAddr); ok {
			   return nil, fmt.Errorf("auto-rotate is forbidden for shared FamMEMO address %s; use ops dual-approve", oldAddr)
		  }
	   }
	  pol := r.policy.Active(ctx, chain)
	  now := r.clock()
	   plan := RotatePlan{
		  ID: uuid.NewString(),
		  Chain:      chain,
		   OldAddress: oldAddr,
		   UserID:     userID,
		   CaseID:     caseID,
		   State:      RStatePlanned,
		   Trigger:    trigger,
		   Reason:    fmt.Sprintf("auto rotate triggered by %s", trigger),
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	  if err := r.store.CreatePlan(ctx, plan); err != nil {
		   return nil, err
	   }
	  audit := newAudit(AuditRotate, plan.ID, "system", "plan", "", string(RStatePlanned), plan.Reason)
	  _ = r.store.WriteAudit(ctx, audit)
	  if pol.AutoRotate {
		   executed, err := r.executeRotate(ctx, &plan, "system")
		  if err != nil {
			  // 执行失败记录但不回滚计划；留人工处理
			  DustRotateTotal.WithLabelValues(chain, "auto_failed").Inc()
			  return &plan, fmt.Errorf("plan created but execute failed: %w", err)
		   }
		   DustRotateTotal.WithLabelValues(chain, "auto_done").Inc()
		  return executed, nil
	  }
	   DustRotateTotal.WithLabelValues(chain, "planned").Inc()
	   return &plan, nil
  }

// OpenOpsRotate 运维手动触发轮换计划。
// FamMEMO 共享地址强制走双人复核，返回计划 ID 和 approvalID。
// 1:1 地址若 AutoRotate=true 也可直接执行（ops 明确传 execute=true）。
  func (r *Rotator) OpenOpsRotate(ctx context.Context, chain, oldAddr, newAddr, userID, actor, reason string) (*RotatePlan, error) {
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   actor = strings.TrimSpace(actor)
	   if actor == "" {
		   return nil, fmt.Errorf("actor required for ops rotate")
	  }
	  isShared := false
	   if r.shared != nil {
		  if _, ok := r.shared.IsShared(oldAddr); ok {
			  isShared = true
		  }
	   }
	  now := r.clock()
	  plan := RotatePlan{
		  ID:         uuid.NewString(),
		  Chain:      chain,
		  OldAddress: oldAddr,
		   NewAddress: newAddr,
		  UserID:     userID,
		  State:      RStatePlanned,
		   Trigger: TriggerOps,
		   Reason: reason,
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	   if isShared {
		   // FamMEMO：必须双人复核；开 approval
		  if r.approval == nil {
			   return nil, fmt.Errorf("approval store not configured; cannot open FamMEMO rotate")
		   }
		   approvalID, err := r.approval.OpenRotate(ctx, actor, plan.ID, chain, oldAddr)
		  if err != nil {
			  return nil, fmt.Errorf("open rotate approval: %w", err)
		  }
		  plan.ApprovalID = approvalID
		  plan.State = RStatePlanned
	   }
	  if err := r.store.CreatePlan(ctx, plan); err != nil {
		  return nil, err
	  }
	  audit := newAudit(AuditRotate, plan.ID, actor, "ops_plan", "", string(RStatePlanned), reason)
	   _ = r.store.WriteAudit(ctx, audit)
	  DustRotateTotal.WithLabelValues(chain, "ops_planned").Inc()
	  return &plan, nil
   }

// ApproveOpsRotate 第二名运维审批轮换计划（仅 FamMEMO 共享地址必须）。
// epoch/token 由前端从 GET approval 接口取得（approvestore 围栏机制）。
  func (r *Rotator) ApproveOpsRotate(ctx context.Context, planID, actor string, allow bool, epoch int64, token string) (*RotatePlan, error) {
	  plan, err := r.store.GetPlan(ctx, planID)
	   if err != nil {
		  return nil, err
	   }
	   if plan.ApprovalID == "" {
		  return nil, fmt.Errorf("plan %s has no approval; not a FamMEMO rotate", planID)
	   }
	  if r.approval == nil {
		   return nil, fmt.Errorf("approval store not configured")
	  }
	  if err := r.approval.ApproveRotate(ctx, plan.ApprovalID, actor, allow, epoch, token); err != nil {
		   return nil, fmt.Errorf("approve rotate: %w", err)
	   }
	  now := r.clock()
	  from := string(plan.State)
	   if allow {
		   plan.State = RStateApproved
	   } else {
		  plan.State = RStateAborted
	   }
	   plan.UpdatedAt = now
	  if err := r.store.UpdatePlan(ctx, plan); err != nil {
		  return nil, err
	   }
	  action := "approve"
	   if !allow {
		   action = "deny"
	   }
	  audit := newAudit(AuditRotate, planID, actor, action, from, string(plan.State), "")
	  _ = r.store.WriteAudit(ctx,
		   audit)
	   return &plan, nil
  }

// ExecuteRotate 执行已通过复核的轮换计划。
// FamMEMO 须先通过 ApproveOpsRotate；1:1 直接执行。
// 执行成功后 RotatePlan 进入 swapped，轮换后隔离 old_address。

  func (r *Rotator) ExecuteRotate(ctx context.Context, planID, actor string, epoch int64, token string) (*RotateResult, error) {
	   plan, err := r.store.GetPlan(ctx, planID)
	  if err != nil {
		   return nil, err
	   }
	   isShared := false
	   if r.shared != nil {
		  if _, ok := r.shared.IsShared(plan.OldAddress); ok {
			   isShared = true
		   }
	  }
	   if isShared {
		   // FamMEMO 须确认 approvestore 已 approved
		   if plan.State != RStateApproved {
			  return nil, fmt.Errorf("%w: plan state is %s", ErrRotateNotApproved, plan.State)
		   }
		  if r.approval == nil {
			  return nil, fmt.Errorf("approval store not configured")
		  }
		  approved, err := r.approval.IsApproved(ctx, plan.ApprovalID)
		   if err != nil {
			  return nil, err
		  }
		  if !approved {
			   return nil, ErrRotateNotApproved
		  }
		  // 标记 approval executed
		   if err := r.approval.MarkExecuted(ctx, plan.ApprovalID, actor, epoch, token); err != nil {
			   return nil, fmt.Errorf("mark approval executed: %w", err)
		  }
		  // 更新共享地址
		  if plan.NewAddress != "" && r.shared != nil {
			  if err := r.shared.Register(plan.Chain, plan.NewAddress); err != nil {
				   return nil, fmt.Errorf("register new shared address: %w", err)
			  }
		  }
	  } else {
		   // 1:1 地址：Reserve + Bind + ForceRelease
		   executed, err := r.executeRotate(ctx, &plan, actor)
		  if err != nil {
			  return nil, err
		  }
		   return &RotateResult{Plan: *executed,
			   NewAddress: executed.NewAddress, UserID: plan.UserID}, nil
	  }
	   now := r.clock()
	   from := string(plan.State)
	  plan.State = RStateSwapped
	   plan.ExecutedBy = actor
	  plan.UpdatedAt = now
	  if err := r.store.UpdatePlan(ctx, plan); err != nil {
		   return nil, err
	  }
	  // 封印
	  t := now
	   plan.SealedAt = &t
	   plan.State = RStateSealed
	   plan.UpdatedAt = now
	  _ = r.store.UpdatePlan(ctx, plan)
	   audit := newAudit(AuditRotate, planID, actor, "execute", from, string(RStateSealed), "")
	  _ = r.store.WriteAudit(ctx, audit)
	   DustRotateTotal.WithLabelValues(plan.Chain, "done").Inc()
	  return &RotateResult{Plan: plan, NewAddress: plan.NewAddress, UserID: plan.UserID}, nil
  }

// executeRotate 执行 1:1 地址轮换的实际 IO 操作。
// 流程：Reserve 新槽 → Bind → ForceRelease 旧地址 → 更新计划。
  func (r *Rotator) executeRotate(ctx context.Context, plan *RotatePlan, actor string) (*RotatePlan, error) {
	   if r.pool == nil || r.inv == nil {
		  return nil, ErrNoFreeSlot
	   }
	   userID := plan.UserID
	   chain := plan.Chain
	   if userID == "" {
		  // 从 pool 查询旧地址的 owner
		   uid,
			   _, ok := r.pool.ByAddress(plan.OldAddress)
		  if !ok {
			   return nil, fmt.Errorf("old address %s not found in pool", plan.OldAddress)
		  }
		  userID = uid
		  plan.UserID = uid
	   }
	   // 预留新槽
	   newAddr, err := r.inv.Reserve(chain, userID)
	   if err != nil {
		   return nil, fmt.Errorf("%w: reserve: %v", ErrNoFreeSlot, err)
	  }
	  if err := r.inv.Bind(newAddr, userID); err != nil {
		  return nil, fmt.Errorf("bind new address %s: %w", newAddr, err)
	  }
	  // 强制释放旧地址（绕过冷却）
	   if err := r.pool.ForceRelease(userID, chain); err != nil {
		   return nil, fmt.Errorf("force release old address: %w", err)
	   }
	  now := r.clock()
	   from := string(plan.State)
	   plan.NewAddress = newAddr
	   plan.State = RStateSwapped
	   plan.ExecutedBy = actor
	  plan.UpdatedAt = now
	   if err := r.store.UpdatePlan(ctx, *plan); err != nil {
		  return nil, err
	  }
	   t := now
	   plan.SealedAt = &t
	   plan.State = RStateSealed
	  plan.UpdatedAt = now
	  _ = r.store.UpdatePlan(ctx, *plan)
	   audit := newAudit(AuditRotate, plan.ID, actor, "execute", from, string(RStateSealed),
		   fmt.Sprintf("new=%s old=%s", newAddr, plan.OldAddress))
	  _ = r.store.WriteAudit(ctx, audit)
	   DustRotateTotal.WithLabelValues(chain, "done").Inc()
	  return plan, nil
   }

// AbortPlan 中止一个尚未执行的计划。

   func (r *Rotator) AbortPlan(ctx context.Context, planID, actor, reason string) (*RotatePlan, error) {
	  plan, err := r.store.GetPlan(ctx, planID)
	   if err != nil {
		  return nil, err
	   }
	   if plan.State == RStateSealed || plan.State == RStateAborted {
		  return nil, fmt.Errorf("plan already in terminal state %s", plan.State)
	  }
	   from := string(plan.State)
	   now := r.clock()
	   plan.State = RStateAborted
	   plan.UpdatedAt = now
	  if err := r.store.UpdatePlan(ctx, plan); err != nil {
		  return nil, err
	  }
	  audit := newAudit(AuditRotate, planID, actor, "abort", from, string(RStateAborted), reason)
	  _ = r.store.WriteAudit(ctx,
		   audit)
	   DustRotateTotal.WithLabelValues(plan.Chain, "aborted").Inc()
	   return &plan, nil
   }

// ListPlans 列出轮换计划。
  func (r *Rotator) ListPlans(ctx context.Context, state RotateState, chain string) ([]RotatePlan, error) {
	   return r.store.ListPlans(ctx, state, chain)
  }

// GetPlan 按 ID 取计划。
  func (r *Rotator) GetPlan(ctx context.Context, id string) (RotatePlan, error) {
	  return r.store.GetPlan(ctx, id)
  }

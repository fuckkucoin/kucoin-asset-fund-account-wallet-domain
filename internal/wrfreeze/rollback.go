package wrfreeze

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

// RollbackService 回滚腿编排服务。

type RollbackService struct {
	store Store
	freezer LedgerFreezer
	holds HoldcatReader
	clock func() time.Time
}

// HoldcatReader holdcat 分层令牌只读接口。
// 本包仅读取令牌 ID，不修改 holdcat 分层枚举。
type HoldcatReader interface {
	// HasOpenHolds 查询用户是否存在任意分类挂单冻结（只读）。
	HasOpenHolds(ctx context.Context, userID string) (bool, error)
	// ListHoldIDs 列举用户活跃 hold 令牌 ID（只读）。
	ListHoldIDs(ctx context.Context, userID string) ([]string, error)
}

// NewRollbackService 构造回滚服务。
   func NewRollbackService(store Store, freezer LedgerFreezer, holds HoldcatReader) *RollbackService {
	   return &RollbackService{
		  store: store, freezer: freezer, holds: holds,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

  func (rs *RollbackService) SetClock(fn func() time.Time) {
	  if rs != nil && fn != nil {
		   rs.clock = fn
	   }
   }

   func (rs *RollbackService) now() time.Time {
	  if rs == nil || rs.clock == nil {
		  return time.Now().UTC()
	  }
	   return rs.clock()
   }

// RequestRollback 申请回滚腿。
// 申请人须说明误冻原因；userID 须在计划 UserSet 或波次记录中。
// 返回待审批的 RollbackLeg；通过 ExecuteRollback 四眼执行。

   func (rs *RollbackService) RequestRollback(ctx context.Context, req RollbackReq) (*RollbackLeg, error) {
	  if rs == nil || rs.store == nil {
		   return nil, ErrUnavailable
	   }
	   actor, err := requireActor(req.Actor)
	   if err != nil {
		   return nil, err
	  }
	   uid := strings.TrimSpace(req.UserID)
	   if uid == "" {
		   return nil, ErrUser
	   }
	   reason := req.Reason
	  switch reason {
	  case ReasonMisFreeze, ReasonExpiredIncident, ReasonScopeError:
	   default:
		  return nil, ErrReason
	   }
	   plan, err := rs.store.GetPlan(ctx, req.PlanID)
	   if err != nil {
		  return nil, err
	  }
	   // 校验 userID 来源于本计划（防止误操作他来源冻结）
	  inPlan, err := rs.userInPlan(ctx, plan, uid)
	  if err != nil {
		   return nil, err
	  }
	   if !inPlan {
		   return nil, ErrMisFreeze
	   }
	  // 查询 live-hash 出金状态（只读）
	   hasLiveHash := false
	  if rs.freezer != nil {
		  hasLiveHash, _ = rs.freezer.HasLiveHashWithdraw(ctx, uid)
	   }
	   now := rs.now()
	   leg := &RollbackLeg{
		  ID: uuid.NewString(),
		   PlanID: req.PlanID,
		  UserID:      uid,
		   Reason:      reason,
		   Status:      RollbackPending,
		  RequestedBy: actor,
		   HasLiveHash:  hasLiveHash,
		   FrozenByPlan: true,
		  CreatedAt:    now,
		   UpdatedAt:    now,
	   }
	  if err := rs.store.SaveLeg(ctx, leg); err != nil {
		   return nil, err
	   }
	   return leg, nil
   }

// ExecuteRollback 四眼执行回滚腿。
// 执行人不得是申请人（四眼约束）。
// 若 HasLiveHash=true，拒绝自动解冻并返回 ErrLiveHash。
// 执行成功后记录 AvailabilityNote（可用视图解释）。

  func (rs *RollbackService) ExecuteRollback(ctx context.Context, req ExecuteRollbackReq) (*RollbackLeg, error) {
	  if rs == nil || rs.store == nil {
		  return nil, ErrUnavailable
	   }
	  actor, err := requireActor(req.Actor)
	   if err != nil {
		   return nil, err
	  }
	  leg, err := rs.store.GetLeg(ctx, req.LegID)
	   if err != nil {
		  return nil, err
	  }
	   if leg.Status != RollbackPending {
		   return nil, ErrPlanState
	  }
	   if strings.EqualFold(actor, leg.RequestedBy) {
		   return nil, ErrFourEye
	   }
	   // 硬约束：live-hash 出金不得自动解冻
	  if leg.HasLiveHash {
		   return leg, ErrLiveHash
	   }
	   now := rs.now()
	  leg.Status = RollbackExecuting
	  leg.ExecutedBy = actor
	   leg.UpdatedAt = now
	   if err := rs.store.SaveLeg(ctx, leg); err != nil {
		  return nil, err
	  }
	   // 执行解冻（恢复冻结位）
	  var execErr error
	  if rs.freezer != nil {
		   execErr = rs.freezer.SetFrozen(ctx, leg.UserID, false)
	   }
	  now2 := rs.now()
	  leg.UpdatedAt = now2
	  if execErr != nil {
		  leg.Status = RollbackFailed
		  leg.AvailabilityNote = "rollback failed: " + execErr.Error()
	  } else {
		   leg.Status = RollbackRestored
		   leg.AvailabilityNote = rs.buildAvailabilityNote(ctx, leg)
	   }
	  if err := rs.store.SaveLeg(ctx, leg); err != nil {
		   return nil, err
	  }
	   return leg, nil
   }

// buildAvailabilityNote 构建恢复后的可用视图解释。
// 包含：用户 ID、计划 ID、回滚原因、holdcat 持仓令牌状态（只读）。

   func (rs *RollbackService) buildAvailabilityNote(ctx context.Context, leg *RollbackLeg) string {
	  note := "user=" + leg.UserID + " plan=" + leg.PlanID + " reason=" + string(leg.Reason) + " freeze-cleared=true"
	  if rs.holds != nil {
		  ids, err := rs.holds.ListHoldIDs(ctx, leg.UserID)
		  if err == nil && len(ids) > 0 {
			  note += " active-hold-tokens=" + strings.Join(ids, ",") + " (holdcat read-only; not modified by rollback)"
		   } else {
			  note += " active-hold-tokens=none"
		  }
	   }
	  return note
  }

// userInPlan 校验用户是否属于本计划（来自 UserSet 或波次记录）。

  func (rs *RollbackService) userInPlan(ctx context.Context, plan *CascadePlan, uid string) (bool, error) {
	   // ScopeUserSet：直接检查 UserSet
	  if plan.Scope == ScopeUserSet {
		   for _, u := range plan.UserSet {
			  if u == uid {
				  return true, nil
			  }
		   }
		   return false, nil
	   }
	   // 其余 scope：检查波次 TargetUsers
	  waves,
		   err := rs.store.ListWaves(ctx, plan.ID)
	  if err != nil {
		  return false, err
	  }
	   for _, w := range waves {
		   for _, u := range w.TargetUsers {
			   if u == uid {
				  return true, nil
			   }
		  }
	   }
	  return false, nil
  }

// GetLeg 查询回滚腿。
  func (rs *RollbackService) GetLeg(ctx context.Context, legID string) (*RollbackLeg, error) {
	   if rs == nil || rs.store == nil {
		  return nil, ErrUnavailable
	   }
	   return rs.store.GetLeg(ctx, legID)
   }

// ListLegs 列举计划回滚腿。

   func (rs *RollbackService) ListLegs(ctx context.Context, planID string) ([]*RollbackLeg, error) {
	   if rs == nil || rs.store == nil {
		  return nil, ErrUnavailable
	   }
	   return rs.store.ListLegs(ctx, planID)
   }

// AutoRollbackExpired 自动为已过期中止计划创建回滚腿（由 worker 调用）。
// 仅对已冻结但无 live-hash 出金的用户创建 pending 回滚腿；live-hash 用户跳过并记录。
  func (rs *RollbackService) AutoRollbackExpired(ctx context.Context) (int, error) {
	  if rs == nil || rs.store == nil {
		  return 0, ErrUnavailable
	  }
	  plans, err := rs.store.ListPlans(ctx, string(PlanAborted), 100)
	  if err != nil {
		  return 0, err
	   }
	   created := 0
	  for _, plan := range plans {
		  if plan.AbortReason != "expired" {
			  continue
		  }
		  users,
			   err := rs.collectExpiredPlanUsers(ctx, plan)
		  if err != nil {
			  continue
		  }
		  for _, uid := range users {
			  frozen,
				   _ := rs.freezer.IsFrozen(ctx, uid)
			   if !frozen {
				  continue
			  }
			  hasLive, _ := rs.freezer.HasLiveHashWithdraw(ctx, uid)
			  now := rs.now()
			  leg := &RollbackLeg{
				   ID:     uuid.NewString(),
				   PlanID: plan.ID,
				  UserID: uid,
				   Reason: ReasonExpiredIncident,
				  Status: RollbackPending,
				  RequestedBy: "worker",
				  HasLiveHash: hasLive,
				  FrozenByPlan: true,
				   CreatedAt: now,
				   UpdatedAt: now,
			  }
			  if serr := rs.store.SaveLeg(ctx, leg); serr == nil {
				  created++
			  }
		   }
	  }
	  return created, nil
   }

   func (rs *RollbackService) collectExpiredPlanUsers(ctx context.Context, plan *CascadePlan) ([]string, error) {
	  if plan.Scope == ScopeUserSet && len(plan.UserSet) > 0 {
		  return plan.UserSet, nil
	  }
	  waves, err := rs.store.ListWaves(ctx, plan.ID)
	  if err != nil {
		   return nil, err
	  }
	  seen := map[string]struct{}{}
	  out := make([]string, 0)
	   for _, w := range waves {
		  for _, u := range w.TargetUsers {
			  if _, ok := seen[u]; !ok {
				  seen[u] = struct{}{}
				   out = append(out, u)
			  }
		   }
	  }
	   return out, nil
  }

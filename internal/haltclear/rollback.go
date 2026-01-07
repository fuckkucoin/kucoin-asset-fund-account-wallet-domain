package haltclear

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
)

// ---- 错误 -----------------------------------------------------------

var (
	ErrNoActiveLeg    = errors.New("reengage rollback: no active leg for case")
	ErrAlreadyEngaged = errors.New("reengage rollback: case already reengaged")
	ErrCaseNotOpen    = errors.New("reengage rollback: case must be in open state")
	ErrLegNotPending = errors.New("reengage rollback: leg must be in pending state to execute")
)

// ---- ReEngageRollback -----------------------------------------------
// ReEngageRollback 误清再熔断执行器。
// 功能：根据 MisClearCase 状态创建 ReEngageLeg，调用 Gate.Engage/EngageScope 恢复熔断，产出回滚证明。
// 不写战争室 freeze_plan；不调用 ClearForced；不写对账工单。
// 不变量：每个 open 案例至多一条活跃腿；再熔断成功后案例进入 reengaged 态。
type ReEngageRollback struct {
	gate     *custodyhalt.Gate
	caseRepo CaseRepo
	legRepo   LegRepo
	diffQuery OpenDiffQuerier
}

// NewReEngageRollback 构造再熔断执行器。

   func NewReEngageRollback(
	   gate *custodyhalt.Gate,
	  caseRepo CaseRepo,
	   legRepo LegRepo,
	  diffQuery OpenDiffQuerier,
  ) *ReEngageRollback {
	  return &ReEngageRollback{
		  gate:     gate,
		  caseRepo: caseRepo,
		   legRepo:   legRepo,
		  diffQuery: diffQuery,
	  }
  }

// ScheduleReEngage 为误清案例创建待执行再熔断腿。
// isAuto=true：worker 自动调度；false：需人工确认后执行。
// 前置：案例处于 open 状态；无活跃腿。
   func (r *ReEngageRollback) ScheduleReEngage(ctx context.Context, misclearID, actorID, reason string, isAuto bool) (*ReEngageLeg, error) {
	   c, err := r.caseRepo.Get(ctx, misclearID)
	   if err != nil {
		   return nil, err
	  }
	  if c.State != MisClearStateOpen {
		   return nil, fmt.Errorf("%w: state=%s", ErrCaseNotOpen, c.State)
	  }
	   existing, err := r.legRepo.ActiveLeg(ctx, misclearID)
	  if err != nil {
		  return nil, err
	  }
	   if existing != nil {
		  return existing, nil // 已有活跃腿，幂等返回
	   }
	  now := time.Now().UTC()
	   leg := &ReEngageLeg{
		  LegID:      newLegID(misclearID),
		   MisclearID: misclearID,
		  Scope:      c.Scope,
		  Chain:   c.Chain,
		   Asset:   c.Asset,
		   State:   ReEngageStatePending,
		  ActorID: strings.TrimSpace(actorID),
		   Reason:  strings.TrimSpace(reason),
		  IsAuto: isAuto,
		  ScheduledAt: now,
		   CreatedAt:   now,
		  UpdatedAt:   now,
	   }
	  if err := r.legRepo.Create(ctx, leg); err != nil {
		   return nil, fmt.Errorf("schedule reengage leg: %w", err)
	   }
	  return leg, nil
   }

// ExecuteLeg 执行指定再熔断腿：调用 Gate 熔断，更新腿与案例状态，生成回滚证明。
// 前置：腿处于 pending 状态。
// 功能：自动/半自动路径均通过此入口执行；失败时腿标记为 failed，案例保持 open。
  func (r *ReEngageRollback) ExecuteLeg(ctx context.Context, legID string) (*RollbackProof, error) {
	   leg, err := r.legRepo.Get(ctx, legID)
	   if err != nil {
		  return nil, err
	  }
	  if leg.State != ReEngageStatePending {
		  return nil, fmt.Errorf("%w: state=%s", ErrLegNotPending, leg.State)
	   }
	  // 标记为执行中。
	   now := time.Now().UTC()
	   leg.State = ReEngageStateExecuting
	  leg.ExecutedAt = now
	   leg.UpdatedAt = now
	   if err := r.legRepo.Update(ctx, leg); err != nil {
		   return nil, fmt.Errorf("mark leg executing: %w", err)
	  }
	   // 查询当前差异数（写入证明）。
	  openDiffs, _, diffErr := r.diffQuery.QueryOpenDiffs(ctx, leg.Scope, leg.Chain, leg.Asset)
	  if diffErr != nil {
		   openDiffs = -1 // 查询失败时记为 -1，不阻断再熔断
	   }
	  // 执行熔断。
	  engageErr := r.doEngage(ctx, leg)
	   now = time.Now().UTC()
	   if engageErr != nil {
		   leg.State = ReEngageStateFailed
		   leg.FailedAt = now
		   leg.FailReason = engageErr.Error()
		  leg.UpdatedAt = now
		   _ = r.legRepo.Update(ctx, leg)
		   return nil, fmt.Errorf("reengage engage failed: %w", engageErr)
	  }
	  // 生成回滚证明。
	  proof := NewRollbackProof(leg,
		  openDiffs)
	   leg.State = ReEngageStateEngaged
	   leg.EngagedAt = now
	  leg.RollbackProof = proof
	  leg.UpdatedAt = now
	  if err := r.legRepo.Update(ctx, leg); err != nil {
		   return nil, fmt.Errorf("update leg engaged: %w", err)
	  }
	   // 更新案例状态为 reengaged。
	   c, err := r.caseRepo.Get(ctx, leg.MisclearID)
	  if err == nil {
		  c.State = MisClearStateReengaged
		  c.ReengagedAt = now
		  c.ReengageActorID = leg.ActorID
		   c.ReengageLegID = leg.LegID
		   c.UpdatedAt = now
		  _ = r.caseRepo.Update(ctx, c)
	   }
	  return proof, nil
  }

// CloseCase 人工关闭已 reengaged 的案例（差异已处理）。

   func (r *ReEngageRollback) CloseCase(ctx context.Context, misclearID, closedBy, closeNote string) error {
	   c, err := r.caseRepo.Get(ctx, misclearID)
	   if err != nil {
		  return err
	   }
	   if c.State == MisClearStateClosed {
		   return ErrCaseTerminal
	   }
	   now := time.Now().UTC()
	   c.State = MisClearStateClosed
	  c.ClosedAt = now
	   c.ClosedBy = strings.TrimSpace(closedBy)
	   c.CloseNote = strings.TrimSpace(closeNote)
	   c.UpdatedAt = now
	   return r.caseRepo.Update(ctx, c)
   }

// AutoReEngage 便捷方法：一步调度 + 执行（仅供 auto 路径调用）。
// 功能：检测到 open 案例后立即调度并执行自动再熔断。

  func (r *ReEngageRollback) AutoReEngage(ctx context.Context, misclearID string) (*RollbackProof, error) {
	   leg, err := r.ScheduleReEngage(ctx, misclearID, "system", "auto-reengage: misclear detected", true)
	   if err != nil {
		  return nil, err
	  }
	  return r.ExecuteLeg(ctx, leg.LegID)
  }

// ---- 内部辅助 -------------------------------------------------------

  func (r *ReEngageRollback) doEngage(ctx context.Context, leg *ReEngageLeg) error {
	   scope := custodyhalt.Scope(leg.Scope)
	   actor := leg.ActorID
	  reason := fmt.Sprintf("[reengage-rollback] misclear=%s leg=%s %s", leg.MisclearID, leg.LegID, leg.Reason)
	  if scope == custodyhalt.ScopeGlobal || scope == "" {
		   return r.gate.Engage(ctx, actor, reason)
	  }
	  return r.gate.EngageScope(ctx, scope, leg.Chain, leg.Asset, actor, reason)
  }

   func newLegID(misclearID string) string {
	  return fmt.Sprintf("leg:%s:%d", misclearID, time.Now().UnixNano())
  }

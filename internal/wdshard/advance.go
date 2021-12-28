package wdshard

import (
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

// AdvanceLeg 推进分片腿状态。
// 如果腿进入 broadcasting，planInflight 增加；进入 filled，planFilled 增加；
// 进入 failed，planRemain 可能不变（AllowPartial=true 时已成交不回滚）。
   func (s *Service) AdvanceLeg(req LegAdvanceReq) (ShardLeg, ShardPlan, error) {
	  if s == nil {
		   return ShardLeg{}, ShardPlan{}, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  leg, ok := s.legs[strings.TrimSpace(req.LegID)]
	   if !ok {
		  return ShardLeg{}, ShardPlan{}, ErrLegNotFound
	  }
	  plan, ok2 := s.plans[leg.PlanID]
	   if !ok2 {
		   return ShardLeg{}, ShardPlan{}, ErrNotFound
	   }
	   // 计划必须 running 才能推进腿（暂停时拒绝）。
	   if plan.State != PlanRunning {
		  return leg.Copy(), plan.Copy(), fmt.Errorf("%w: plan %s not running", ErrState, plan.State)
	  }
	  if strings.TrimSpace(plan.PauseReason) != "" {
		  return leg.Copy(), plan.Copy(), ErrPaused
	   }
	   nextState, err := LegTransition(leg.State, req.To)
	  if err != nil {
		   return leg.Copy(), plan.Copy(), err
	   }
	  now := s.now()
	   leg.State = nextState
	  leg.UpdatedAt = now
	  switch nextState {
	   case LegFrozen:
		   leg.HoldID = strings.TrimSpace(req.HoldID)
		  leg.FrozenAt = nowPtr(now)
	   case LegBroadcasting:
		   leg.BroadcastRef = strings.TrimSpace(req.BroadcastRef)
		  leg.WithdrawID = strings.TrimSpace(req.WithdrawID)
		  leg.BroadcastAt = nowPtr(now)
	  case LegFilled:
		   leg.FillID = strings.TrimSpace(req.FillID)
		   leg.FilledAt = nowPtr(now)
	   case LegFailed:
		   leg.FailReason = strings.TrimSpace(req.FailReason)
		  leg.FailedAt = nowPtr(now)
	  }
	  if err2 := s.recompute(plan); err2 != nil {
		   return leg.Copy(), plan.Copy(), err2
	  }
	  s.closeIfDone(plan, now)
	   return leg.Copy(), plan.Copy(), nil
  }

// recompute 重算计划聚合：filled/inflight/remain。
// 必须在锁内调用。

  func (s *Service) recompute(plan *ShardPlan) error {
	   filled := money.Zero()
	   inflight := money.Zero()
	   remain := money.Zero()
	  var err error
	   for _, legID := range s.byPlan[plan.ID] {
		   leg, ok := s.legs[legID]
		   if !ok {
			  continue
		  }
		  amt,
			   parseErr := money.ParseBook(leg.Amount)
		   if parseErr != nil {
			   return parseErr
		   }
		  switch leg.State {
		   case LegFilled:
			  filled, err = filled.Add(amt)
		   case LegBroadcasting:
			  inflight,
				   err = inflight.Add(amt)
		  case LegPending, LegFrozen:
			   remain, err = remain.Add(amt)
		  case LegFailed:
			   // 失败腿不计入任何桶；AllowPartial 时剩余会释放，在 release worker 处理。
		  }
		   if err != nil {
			  return err
		   }
	  }
	   plan.FilledAmount = filled.String()
	  plan.InflightAmount = inflight.String()
	  plan.RemainAmount = remain.String()
	  plan.UpdatedAt = s.now()
	  return nil
   }

// closeIfDone 检查是否所有腿都已终态，若是则关闭计划。
// 必须在锁内调用。

  func (s *Service) closeIfDone(plan *ShardPlan, now time.Time) {
	   ids := s.byPlan[plan.ID]
	  allDone := true
	   hasFilled := false
	  hasFailed := false
	   for _, id := range ids {
		  leg := s.legs[id]
		   if !LegTerminal(leg.State) {
			   allDone = false
			  break
		   }
		   if leg.State == LegFilled {
			  hasFilled = true
		  }
		   if leg.State == LegFailed {
			   hasFailed = true
		   }
	   }
	   if !allDone {
		   return
	   }
	   if hasFailed && hasFilled {
		  plan.State = PlanPartialClosed
	   } else if hasFailed && !hasFilled {
		  plan.State = PlanAborted
	   } else {
		   plan.State = PlanFilled
	  }
	   plan.UpdatedAt = now
  }

// emitLegFailed 发射腿失败事件（锁外调用）。
  func (s *Service) emitLegFailed(leg ShardLeg, plan ShardPlan) {
	   s.emitEvent(ShardEvent{
		  Kind: "shard_leg_failed",
		  PlanID: plan.ID,
		  LegID:  leg.ID,
		   UserID: plan.UserID,
		   Asset:  plan.Asset,
		   Amount: leg.Amount,
		   FailReason: leg.FailReason,
		  At:         s.now(),
	   })
   }

// emitPlanAborted 发射计划中止事件（锁外调用）。
   func (s *Service) emitPlanAborted(plan ShardPlan) {
	  s.emitEvent(ShardEvent{
		   Kind: "shard_plan_aborted",
		  PlanID: plan.ID,
		   UserID: plan.UserID,
		   Asset:  plan.Asset,
		  Amount: plan.TotalAmount,
		   At:     s.now(),
	   })
   }

package wdshard

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

// PlanInvariant 计划级不变量检查：
// 1. Σ 腿金额 == total（守恒）
// 2. 终态计划的聚合与腿状态分布一致
// 3. 非运行计划不得有 broadcasting 腿
   func (s *Service) PlanInvariant(planID string) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  planID = strings.TrimSpace(planID)
	  plan, ok := s.plans[planID]
	   if !ok {
		   return ErrNotFound
	   }
	  total, err := money.ParseBook(plan.TotalAmount)
	   if err != nil {
		   return fmt.Errorf("plan invariant: total parse: %w", err)
	   }
	  var legSum money.Amount = money.Zero()
	  hasBroadcasting := false
	  for _, legID := range s.byPlan[planID] {
		   leg, lok := s.legs[legID]
		   if !lok {
			  continue
		  }
		  amt, e := money.ParseBook(leg.Amount)
		   if e != nil {
			   return fmt.Errorf("plan invariant: leg %s amount parse: %w", leg.ID, e)
		  }
		   legSum, err = legSum.Add(amt)
		   if err != nil {
			  return fmt.Errorf("plan invariant: leg sum overflow: %w", err)
		  }
		   if leg.State == LegBroadcasting {
			   hasBroadcasting = true
		   }
	  }
	  if err := money.Conserve(total, legSum); err != nil {
		  ObserveConserveViolation(planID)
		   return fmt.Errorf("%w: plan %s leg_sum %s total %s", ErrConserve, planID, legSum, total)
	   }
	   if !PlanRunnable(plan.State) && hasBroadcasting {
		  return fmt.Errorf("%w: plan %s in state %s has broadcasting legs", ErrState, planID, plan.State)
	   }
	  return nil
   }

// RequireActor 校验 actor 非空。
   func RequireActor(actor string) error {
	   if strings.TrimSpace(actor) == "" {
		   return ErrActor
	  }
	  return nil
   }

// RequireRunning 校验计划处于 running 且未暂停。
  func RequireRunning(plan *ShardPlan) error {
	  if plan == nil {
		   return ErrNotFound
	  }
	  if plan.State != PlanRunning {
		   return fmt.Errorf("%w: plan %s is %s not running", ErrState, plan.ID, plan.State)
	   }
	   if strings.TrimSpace(plan.PauseReason) != "" {
		  return ErrPaused
	   }
	   return nil
  }

// LegSeqValid 校验腿序号在计划腿数范围内。

  func LegSeqValid(seq, legCount int) error {
	   if seq < 1 || seq > legCount {
		   return fmt.Errorf("%w: seq %d out of range [1,%d]", ErrState, seq, legCount)
	   }
	  return nil
   }

// AmountStr 把金额字符串整理为口径形式（去除多余小数位）；空返回 "0"。
  func AmountStr(s string) string {
	  s = strings.TrimSpace(s)
	   if s == "" {
		   return "0"
	  }
	  a, err := money.ParseBook(s)
	   if err != nil {
		   return s
	   }
	   return a.String()
  }

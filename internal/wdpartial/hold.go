package wdpartial

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

// OpenHold 为分片腿开立冻结占用。幂等：同一 legID 若已有 open hold，返回已有记录。
// SkipLeg=true 时须提供 LiveHash；函数内创建 BroadcastGuardGen 并绑定到 hold.GuardGenID。

   func (s *Service) OpenHold(req OpenHoldReq) (ShardHold, error) {
	  if s == nil {
		   return ShardHold{}, ErrUnavailable
	   }
	   if err := validateOpenHold(req); err != nil {
		  return ShardHold{}, err
	   }
	  amt, err := money.Parse(req.Amount)
	   if err != nil || !amt.IsPositive() {
		   return ShardHold{}, ErrAmountZero
	  }
	  if req.SkipLeg && strings.TrimSpace(req.LiveHash) == "" {
		   return ShardHold{}, ErrLiveHash
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   // 幂等：已有活跃 hold 直接返回。
	   if hid, ok := s.byLeg[req.LegID]; ok {
		  if h := s.shardHolds[hid]; h != nil && h.State == HoldOpen {
			  return h.Copy(), nil
		  }
	  }
	   now := s.now()
	   hold := ShardHold{
		   ID:     uuid.NewString(),
		   PlanID: strings.TrimSpace(req.PlanID),
		   LegID:  strings.TrimSpace(req.LegID),
		  UserID: strings.TrimSpace(req.UserID),
		  Asset:     strings.TrimSpace(req.Asset),
		   Amount:    amt.String(),
		   State:     HoldOpen,
		  CreatedAt: now,
		   UpdatedAt: now,
	   }
	   if req.ExpiryTTL > 0 {
		   exp := now.Add(time.Duration(req.ExpiryTTL) * time.Second)
		  hold.ExpiresAt = &exp
	   }
	   // skip 腿：创建 BroadcastGuardGen 并绑定到 hold。
	   if req.SkipLeg {
		  gen := newGuardGen(hold.PlanID,
			  strings.TrimSpace(req.LiveHash), now)
		  s.remGuards[gen.ID] = gen
		  hold.GuardGenID = gen.ID
	   }
	  s.shardHolds[hold.ID] = &hold
	  s.byLeg[hold.LegID] = hold.ID
	  s.byPlan[hold.PlanID] = append(s.byPlan[hold.PlanID],
		   hold.ID)
	   return hold.Copy(), nil
  }

// ConsumeHold 成交时消耗 hold。成交额必须 <= hold 额。
// 重点：调用前须确保无 open ConserveCase（由调用方或 ReleaseGate 保证）。
// 风险点：并发调用须依赖外层事务/行锁保证幂等性。

  func (s *Service) ConsumeHold(req ConsumeHoldReq) (ShardHold, error) {
	  if s == nil {
		   return ShardHold{}, ErrUnavailable
	  }
	  if strings.TrimSpace(req.HoldID) == "" {
		   return ShardHold{}, ErrNoHold
	   }
	   if strings.TrimSpace(req.FillID) == "" {
		  return ShardHold{}, ErrNoFill
	   }
	   consumeAmt, err := money.Parse(req.Amount)
	   if err != nil || !consumeAmt.IsPositive() {
		   return ShardHold{}, ErrAmountZero
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  // 守恒案件挡写：planID 有 open ConserveCase 时拒绝 ConsumeHold。
	  h, ok := s.shardHolds[req.HoldID]
	  if !ok {
		   return ShardHold{}, ErrNotFound
	  }
	   if err := s.checkNoOpenConserveLocked(h.PlanID); err != nil {
		   return ShardHold{}, err
	  }
	  if h.State != HoldOpen {
		   return h.Copy(), ErrState
	  }
	  holdAmt, err := money.Parse(h.Amount)
	   if err != nil {
		  return h.Copy(), ErrAmount
	  }
	  if consumeAmt.Cmp(holdAmt) > 0 {
		   return h.Copy(), ErrOverConsume
	  }
	   now := s.now()
	  h.State = HoldConsumed
	  h.FillID = strings.TrimSpace(req.FillID)
	   h.ConsumedAt = ptrTime(now)
	  h.UpdatedAt = now
	   return h.Copy(), nil
   }

// ReleaseHold 释放冻结占用（腿失败/剩余释放）。
// 不得释放 consumed hold（已成交不回滚）。
// 不实现 live-hash 广播后自动解冻（由 BroadcastGuardGen 守卫，调用方须在调用前检查）。
  func (s *Service) ReleaseHold(req ReleaseHoldReq) (ShardHold, error) {
	   if s == nil {
		  return ShardHold{}, ErrUnavailable
	   }
	  if strings.TrimSpace(req.HoldID) == "" {
		   return ShardHold{}, ErrNoHold
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  h, ok := s.shardHolds[req.HoldID]
	  if !ok {
		   return ShardHold{}, ErrNotFound
	  }
	  if h.State == HoldConsumed {
		   return h.Copy(), ErrState
	   }
	  if h.State == HoldReleased {
		   return h.Copy(), nil // 幂等
	   }
	   now := s.now()
	   h.State = HoldReleased
	   h.ReleaseReason = strings.TrimSpace(req.Reason)
	   h.ReleasedAt = ptrTime(now)
	   h.UpdatedAt = now
	  return h.Copy(), nil
   }

// checkNoOpenConserveLocked 检查 planID 是否有 open ConserveCase（已持锁调用）。
// 风险点：须在 s.mu.Lock 保护下调用，防止并发检查/写入竞态。
  func (s *Service) checkNoOpenConserveLocked(planID string) error {
	   for _, c := range s.remConserves {
		  if c.PlanID == planID && c.Status == ConserveCaseOpen {
			  return ErrConserveCaseOpen
		   }
	   }
	   return nil
   }

   func validateOpenHold(r OpenHoldReq) error {
	  if strings.TrimSpace(r.PlanID) == "" {
		   return ErrNoPlan
	  }
	  if strings.TrimSpace(r.LegID) == "" {
		   return ErrNoLeg
	   }
	  if strings.TrimSpace(r.UserID) == "" {
		   return ErrNoUser
	  }
	   if strings.TrimSpace(r.Asset) == "" {
		   return ErrNoAsset
	  }
	   return nil
  }

  func ptrTime(t time.Time) *time.Time {
	   return &t
   }

  func (s *HoldService) OpenHold(req OpenHoldReq) (ShardHold, error) {
	   if s == nil {
		  return ShardHold{}, ErrUnavailable
	  }
	   if err := validateOpenHold(req); err != nil {
		  return ShardHold{}, err
	   }
	  amt, err := money.Parse(req.Amount)
	  if err != nil || !amt.IsPositive() {
		   return ShardHold{}, ErrAmountZero
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   // 幂等：已有活跃 hold 直接返回。
	   if hid, ok := s.byLeg[req.LegID]; ok {
		  if h := s.holds[hid]; h != nil && h.State == HoldOpen {
			   return h.Copy(), nil
		  }
	   }
	  now := s.now()
	  hold := ShardHold{
		  ID: uuid.NewString(),
		  PlanID: strings.TrimSpace(req.PlanID),
		   LegID: strings.TrimSpace(req.LegID),
		   UserID: strings.TrimSpace(req.UserID),
		  Asset: strings.TrimSpace(req.Asset),
		  Amount:    amt.String(),
		   State:     HoldOpen,
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	   if req.ExpiryTTL > 0 {
		   exp := now.Add(time.Duration(req.ExpiryTTL) * time.Second)
		   hold.ExpiresAt = &exp
	   }
	   s.holds[hold.ID] = &hold
	  s.byLeg[hold.LegID] = hold.ID
	   s.byPlan[hold.PlanID] = append(s.byPlan[hold.PlanID], hold.ID)
	   return hold.Copy(), nil
   }

// ConsumeHold 成交时消耗 hold。成交额必须 <= hold 额。

   func (s *HoldService) ConsumeHold(req ConsumeHoldReq) (ShardHold, error) {
	  if s == nil {
		   return ShardHold{}, ErrUnavailable
	   }
	   if strings.TrimSpace(req.HoldID) == "" {
		  return ShardHold{}, ErrNoHold
	  }
	  if strings.TrimSpace(req.FillID) == "" {
		   return ShardHold{}, ErrNoFill
	   }
	   consumeAmt, err := money.Parse(req.Amount)
	   if err != nil || !consumeAmt.IsPositive() {
		   return ShardHold{}, ErrAmountZero
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  h, ok := s.holds[req.HoldID]
	   if !ok {
		   return ShardHold{}, ErrHoldNotFound
	  }
	   if h.State != HoldOpen {
		  return h.Copy(), ErrState
	   }
	   holdAmt, err := money.Parse(h.Amount)
	   if err != nil {
		  return h.Copy(),
			   ErrAmountInvalid
	  }
	   if consumeAmt.Cmp(holdAmt) > 0 {
		   return h.Copy(), ErrOverConsume
	  }
	   now := s.now()
	   h.State = HoldConsumed
	   h.FillID = strings.TrimSpace(req.FillID)
	   h.ConsumedAt = ptrTime(now)
	   h.UpdatedAt = now
	  return h.Copy(), nil
  }

// ReleaseHold 释放冻结占用（腿失败/剩余释放）。
// 不得释放 consumed hold（已成交不回滚）。
// 不实现 live-hash 广播后自动解冻（由 LegCanUnfreeze 守卫，调用方须在调用前检查）。
  func (s *HoldService) ReleaseHold(req ReleaseHoldReq) (ShardHold, error) {
	  if s == nil {
		  return ShardHold{}, ErrUnavailable
	  }
	   if strings.TrimSpace(req.HoldID) == "" {
		  return ShardHold{}, ErrNoHold
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  h, ok := s.holds[req.HoldID]
	  if !ok {
		  return ShardHold{}, ErrHoldNotFound
	   }
	  if h.State == HoldConsumed {
		   return h.Copy(), ErrState
	   }
	  if h.State == HoldReleased {
		   return h.Copy(), nil // 幂等
	  }
	   now := s.now()
	  h.State = HoldReleased
	   h.ReleaseReason = strings.TrimSpace(req.Reason)
	  h.ReleasedAt = ptrTime(now)
	  h.UpdatedAt = now
	  return h.Copy(), nil
   }

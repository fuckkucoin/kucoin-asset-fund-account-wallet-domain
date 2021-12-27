package wdpartial

import (
	"encoding/json"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

// ReleasePlanRemainder 释放计划下所有 open holds（失败腿或超出成交部分）。
// 须持有未过期 RemReleaseLease（rem_release_gen 互斥）；
// 与 FillMutexToken 互斥——持有 Fill 锁期间返回 ErrLeaseConflict；
// skip 腿 Hold（GuardGenID 非空）须校验 BroadcastGuardGen 活跃，失效则整体拒绝；
// 幂等：已 released 的 Hold 静默跳过，不报错。
   func (s *Service) ReleasePlanRemainder(req ReleasePlanRemainderReq) (*ReleasePlanRemainderResp, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	  if strings.TrimSpace(req.PlanID) == "" {
		   return nil, ErrNoPlan
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  now := s.now()
	   // 1. 获取并校验租约（rem_release_gen 互斥）。
	  lease, ok := s.remLeases[req.LeaseID]
	  if !ok {
		   return nil, ErrLeaseNotFound
	   }
	  if lease.PlanID != req.PlanID {
		   return nil, fmt.Errorf("wdpartial: lease plan mismatch: lease=%s req=%s", lease.PlanID, req.PlanID)
	   }
	  if err := checkLeaseActiveLocked(lease, now); err != nil {
		  return nil, err
	  }
	   if lease.Generation != req.Generation {
		  return nil, ErrLeaseGenMismatch
	   }
	   // 2. 检查活跃 FillMutexToken（与 Fill 路径互斥）。
	  if s.hasActiveFillMutexLocked(req.PlanID, now) {
		  // 将租约标记为 blocked 并返回冲突错误。
		   lease.Status = LeaseStatusBlocked
		  lease.UpdatedAt = now
		  return nil, ErrLeaseConflict
	  }
	  // 3. 迁移租约 open→releasing。
	   lease.Status = LeaseStatusReleasing
	  t := now
	   lease.ReleasingAt = &t
	   lease.UpdatedAt = now
	  // 4. 对所有 open hold 执行释放（skip 腿先校验 GuardGen）。
	   ids := s.byPlan[req.PlanID]
	   var released []string
	  remainder := money.Zero()
	   for _, hid := range ids {
		  h, exists := s.shardHolds[hid]
		   if !exists || h.State != HoldOpen {
			  continue
		   }
		   // skip 腿：校验 BroadcastGuardGen 活跃性。
		   if h.GuardGenID != "" {
			   gen, gok := s.remGuards[h.GuardGenID]
			   if !gok {
				  // 守卫不存在：整体回滚，重置租约状态。
				   lease.Status = LeaseStatusOpen
				  lease.ReleasingAt = nil
				   lease.UpdatedAt = now
				   return nil, ErrGuardGenNotFound
			   }
			  if err := checkGuardActiveLocked(gen); err != nil {
				  // 守卫失效：回滚租约，返回错误（不得静默跳过）。
				  lease.Status = LeaseStatusOpen
				  lease.ReleasingAt = nil
				  lease.UpdatedAt = now
				   return nil, fmt.Errorf("wdpartial: skip hold %s guard check: %w", h.ID, err)
			  }
		   }
		  // 幂等：已释放跳过（不报错）。
		   if h.State == HoldReleased {
			   continue
		  }
		  h.State = HoldReleased
		  h.ReleaseReason = "plan_remainder_release"
		  h.ReleasedAt = ptrTime(now)
		  h.UpdatedAt = now
		   // skip 腿：消费守卫世代。
		  if h.GuardGenID != "" {
			  gen := s.remGuards[h.GuardGenID]
			   if gen != nil && gen.Status == GuardGenActive {
				   gen.Status = GuardGenConsumed
				  gen.ConsumedAt = ptrTime(now)
				   gen.UpdatedAt = now
			   }
		  }
		   amt, _ := money.ParseBook(h.Amount)
		  remainder, _ = remainder.Add(amt)
		   released = append(released,
			  h.ID)
	  }
	   // 5. 租约迁移 releasing→done。
	   lease.Status = LeaseStatusDone
	  doneAt := now
	  lease.DoneAt = &doneAt
	   lease.UpdatedAt = now
	   return &ReleasePlanRemainderResp{
		  PlanID: req.PlanID,
		  ReleasedHolds: released,
		   Remainder:     remainder.String(),
	   }, nil
  }

// PlanConservation 断言计划级守恒：Σ held = Σ consumed + Σ released + Σ open。
// 失败时必须升级 ConserveCase（禁止仅返回错误后放行后续 ConsumeHold）。
// 风险点：守恒失败后须立即返回 ErrConservationFail，调用方禁止接着调用 ConsumeHold。

  func (s *Service) PlanConservation(planID string) error {
	   if s == nil {
		   return ErrUnavailable
	  }
	   if strings.TrimSpace(planID) == "" {
		   return ErrNoPlan
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   ids := s.byPlan[planID]
	  open := money.Zero()
	  consumed := money.Zero()
	   released := money.Zero()
	   var err error
	   for _, hid := range ids {
		   h, ok := s.shardHolds[hid]
		   if !ok {
			  continue
		  }
		  amt, e := money.ParseBook(h.Amount)
		   if e != nil {
			  return e
		   }
		  switch h.State {
		   case HoldOpen:
			   open, err = open.Add(amt)
		   case HoldConsumed:
			   consumed, err = consumed.Add(amt)
		  case HoldReleased, HoldExpired:
			  released,
				   err = released.Add(amt)
		   }
		  if err != nil {
			   return err
		   }
	  }
	   // 确认成交分录总额 <= consumed holds 总额（fill_total 不得超过已消耗 hold 总额）。
	   fillIds := s.fillsByPlan[planID]
	   fillTotal := money.Zero()
	   for _, fid := range fillIds {
		  f, ok := s.shardFills[fid]
		   if !ok || f.State == FillVoided {
			  continue
		  }
		   amt, e := money.ParseBook(f.Amount)
		   if e != nil {
			  return e
		  }
		  fillTotal, err = fillTotal.Add(amt)
		   if err != nil {
			   return err
		  }
	   }
	   if fillTotal.Cmp(consumed) > 0 {
		   // 守恒失败：必须创建 ConserveCase，禁止仅返回 ErrConserve 后放行。
		   summary := PlanFillSummary{
			  PlanID: planID,
			   TotalConsumed: consumed.String(),
			   TotalOpen: open.String(),
			  TotalReleased: released.String(),
		   }
		  snapBytes, _ := json.Marshal(summary)
		   now := s.now()
		   c := &ConserveCase{
			   ID: uuid.NewString(),
			  PlanID:           planID,
			  Status:           ConserveCaseOpen,
			   ConserveSnapshot: string(snapBytes),
			   FailReason: fmt.Sprintf("fill_total %s > consumed %s",
				  fillTotal, consumed),
			   DetectedAt: now,
			   UpdatedAt: now,
		   }
		   s.remConserves[c.ID] = c
		  return fmt.Errorf("%w: plan %s fill_total %s > consumed %s (case=%s)",
			   ErrConservationFail, planID, fillTotal, consumed, c.ID)
	   }
	   return nil
  }

// HoldSummary 计算计划下 hold 汇总。

  func (s *Service) HoldSummary(planID string) (PlanFillSummary, error) {
	   if s == nil {
		   return PlanFillSummary{},
			  ErrUnavailable
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   ids := s.byPlan[planID]
	  totalHeld := money.Zero()
	  totalConsumed := money.Zero()
	  totalReleased := money.Zero()
	   totalOpen := money.Zero()
	   var err error
	  for _, hid := range ids {
		  h, ok := s.shardHolds[hid]
		  if !ok {
			   continue
		  }
		   amt, e := money.ParseBook(h.Amount)
		  if e != nil {
			   return PlanFillSummary{}, e
		   }
		   totalHeld, err = totalHeld.Add(amt)
		   if err != nil {
			  return PlanFillSummary{}, err
		  }
		  switch h.State {
		   case HoldConsumed:
			  totalConsumed, err = totalConsumed.Add(amt)
		   case HoldReleased, HoldExpired:
			   totalReleased, err = totalReleased.Add(amt)
		   case HoldOpen:
			   totalOpen, err = totalOpen.Add(amt)
		   }
		  if err != nil {
			   return PlanFillSummary{}, err
		   }
	   }
	   fills := s.fillsByPlan[planID]
	   fillCount := len(fills)
	  fillConfirmed := 0
	  for _, fid := range fills {
		   if f, ok := s.shardFills[fid]; ok && f.State == FillConfirmed {
			   fillConfirmed++
		   }
	  }
	   // 检查是否有 open ConserveCase。
	  hasOpenCase := false
	   for _, c := range s.remConserves {
		  if c.PlanID == planID && c.Status == ConserveCaseOpen {
			  hasOpenCase = true
			  break
		  }
	   }
	  return PlanFillSummary{
		   PlanID: planID,
		   TotalHeld: totalHeld.String(),
		  TotalConsumed: totalConsumed.String(),
		  TotalReleased: totalReleased.String(),
		  TotalOpen:     totalOpen.String(),
		   FillCount:     fillCount,
		  FillConfirmed: fillConfirmed,
		  HasOpenConserveCase: hasOpenCase,
	   }, nil
   }

   func (s *HoldService) ReleasePlanRemainder(planID string, skipBroadcasting []string) ([]ShardHold, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	   skipSet := make(map[string]struct{}, len(skipBroadcasting))
	  for _, id := range skipBroadcasting {
		  skipSet[id] = struct{}{}
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  ids := s.byPlan[planID]
	  var released []ShardHold
	  now := s.now()
	  for _, hid := range ids {
		  h, ok := s.holds[hid]
		   if !ok || h.State != HoldOpen {
			   continue
		  }
		   // 广播中腿的 hold 不自动释放（live-hash 守卫）。
		  if _, skip := skipSet[h.LegID]; skip {
			  continue
		   }
		  h.State = HoldReleased
		  h.ReleaseReason = "plan_remainder_release"
		   h.ReleasedAt = ptrTime(now)
		   h.UpdatedAt = now
		   released = append(released,
			  h.Copy())
	  }
	   return released, nil
   }

// PlanConservation 断言计划级守恒：Σ held = Σ consumed + Σ released + Σ open。
// 失败返回 ErrConserveHold 及详细信息。

   func (s *HoldService) PlanConservation(planID string) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  ids := s.byPlan[planID]
	  open := money.Zero()
	   consumed := money.Zero()
	  released := money.Zero()
	  var err error
	   for _, hid := range ids {
		   h, ok := s.holds[hid]
		  if !ok {
			   continue
		   }
		  amt, e := money.ParseBook(h.Amount)
		  if e != nil {
			   return e
		  }
		  switch h.State {
		  case HoldOpen:
			  open, err = open.Add(amt)
		   case HoldConsumed:
			  consumed, err = consumed.Add(amt)
		  case HoldReleased, HoldExpired:
			  released, err = released.Add(amt)
		   }
		  if err != nil {
			   return err
		  }
	   }
	   // 确认成交分录总额 == consumed holds 总额。
	   fillIds := s.fillsByPlan[planID]
	   fillTotal := money.Zero()
	  for _, fid := range fillIds {
		   f, ok := s.fills[fid]
		  if !ok || f.State == FillVoided {
			  continue
		  }
		  amt, e := money.ParseBook(f.Amount)
		  if e != nil {
			   return e
		  }
		  fillTotal, err = fillTotal.Add(amt)
		   if err != nil {
			  return err
		   }
	   }
	   if fillTotal.Cmp(consumed) > 0 {
		   return fmt.Errorf("%w: plan %s fill_total %s > consumed %s",
			  ErrConserveHold, planID, fillTotal, consumed)
	   }
	  return nil
  }

// HoldSummary 计算计划下 hold 汇总。

   func (s *HoldService) HoldSummary(planID string) (PlanFillSummary, error) {
	   if s == nil {
		   return PlanFillSummary{}, ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  ids := s.byPlan[planID]
	   totalHeld := money.Zero()
	  totalConsumed := money.Zero()
	  totalReleased := money.Zero()
	  totalOpen := money.Zero()
	   var err error
	   for _, hid := range ids {
		   h, ok := s.holds[hid]
		   if !ok {
			   continue
		  }
		  amt, e := money.ParseBook(h.Amount)
		  if e != nil {
			  return PlanFillSummary{}, e
		  }
		  totalHeld, err = totalHeld.Add(amt)
		   if err != nil {
			   return PlanFillSummary{}, err
		  }
		   switch h.State {
		   case HoldConsumed:
			   totalConsumed, err = totalConsumed.Add(amt)
		  case HoldReleased, HoldExpired:
			  totalReleased, err = totalReleased.Add(amt)
		  case HoldOpen:
			   totalOpen, err = totalOpen.Add(amt)
		   }
		   if err != nil {
			   return PlanFillSummary{}, err
		  }
	   }
	  fills := s.fillsByPlan[planID]
	  fillCount := len(fills)
	  fillConfirmed := 0
	   for _, fid := range fills {
		  if f, ok := s.fills[fid]; ok && f.State == FillConfirmed {
			   fillConfirmed++
		  }
	  }
	   return PlanFillSummary{
		   PlanID:        planID,
		   TotalHeld:     totalHeld.String(),
		  TotalConsumed: totalConsumed.String(),
		  TotalReleased: totalReleased.String(),
		  TotalOpen:     totalOpen.String(),
		  FillCount: fillCount,
		   FillConfirmed: fillConfirmed,
	  }, nil
   }

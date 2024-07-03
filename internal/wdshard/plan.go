package wdshard

import (
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

// Preview 预览拆分方案，不落库。
  func (s *Service) Preview(req PreviewReq) (PreviewResp, error) {
	  if s == nil {
		   return PreviewResp{}, ErrUnavailable
	   }
	  if strings.TrimSpace(req.Asset) == "" {
		  return PreviewResp{}, ErrNoAsset
	  }
	  if strings.TrimSpace(req.Chain) == "" {
		  return PreviewResp{}, ErrNoChain
	   }
	  total, err := money.Parse(req.TotalAmount)
	   if err != nil || !total.IsPositive() {
		   return PreviewResp{},
			   ErrAmountZero
	  }
	   pid := strings.TrimSpace(req.PolicyID)
	  if pid == "" {
		   pid = DefaultPolicyID
	  }
	   s.mu.Lock()
	   pol, ok := s.policies[pid]
	   s.mu.Unlock()
	   if !ok {
		  return PreviewResp{}, ErrPolicyNotFound
	   }
	  sr, err := SplitAmount(total, pol, req.HotAvailable)
	   if err != nil {
		   return PreviewResp{}, err
	   }
	  amts := make([]string, len(sr.Amounts))
	  for i, a := range sr.Amounts {
		  amts[i] = a.String()
	   }
	  return PreviewResp{
		  PolicyID: pid,
		  LegCount: sr.LegCount,
		  Amounts:     amts,
		  TotalAmount: total.String(),
		  Warning:     sr.Warning,
	   }, nil
  }

// Create 创建草稿计划，幂等。返回 plan 与 legs。

  func (s *Service) Create(req CreatePlanReq) (ShardPlan, []ShardLeg, error) {
	  if s == nil {
		   return ShardPlan{}, nil, ErrUnavailable
	  }
	   if err := validateCreateReq(req); err != nil {
		   return ShardPlan{}, nil, err
	  }
	   s.mu.Lock()
	   if s.halt {
		   s.mu.Unlock()
		  return ShardPlan{}, nil, ErrHalt
	  }
	  idem := strings.TrimSpace(req.IdempotencyKey)
	  if idem != "" {
		   if pid, ok := s.byIdem[idem]; ok {
			  plan := s.plans[pid].Copy()
			  legs := s.listLegs(pid)
			   s.mu.Unlock()
			  return plan, legs, nil
		   }
	   }
	  pol, ok := s.policies[strings.TrimSpace(req.PolicyID)]
	   if !ok {
		  s.mu.Unlock()
		  return ShardPlan{}, nil, ErrPolicyNotFound
	   }
	  s.mu.Unlock()
	   total, err := money.Parse(req.TotalAmount)
	  if err != nil {
		  return ShardPlan{},
			   nil, ErrAmount
	  }
	  sr, err := SplitAmount(total, pol, req.HotAvailable)
	   if err != nil {
		   return ShardPlan{}, nil, err
	  }
	   now := s.now()
	   planID := uuid.NewString()
	   plan := ShardPlan{
		   ID: planID,
		   UserID: strings.TrimSpace(req.UserID),
		   Asset:  strings.TrimSpace(req.Asset),
		   Chain:  strings.TrimSpace(req.Chain),
		   ToAddress: strings.TrimSpace(req.ToAddress),
		  TotalAmount: total.String(),
		  PolicyID:    pol.ID,
		   BatchID:     strings.TrimSpace(req.BatchID),
		  WithdrawID: strings.TrimSpace(req.WithdrawID),
		  State:      PlanDraft,
		  LegCount:   sr.LegCount,
		   FilledAmount: money.Zero().String(),
		  InflightAmount: money.Zero().String(),
		   RemainAmount:   total.String(),
		   Actor:          strings.TrimSpace(req.Actor),
		  IdempotencyKey: idem,
		  CreatedAt:      now,
		   UpdatedAt:      now,
	   }
	  legs := make([]ShardLeg, sr.LegCount)
	   legIDs := make([]string, sr.LegCount)
	  for i, a := range sr.Amounts {
		  legID := uuid.NewString()
		   legs[i] = ShardLeg{
			   ID: legID,
			  PlanID: planID,
			  Seq:    i + 1,
			   Amount: a.String(),
			   State:     LegPending,
			  CreatedAt: now,
			  UpdatedAt: now,
		   }
		   legIDs[i] = legID
	   }
	  s.mu.Lock()
	  // 二次检查幂等（锁区间可能被并发抢先）。
	   if idem != "" {
		   if pid, ok := s.byIdem[idem]; ok {
			  p := s.plans[pid].Copy()
			   ll := s.listLegs(pid)
			  s.mu.Unlock()
			   return p, ll, nil
		   }
	   }
	   s.plans[planID] = &plan
	  for i := range legs {
		  cp := legs[i]
		   s.legs[cp.ID] = &cp
	   }
	   s.byPlan[planID] = legIDs
	   if idem != "" {
		   s.byIdem[idem] = planID
	   }
	   s.mu.Unlock()
	   return plan, legs, nil
   }

// GetPlan 查询计划。
   func (s *Service) GetPlan(id string) (ShardPlan, error) {
	   if s == nil {
		  return ShardPlan{}, ErrUnavailable
	  }
	  s.mu.Lock()
	   p, ok := s.getPlan(strings.TrimSpace(id))
	  s.mu.Unlock()
	  if !ok {
		   return ShardPlan{}, ErrNotFound
	   }
	  return p, nil
  }

// GetLegs 查询计划下所有腿。
  func (s *Service) GetLegs(planID string) ([]ShardLeg, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	   s.mu.Lock()
	   if _, ok := s.plans[strings.TrimSpace(planID)]; !ok {
		  s.mu.Unlock()
		   return nil, ErrNotFound
	   }
	  legs := s.listLegs(strings.TrimSpace(planID))
	   s.mu.Unlock()
	  return legs, nil
  }

// Approve 把计划从 draft 推进到 approved_exec。
// 调用方（审批模块或运维）在审批通过后调用此接口，本包只处理执行态。

   func (s *Service) Approve(planID, actor string) (ShardPlan, error) {
	  if s == nil {
		   return ShardPlan{}, ErrUnavailable
	  }
	   if strings.TrimSpace(actor) == "" {
		  return ShardPlan{}, ErrActor
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   p, ok := s.plans[strings.TrimSpace(planID)]
	  if !ok {
		   return ShardPlan{}, ErrNotFound
	   }
	  next, err := PlanTransition(p.State, PlanApprovedExec)
	   if err != nil {
		   return p.Copy(), err
	   }
	  p.State = next
	  p.UpdatedAt = s.now()
	   return p.Copy(), nil
  }

// Start 把计划从 approved_exec 推进到 running。

   func (s *Service) Start(planID, actor string) (ShardPlan, error) {
	   if s == nil {
		   return ShardPlan{}, ErrUnavailable
	   }
	  if strings.TrimSpace(actor) == "" {
		  return ShardPlan{}, ErrActor
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  p, ok := s.plans[strings.TrimSpace(planID)]
	   if !ok {
		  return ShardPlan{}, ErrNotFound
	  }
	  next, err := PlanTransition(p.State, PlanRunning)
	   if err != nil {
		   return p.Copy(), err
	  }
	   p.State = next
	  p.UpdatedAt = s.now()
	  return p.Copy(), nil
   }

// Abort 中止计划（任意非终态）。

   func (s *Service) Abort(planID, actor, reason string) (ShardPlan, error) {
	   if s == nil {
		   return ShardPlan{}, ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  p, ok := s.plans[strings.TrimSpace(planID)]
	   if !ok {
		  return ShardPlan{}, ErrNotFound
	  }
	  if !PlanCanAbort(p.State) {
		   return p.Copy(), fmt.Errorf("%w: already terminal %s", ErrState, p.State)
	   }
	  next, err := PlanTransition(p.State, PlanAborted)
	   if err != nil {
		   return p.Copy(), err
	  }
	   p.State = next
	  p.AbortReason = strings.TrimSpace(reason)
	   p.UpdatedAt = s.now()
	  cp := p.Copy()
	  return cp, nil
  }

// Pause 暂停（running→已暂停；用 AbortReason 字段存暂停原因，State 不变）。
// 暂停不改状态机态，只置 PauseReason；workers 检查 PauseReason 自行停跳。
  func (s *Service) Pause(req PauseReq) (ShardPlan, error) {
	   if s == nil {
		   return ShardPlan{}, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   p, ok := s.plans[strings.TrimSpace(req.PlanID)]
	   if !ok {
		   return ShardPlan{}, ErrNotFound
	   }
	  if p.State != PlanRunning {
		  return p.Copy(), fmt.Errorf("%w: can only pause running plans", ErrState)
	   }
	   p.PauseReason = strings.TrimSpace(req.Reason)
	   if p.PauseReason == "" {
		   p.PauseReason = "ops pause"
	   }
	  p.UpdatedAt = s.now()
	   return p.Copy(), nil
  }

// Resume 续跑（清除 PauseReason）。
  func (s *Service) Resume(req ResumeReq) (ShardPlan, error) {
	  if s == nil {
		  return ShardPlan{}, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  p, ok := s.plans[strings.TrimSpace(req.PlanID)]
	   if !ok {
		   return ShardPlan{}, ErrNotFound
	  }
	   if p.State != PlanRunning {
		  return p.Copy(), fmt.Errorf("%w: can only resume running plans", ErrState)
	   }
	   p.PauseReason = ""
	   p.UpdatedAt = s.now()
	   return p.Copy(), nil
  }

// ListPlans 按用户查询计划（简单全量扫描，可替换为 SQL 范围查询）。

   func (s *Service) ListPlans(userID string, limit int) []ShardPlan {
	   if s == nil {
		   return nil
	   }
	   if limit <= 0 {
		  limit = 50
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]ShardPlan,
		   0, limit)
	  for _, p := range s.plans {
		  if userID != "" && p.UserID != userID {
			  continue
		   }
		  out = append(out, p.Copy())
		  if len(out) >= limit {
			   break
		  }
	  }
	  return out
  }

  func validateCreateReq(r CreatePlanReq) error {
	   if strings.TrimSpace(r.UserID) == "" {
		   return ErrNoUser
	   }
	  if strings.TrimSpace(r.Asset) == "" {
		  return ErrNoAsset
	   }
	   if strings.TrimSpace(r.Chain) == "" {
		  return ErrNoChain
	   }
	   if strings.TrimSpace(r.ToAddress) == "" {
		  return ErrNoAddress
	   }
	  if strings.TrimSpace(r.PolicyID) == "" {
		  r.PolicyID = DefaultPolicyID
	   }
	  amt, err := money.Parse(r.TotalAmount)
	   if err != nil {
		  return ErrAmount
	   }
	  if !amt.IsPositive() {
		   return ErrAmountZero
	   }
	   if strings.TrimSpace(r.Actor) == "" {
		  return ErrActor
	   }
	   return nil
   }

// nowPtr 辅助：时间指针。
  func nowPtr(t time.Time) *time.Time {
	   return &t
   }

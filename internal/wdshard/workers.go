package wdshard

import (
	"context"
	"strings"
	"time"
)

// BroadcastFunc 由外部注入的广播意图提交接口（对应 withdraw.Service.Request 或 wdbatch）。
type BroadcastFunc func(ctx context.Context, planID, legID, userID, asset, chain, toAddress, amount, idemKey string) (withdrawID, broadcastRef string, err error)

// BroadcastStatusFunc 查询已提交广播的状态。返回 "filled"/"failed"/"pending"。
type BroadcastStatusFunc func(ctx context.Context, legID, withdrawID string) (status, failReason string, err error)

// HoldFunc 为分片腿冻结资金占用，返回 holdID。

type HoldFunc func(ctx context.Context,
	   planID, legID, userID, asset, amount string) (holdID string, err error)

// ReleaseHoldFunc 释放指定 hold。
type ReleaseHoldFunc func(ctx context.Context,
	  holdID, reason string) error

// WorkerConfig 注入的依赖集合。
type WorkerConfig struct {
	Broadcast     BroadcastFunc
	BroadcastStat BroadcastStatusFunc
	Hold          HoldFunc
	ReleaseHold   ReleaseHoldFunc
	// TickInterval Worker 轮询间隔，默认 5s。
	TickInterval time.Duration
}

// RunWorkers 启动四个 Worker goroutine，ctx 取消时全部退出。

  func (s *Service) RunWorkers(ctx context.Context, cfg WorkerConfig) {
	   interval := cfg.TickInterval
	   if interval <= 0 {
		   interval = 5 * time.Second
	  }
	   go s.runSplitWorker(ctx,
		   cfg, interval)
	   go s.runAdvanceWorker(ctx, cfg, interval)
	   go s.runAlignWorker(ctx, cfg, interval)
	  go s.runReleaseWorker(ctx, cfg, interval)
  }

// runSplitWorker 扫描 approved_exec 计划，把它推进到 running。
// 注意：Create 由外部（HTTP API 或 wdbatch hook）完成；此 worker 只负责 Start。

  func (s *Service) runSplitWorker(ctx context.Context, cfg WorkerConfig, interval time.Duration) {
	   tick := time.NewTicker(interval)
	   defer tick.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			  return
		   case <-tick.C:
			   s.tickSplit(ctx)
		  }
	  }
  }

   func (s *Service) tickSplit(ctx context.Context) {
	  s.mu.Lock()
	   var targets []ShardPlan
	  for _, p := range s.plans {
		  if p.State == PlanApprovedExec {
			   targets = append(targets, p.Copy())
		   }
	   }
	   s.mu.Unlock()
	   for _, p := range targets {
		   if _, err := s.Start(p.ID, "split_worker"); err != nil {
			   // 非法跃迁或已处于 running，忽略。
			  _ = err
		   }
	  }
  }

// runAdvanceWorker 扫描 pending legs（属于 running 计划），执行冻结并提交广播意图。

   func (s *Service) runAdvanceWorker(ctx context.Context, cfg WorkerConfig, interval time.Duration) {
	   tick := time.NewTicker(interval)
	  defer tick.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			  return
		  case <-tick.C:
			  s.tickAdvance(ctx, cfg)
		   }
	  }
   }

  func (s *Service) tickAdvance(ctx context.Context, cfg WorkerConfig) {
	   s.mu.Lock()
	   var targets []ShardLeg
	   for _, leg := range s.legs {
		   if leg.State != LegPending {
			   continue
		  }
		  plan := s.plans[leg.PlanID]
		   if plan == nil || plan.State != PlanRunning {
			  continue
		   }
		   if strings.TrimSpace(plan.PauseReason) != "" {
			  continue
		  }
		   targets = append(targets, leg.Copy())
	  }
	  s.mu.Unlock()
	  for _, leg := range targets {
		  s.advanceLegOnce(ctx, cfg, leg)
	   }
   }

  func (s *Service) advanceLegOnce(ctx context.Context, cfg WorkerConfig, leg ShardLeg) {
	   s.mu.Lock()
	  plan, ok := s.plans[leg.PlanID]
	  if !ok {
		  s.mu.Unlock()
		   return
	   }
	   planCp := plan.Copy()
	  s.mu.Unlock()
	   // 1. 冻结资金占用。
	  holdID := ""
	   if cfg.Hold != nil {
		  var err error
		   holdID,
			  err = cfg.Hold(ctx, leg.PlanID, leg.ID, planCp.UserID, planCp.Asset, leg.Amount)
		   if err != nil {
			  _, _, _ = s.AdvanceLeg(LegAdvanceReq{
				   LegID: leg.ID,
				   To:         LegFailed,
				   FailReason: "hold: " + err.Error(),
				   Actor:      "advance_worker",
			  })
			  s.mu.Lock()
			  updatedLeg, _ := s.getLeg(leg.ID)
			   s.mu.Unlock()
			  s.emitLegFailed(updatedLeg, planCp)
			  return
		  }
	   }
	   // 2. 把腿推到 frozen。
	   _, _, err := s.AdvanceLeg(LegAdvanceReq{
		   LegID: leg.ID,
		   To:     LegFrozen,
		   HoldID: holdID,
		  Actor:  "advance_worker",
	   })
	   if err != nil {
		  return
	  }
	   // 3. 提交广播意图。
	   if cfg.Broadcast == nil {
		   return
	   }
	  idemKey := leg.PlanID + ":" + leg.ID
	   withdrawID, broadcastRef, bErr := cfg.Broadcast(
		   ctx, leg.PlanID, leg.ID, planCp.UserID, planCp.Asset,
		  planCp.Chain, planCp.ToAddress, leg.Amount, idemKey,
	  )
	   if bErr != nil {
		  _, _, _ = s.AdvanceLeg(LegAdvanceReq{
			  LegID: leg.ID,
			  To:         LegFailed,
			  FailReason: "broadcast: " + bErr.Error(),
			  Actor:      "advance_worker",
		   })
		   s.mu.Lock()
		   updatedLeg, _ := s.getLeg(leg.ID)
		  s.mu.Unlock()
		  s.emitLegFailed(updatedLeg,
			  planCp)
		  return
	   }
	   _, _, _ = s.AdvanceLeg(LegAdvanceReq{
		  LegID:        leg.ID,
		  To:           LegBroadcasting,
		   WithdrawID:   withdrawID,
		  BroadcastRef: broadcastRef,
		  Actor:        "advance_worker",
	   })
   }

// runAlignWorker 扫描 broadcasting legs，查询广播结果推进到 filled/failed。

  func (s *Service) runAlignWorker(ctx context.Context, cfg WorkerConfig, interval time.Duration) {
	  tick := time.NewTicker(interval)
	   defer tick.Stop()
	   for {
		  select {
		   case <-ctx.Done():
			  return
		  case <-tick.C:
			  s.tickAlign(ctx, cfg)
		   }
	  }
  }

   func (s *Service) tickAlign(ctx context.Context, cfg WorkerConfig) {
	  if cfg.BroadcastStat == nil {
		  return
	   }
	   s.mu.Lock()
	   var targets []ShardLeg
	   for _, leg := range s.legs {
		   if leg.State == LegBroadcasting {
			  targets = append(targets,
				   leg.Copy())
		   }
	  }
	  s.mu.Unlock()
	   for _, leg := range targets {
		   status, failReason, err := cfg.BroadcastStat(ctx, leg.ID, leg.WithdrawID)
		   if err != nil || status == "pending" {
			  continue
		  }
		  s.mu.Lock()
		   planCp,
			  _ := s.getPlan(leg.PlanID)
		   s.mu.Unlock()
		  switch status {
		  case "filled":
			   _, _, _ = s.AdvanceLeg(LegAdvanceReq{
				  LegID: leg.ID,
				  To:    LegFilled,
				   Actor: "align_worker",
			   })
		  case "failed":
			   _, _, _ = s.AdvanceLeg(LegAdvanceReq{
				   LegID:      leg.ID,
				   To:         LegFailed,
				  FailReason: failReason,
				  Actor: "align_worker",
			  })
			  s.mu.Lock()
			   updatedLeg, _ := s.getLeg(leg.ID)
			  s.mu.Unlock()
			   s.emitLegFailed(updatedLeg, planCp)
		   }
	  }
  }

// runReleaseWorker 扫描 failed legs，在 AllowPartial 策略下释放对应冻结占用。
// 不释放 broadcasting（live-hash 守卫）。
  func (s *Service) runReleaseWorker(ctx context.Context, cfg WorkerConfig, interval time.Duration) {
	   tick := time.NewTicker(interval)
	  defer tick.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			   return
		  case <-tick.C:
			   s.tickRelease(ctx, cfg)
		  }
	  }
   }

  func (s *Service) tickRelease(ctx context.Context, cfg WorkerConfig) {
	  if cfg.ReleaseHold == nil {
		  return
	   }
	   s.mu.Lock()
	   var targets []ShardLeg
	   for _, leg := range s.legs {
		  if leg.State != LegFailed {
			  continue
		  }
		   // LegCanUnfreeze 确认广播后不自动解冻（failed 在 broadcasting 后属于事故流）。
		  if !LegCanUnfreeze(leg.State) {
			  continue
		  }
		  if leg.HoldID == "" {
			  continue
		   }
		  targets = append(targets, leg.Copy())
	  }
	  s.mu.Unlock()
	  for _, leg := range targets {
		  pol, _ := s.policyForLeg(leg)
		  if !pol.AllowPartial {
			   continue
		  }
		  reason := "shard_leg_failed:" + leg.ID
		   if rerr := cfg.ReleaseHold(ctx, leg.HoldID, reason); rerr != nil {
			  _ = rerr
		  }
	  }
   }

// policyForLeg 找到腿所属计划的策略。
   func (s *Service) policyForLeg(leg ShardLeg) (PlanPolicy, bool) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  plan, ok := s.plans[leg.PlanID]
	   if !ok {
		   return PlanPolicy{}, false
	   }
	  pol, ok2 := s.policies[plan.PolicyID]
	  if !ok2 {
		   return DefaultPolicy(), true
	  }
	  return pol, ok2
  }

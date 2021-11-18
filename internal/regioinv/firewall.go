package regioinv

import (
	"context"
	"errors"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

// CheckHotWithdraw 是热取现前的区级防火墙。
// 返回错误时由调用方决定是否中断出金意图；本函数不广播也不改账。
// 判断顺序（所有路径互斥，失败先报最高先级）：
// 1. 区已 halt → ErrRegionHalted
// 2. 防火墙 blocked → ErrFirewallBlocked
// 3. 跨区取热且策略禁止 → ErrCrossRegionHot
// 4. 本区热水位缺失 → ErrWatermark（失败即关）
// 5. 取出后热可用低于低水位 → ErrHotBelowWatermark

  func (svc *Service) CheckHotWithdraw(ctx context.Context, req HotWithdraw) error {
	  regionID := req.RegionID
	   if regionID == "" {
		   r, err := svc.st.FindRegion(ctx, req.RegionCode)
		   if err != nil {
			   return err
		  }
		  regionID = r.ID
	  }
	  region, err := svc.st.GetRegion(ctx, regionID)
	  if err != nil {
		   return err
	  }
	   // 1. 区 halt 先于其他防火墙规则
	   if region.Halted {
		  return ErrRegionHalted
	   }
	  // 2. 防火墙阻断
	   if region.Firewall == FWBlocked {
		  return ErrFirewallBlocked
	  }
	   // 3. 跨区取热
	   if req.CrossRegion {
		   pol, err := svc.st.FindPolicy(ctx, regionID)
		   if err != nil && !errors.Is(err, ErrNotFound) {
			   return err
		  }
		  if pol.BlockCrossHot || (region.Firewall == FWDegraded) {
			   return ErrCrossRegionHot
		   }
	  }
	  // 4. 水位缺失 = 失败即关
	   wm, err := svc.st.FindMark(ctx, regionID, TierHot, req.Asset, req.Chain)
	   if err != nil {
		   if errors.Is(err, ErrNotFound) {
			  return ErrWatermark
		   }
		  return err
	   }
	  if !wm.Enabled {
		  return ErrWatermark
	  }
	  // 5. 取出后是否低于低水位
	   inv, err := svc.st.FindInv(ctx, regionID, TierHot, req.Asset, req.Chain)
	  if err != nil {
		  return ErrWatermark
	  }
	  avail, err := bookOf(inv.Available)
	   if err != nil {
		  return ErrWatermark
	   }
	   amt, err := money.Parse(req.Amount)
	   if err != nil {
		  return ErrInvalid
	  }
	   after, err := avail.Sub(amt)
	  if err != nil {
		  return ErrInsufficient
	   }
	  low, err := bookOf(wm.Low)
	   if err != nil {
		  return ErrWatermark
	  }
	   if after.Cmp(low) < 0 {
		   return ErrHotBelowWatermark
	   }
	   return nil
  }

// CheckRebalance 验证一笔跨区再平衡的合法性（仅判定，不落计划）。
// 判断顺序：
// 1. 源或目标区 halt / blocked
// 2. 水位方向校验：src 必须高于 target（有盈余），dst 必须低于 target（有缺口）
// 3. 窗口
// 4. 日配额
// 5. MinLeg
   func (svc *Service) CheckRebalance(ctx context.Context, t Transfer, now time.Time) error {
	   if t.SrcRegionID == t.DstRegionID {
		   return ErrSameRegion
	   }
	   srcR, err := svc.st.GetRegion(ctx, t.SrcRegionID)
	   if err != nil {
		  return err
	   }
	  dstR, err := svc.st.GetRegion(ctx, t.DstRegionID)
	  if err != nil {
		   return err
	  }
	   if srcR.Halted {
		   return ErrRegionHalted
	   }
	   if dstR.Halted {
		  return ErrRegionHalted
	  }
	  if srcR.Firewall == FWBlocked || dstR.Firewall == FWBlocked {
		  return ErrFirewallBlocked
	   }
	  // 策略校验：禁止错误方向
	   pol, _ := svc.st.FindPolicy(ctx, t.SrcRegionID)
	   if pol.BlockWrongRebal {
		   if err := svc.checkDirection(ctx, t); err != nil {
			   return err
		  }
	  }
	  // 水位约束
	   srcMark, err := svc.st.FindMark(ctx, t.SrcRegionID, t.Tier, t.Asset, t.Chain)
	  if err != nil {
		   return ErrWatermark
	   }
	  if !srcMark.Enabled {
		   return ErrWatermark
	   }
	  dstMark, err := svc.st.FindMark(ctx, t.DstRegionID, t.Tier, t.Asset, t.Chain)
	   if err != nil {
		   return ErrWatermark
	  }
	  if !dstMark.Enabled {
		  return ErrWatermark
	  }
	  // 窗口
	   if !InWindow(srcMark.WindowStart, srcMark.WindowEnd, now) {
		  return ErrOutsideWindow
	  }
	  if !InWindow(dstMark.WindowStart, dstMark.WindowEnd, now) {
		  return ErrOutsideWindow
	   }
	  // MinLeg
	   amt, err := money.Parse(t.Amount)
	  if err != nil {
		  return ErrInvalid
	  }
	  if minLeg, err := bookOf(srcMark.MinLeg); err == nil && minLeg.IsPositive() {
		  if amt.Cmp(minLeg) < 0 {
			   return ErrMinLeg
		   }
	   }
	  // 日配额
	   if cap, err := bookOf(srcMark.DailyCap); err == nil && cap.IsPositive() {
		   used, _ := bookOf(srcMark.Used)
		   day := dayOf(now)
		  if srcMark.UsedDay != day {
			  used = money.Zero()
		   }
		  remain, err := cap.Sub(used)
		   if err != nil || remain.Cmp(amt) < 0 {
			  return ErrQuota
		  }
	  }
	   return nil
   }

  func (svc *Service) checkDirection(ctx context.Context, t Transfer) error {
	   srcInv, err := svc.st.FindInv(ctx, t.SrcRegionID, t.Tier, t.Asset, t.Chain)
	   if err != nil {
		  return ErrWatermark
	   }
	  dstInv, err := svc.st.FindInv(ctx, t.DstRegionID, t.Tier, t.Asset, t.Chain)
	  if err != nil {
		   return ErrWatermark
	   }
	  srcMark, err := svc.st.FindMark(ctx, t.SrcRegionID, t.Tier, t.Asset, t.Chain)
	  if err != nil {
		  return ErrWatermark
	   }
	   dstMark, err := svc.st.FindMark(ctx, t.DstRegionID, t.Tier, t.Asset, t.Chain)
	  if err != nil {
		  return ErrWatermark
	  }
	   // src 可用需 > 目标水位（有盈余可搬出）
	   srcAvail, _ := bookOf(srcInv.Available)
	  srcTarget, _ := bookOf(srcMark.Target)
	   if srcAvail.Cmp(srcTarget) <= 0 {
		   return ErrWrongRebalance
	   }
	  // dst 可用需 < 目标水位（有缺口接收）
	   dstAvail, _ := bookOf(dstInv.Available)
	   dstTarget, _ := bookOf(dstMark.Target)
	  if dstAvail.Cmp(dstTarget) >= 0 {
		   return ErrWrongRebalance
	  }
	  return nil
   }

// EngageHalt 区级隔离 Engage（不解除全局 custodyhalt，不改证明链）。

   func (svc *Service) EngageHalt(ctx context.Context, regionCode, actor, reason string, now time.Time) (Region, error) {
	  if actor == "" {
		  return Region{}, ErrActor
	   }
	  r, err := svc.st.FindRegion(ctx, regionCode)
	   if err != nil {
		  return Region{}, err
	   }
	   r.Halted = true
	  r.HaltActor = actor
	  r.HaltReason = reason
	   r.HaltAt = now
	  r.Firewall = FWBlocked
	  r.FirewallReason = "halted by " + actor
	  r.UpdatedAt = now
	  if err := svc.st.SaveRegion(ctx, r); err != nil {
		   return Region{}, err
	   }
	   return r, nil
  }

// ClearHalt 清除区级隔离。不触碰全局 custodyhalt，不自动解除其他停提。

  func (svc *Service) ClearHalt(ctx context.Context, regionCode, actor, reason string, now time.Time) (ClearResult, error) {
	   if actor == "" {
		   return ClearResult{}, ErrActor
	   }
	   r, err := svc.st.FindRegion(ctx, regionCode)
	   if err != nil {
		  return ClearResult{}, err
	   }
	   if !r.Halted {
		   return ClearResult{}, ErrNotHalted
	   }
	   r.Halted = false
	   r.HaltActor = ""
	  r.HaltReason = ""
	  r.HaltAt = time.Time{}
	   r.Firewall = FWOpen
	   r.FirewallReason = "cleared by " + actor
	   r.UpdatedAt = now
	   if err := svc.st.SaveRegion(ctx, r); err != nil {
		   return ClearResult{}, err
	   }
	   return ClearResult{Region: r, CustodyHaltUntouched: true}, nil
   }

// ApplyProbe 把故障探测结论写入区防火墙状态。
// 不 Engage halt；人工 halt 需要显式调用 EngageHalt。

  func (svc *Service) ApplyProbe(ctx context.Context, rep ProbeReport, now time.Time) (Region, error) {
	  r,
		   err := svc.st.FindRegion(ctx, rep.RegionCode)
	  if err != nil {
		  return Region{}, err
	  }
	  if r.Halted {
		  // 区已 halt，探针结论不覆盖，人工 Clear 才能打开
		  return r, nil
	  }
	   pol, _ := svc.st.FindPolicy(ctx, r.ID)
	   target := FWOpen
	  if !rep.Ready {
		   target = pol.FailTo
		   if target == "" {
			  target = FWBlocked
		  }
	  }
	   if r.Firewall == target && r.Ready == rep.Ready {
		   return r, nil
	   }
	  r.Firewall = target
	  r.FirewallReason = rep.Detail
	   r.Ready = rep.Ready
	  r.ReadyDetail = rep.Detail
	   r.UpdatedAt = now
	  if err := svc.st.SaveRegion(ctx, r); err != nil {
		   return Region{}, err
	  }
	   return r, nil
   }

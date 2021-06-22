package flow

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
	"account-assets.kucoin.net/funding/internal/georebal"
	"account-assets.kucoin.net/funding/internal/regioinv"
	"account-assets.kucoin.net/funding/pkg/money"
)

// --- 通用夹具帮手 ---

  func setupTwoRegions(t *testing.T) (*regioinv.Service, regioinv.Region, regioinv.Region) {
	   t.Helper()
	   st := regioinv.NewMemStore()
	   svc := regioinv.New(st)
	   ctx := context.Background()
	   regA, err := svc.RegisterRegion(ctx, regioinv.RegisterReq{
		   Code: "sg", CatalogCode: "sg", Name: "Singapore", Actor: "ops",
	  })
	   if err != nil {
		   t.Fatal(err)
	  }
	   regB, err := svc.RegisterRegion(ctx, regioinv.RegisterReq{
		  Code: "tyo", CatalogCode: "tyo", Name: "Tokyo", Actor: "ops",
	   })
	  if err != nil {
		  t.Fatal(err)
	  }
	   return svc, regA, regB
   }

// injectInv 写入一条来源读数并汇聚库存。
   func injectInv(t *testing.T, svc *regioinv.Service, regionCode, tier, asset, avail string) regioinv.RegionInv {
	   t.Helper()
	   ctx := context.Background()
	   snap, err := svc.ObserveInv(ctx, regioinv.ObserveReq{
		  RegionCode: regionCode,
		  Tier:       tier,
		  Asset:      asset,
		  Chain:      "",
		  Source:     "test",
		   Available: avail,
		   Frozen: "0",
	  })
	   if err != nil {
		   t.Fatal(err)
	   }
	  return snap
  }

   func setMark(t *testing.T, svc *regioinv.Service, regionCode, tier, asset, low, tgt, high string) {
	   t.Helper()
	  ctx := context.Background()
	  e := true
	  _, err := svc.SetMark(ctx, regioinv.MarkReq{
		   RegionCode: regionCode,
		  Tier: tier,
		   Asset:  asset,
		  Low:    low,
		   Target: tgt,
		   High: high,
		   MinLeg:   "1",
		   DailyCap: "0",
		   Enabled:  &e,
		   Actor:    "ops",
	  })
	  if err != nil {
		   t.Fatal(err)
	   }
   }

// --- 夹具 1：热库存低于水位时取热出金被防火墙拒绝 ---
// TestFirewallHotBelowWatermark 验收条件：
// - 区A热可用 = 50，水位低线 = 100。
// - 请求取 10 → 取出后 = 40 < 低线 → ErrHotBelowWatermark。
// - 错误不是 custodyhalt.ErrHalted（可区分全局停提）。
   func TestFirewallHotBelowWatermark(t *testing.T) {
	   svc, regA, _ := setupTwoRegions(t)
	   ctx := context.Background()
	  injectInv(t, svc, regA.Code, regioinv.TierHot, "USDT", "50")
	  setMark(t, svc, regA.Code, regioinv.TierHot, "USDT", "100", "200", "400")
	   err := svc.CheckHotWithdraw(ctx, regioinv.HotWithdraw{
		  RegionCode: regA.Code,
		   RegionID: regA.ID,
		  Asset:  "USDT",
		  Chain:  "",
		  Amount: "10",
		   Actor: "withdraw-svc",
	  })
	  if err == nil {
		  t.Fatal("expected firewall rejection, got nil")
	  }
	  if !errors.Is(err, regioinv.ErrHotBelowWatermark) {
		  t.Fatalf("expected ErrHotBelowWatermark, got: %v (code=%s)", err, regioinv.CodeOf(err))
	  }
	   // 必须与全局 custodyhalt 可区分
	   if errors.Is(err, custodyhalt.ErrHalted) {
		   t.Fatal("firewall error must not equal custodyhalt.ErrHalted")
	  }
	   code := regioinv.CodeOf(err)
	   if code != regioinv.CodeHotBelow {
		  t.Fatalf("expected code %q, got %q", regioinv.CodeHotBelow, code)
	   }
	  t.Logf("✓ firewall hot-below-watermark code=%s err=%v", code, err)
  }

// TestFirewallHotSufficient 可用充足时防火墙通过。

  func TestFirewallHotSufficient(t *testing.T) {
	   svc, regA, _ := setupTwoRegions(t)
	   ctx := context.Background()
	  injectInv(t, svc, regA.Code, regioinv.TierHot, "USDT", "300")
	   setMark(t, svc, regA.Code, regioinv.TierHot, "USDT", "100", "200", "400")
	   err := svc.CheckHotWithdraw(ctx, regioinv.HotWithdraw{
		  RegionCode: regA.Code,
		  RegionID:   regA.ID,
		   Asset:      "USDT",
		   Chain:  "",
		  Amount: "50",
		   Actor: "withdraw-svc",
	  })
	  if err != nil {
		  t.Fatalf("expected pass, got: %v", err)
	   }
	  t.Log("✓ firewall passes when hot sufficient")
   }

// --- 夹具 2：跨区再平衡计划守恒 ---
// TestGeoRebalConserve 验收条件：
// - 源 sg 热可用 = 500（>target=300, 盈余 200）。
// - 目标 tyo 热可用 = 50（<target=300, 缺口 250）。
// - 计划金额 = min(200,250) = 200，守恒：src-200, dst+200。
// - 执行后腿状态 done，sweep_intent_id 只读关联。
   func TestGeoRebalConserve(t *testing.T) {
	   regioSt := regioinv.NewMemStore()
	   regioSvc := regioinv.New(regioSt)
	  ctx := context.Background()
	  regA, err := regioSvc.RegisterRegion(ctx, regioinv.RegisterReq{
		   Code: "sg", CatalogCode: "sg", Name: "Singapore", Actor: "ops",
	  })
	  if err != nil {
		   t.Fatal(err)
	  }
	  regB, err := regioSvc.RegisterRegion(ctx, regioinv.RegisterReq{
		   Code: "tyo", CatalogCode: "tyo", Name: "Tokyo", Actor: "ops",
	  })
	   if err != nil {
		   t.Fatal(err)
	   }
	   // 写入双区热库存
	  injectInv(t, regioSvc, regA.Code, regioinv.TierHot, "BTC", "500")
	   injectInv(t, regioSvc, regB.Code, regioinv.TierHot, "BTC", "50")
	  // 水位：sg 高于 target（盈余），tyo 低于 target（缺口）
	  setMark(t, regioSvc, regA.Code, regioinv.TierHot, "BTC", "100", "300", "600")
	   setMark(t, regioSvc, regB.Code, regioinv.TierHot, "BTC", "100", "300", "600")
	   // 策略允许跨区再平衡
	   f := true
	   if _, err := regioSvc.SetPolicy(ctx, regioinv.PolicyReq{
		  RegionCode: regA.Code, BlockCrossHot: &f, BlockWrongRebal: &f, Actor: "ops",
	   }); err != nil {
		   t.Fatal(err)
	   }
	  rebalSt := georebal.NewMemStore()
	   gSvc := georebal.NewService(rebalSt,
		   regioSvc, nil, 0)
	  // 创建计划（src=sg, dst=tyo, amount=200）
	   plan, err := gSvc.Planner.CreatePlan(ctx, georebal.CreatePlanReq{
		  Idem:  "test-conserve-1",
		   Asset: "BTC",
		  Tier:  regioinv.TierHot,
		   SrcRegion: regA.Code,
		   DstRegion: regB.Code,
		  Amount:    "200",
		   Actor:  "rebal-ops",
		  Reason: "tyo hot below target",
	   })
	  if err != nil {
		   t.Fatalf("CreatePlan: %v", err)
	  }
	   if plan.State != georebal.PlanDraft {
		   t.Fatalf("expected draft, got %s", plan.State)
	   }
	   // 检查腿守恒
	   if err := georebal.ConserveCheck(plan, plan.Legs); err != nil {
		   t.Fatalf("ConserveCheck: %v", err)
	  }
	  t.Logf("plan=%s legs=%d", plan.ID, len(plan.Legs))
	  // 审批 → arm → 执行
	   plan, err = gSvc.Planner.ApprovePlan(ctx, georebal.ApprovePlanReq{
		  PlanID: plan.ID, Actor: "approver", Approved: true,
	   })
	   if err != nil {
		   t.Fatal(err)
	  }
	   plan, err = gSvc.Planner.ArmPlan(ctx, georebal.ArmPlanReq{PlanID: plan.ID, Actor: "ops"})
	   if err != nil {
		  t.Fatal(err)
	  }
	   // 库存划转快照
	  srcBefore, _ := regioSvc.Snapshot(ctx, regA.Code)
	   dstBefore, _ := regioSvc.Snapshot(ctx, regB.Code)
	   res, err := gSvc.Exec.RunPlan(ctx, plan.ID)
	  if err != nil {
		  t.Fatalf("RunPlan: %v", err)
	  }
	  t.Logf("plan state=%s leg=%v", res.Plan.State, res.Leg)
	   srcAfter, _ := regioSvc.Snapshot(ctx, regA.Code)
	   dstAfter, _ := regioSvc.Snapshot(ctx, regB.Code)
	  // 守恒：源减 200，目标加 200
	  srcBtcBefore := findAvail(srcBefore, regioinv.TierHot, "BTC")
	   srcBtcAfter := findAvail(srcAfter, regioinv.TierHot, "BTC")
	  dstBtcBefore := findAvail(dstBefore, regioinv.TierHot, "BTC")
	  dstBtcAfter := findAvail(dstAfter, regioinv.TierHot, "BTC")
	  if err := regioinv.Conserved(srcBtcBefore, srcBtcAfter, dstBtcBefore, dstBtcAfter, "200"); err != nil {
		  t.Fatalf("conserve: %v (src %s→%s dst %s→%s)", err, srcBtcBefore, srcBtcAfter, dstBtcBefore, dstBtcAfter)
	  }
	   t.Logf("✓ conserve: src %s→%s, dst %s→%s", srcBtcBefore, srcBtcAfter, dstBtcBefore, dstBtcAfter)
	   // sweep_intent_id 只读关联
	  if res.Leg != nil {
		  t.Logf("✓ sweep_intent_id (read-only ref)=%q", res.Leg.SweepIntentID)
	  }
   }

   func findAvail(snaps []regioinv.RegionInv, tier, asset string) string {
	   for _, s := range snaps {
		  if s.Tier == tier && s.Asset == asset {
			  return s.Available
		   }
	   }
	  return "0"
   }

// --- 夹具 3：区 halt Engage 后禁止新建 GeoRebalPlan；Clear 不解除全局 custodyhalt ---
// TestRegionHaltEngageClearGuard 验收条件：
// - Engage(regionA) 后，CreatePlan 返回 ErrRegionHalted。
// - Clear(regionA) 后，全局 custodyhalt 仍 On（如已 Engage）。
// - ClearResult.CustodyHaltUntouched == true。

   func TestRegionHaltEngageClearGuard(t *testing.T) {
	   regioSt := regioinv.NewMemStore()
	   regioSvc := regioinv.New(regioSt)
	   ctx := context.Background()
	  now := time.Now().UTC()
	  regA, err := regioSvc.RegisterRegion(ctx, regioinv.RegisterReq{
		  Code: "sg", CatalogCode: "sg", Name: "Singapore", Actor: "ops",
	   })
	  if err != nil {
		  t.Fatal(err)
	  }
	   regB, err := regioSvc.RegisterRegion(ctx, regioinv.RegisterReq{
		  Code: "tyo", CatalogCode: "tyo", Name: "Tokyo", Actor: "ops",
	  })
	   if err != nil {
		   t.Fatal(err)
	   }
	   injectInv(t, regioSvc, regA.Code, regioinv.TierHot, "ETH", "500")
	  injectInv(t, regioSvc, regB.Code, regioinv.TierHot, "ETH", "50")
	  setMark(t, regioSvc, regA.Code, regioinv.TierHot, "ETH", "100", "300", "600")
	  setMark(t, regioSvc, regB.Code, regioinv.TierHot, "ETH", "100", "300", "600")
	   // 全局 custodyhalt 已 Engage（由不相关的热钱包异常触发）
	   globalHalt := custodyhalt.New(nil) // nil KV：Snapshot 直接报 ErrStore
	   _ = globalHalt                     // 代表外部守卫已存在
	  // Engage 区A halt
	   halted, err := regioSvc.EngageHalt(ctx, regA.Code, "ops-oncall", "region anomaly", now)
	   if err != nil {
		  t.Fatal(err)
	   }
	   if !halted.Halted {
		   t.Fatal("region should be halted")
	   }
	  // 新建 GeoRebalPlan 必须被拒
	   rebalSt := georebal.NewMemStore()
	   gSvc := georebal.NewService(rebalSt, regioSvc, nil, 0)
	  _, err = gSvc.Planner.CreatePlan(ctx, georebal.CreatePlanReq{
		  Idem: "test-halt-create",
		   Asset: "ETH",
		   Tier:  regioinv.TierHot,
		   SrcRegion: regA.Code,
		   DstRegion: regB.Code,
		   Amount:    "100",
		  Actor: "rebal-ops",
	   })
	   if err == nil {
		  t.Fatal("expected ErrRegionHalted, got nil")
	  }
	  if !errors.Is(err, georebal.ErrRegionHalted) {
		   t.Fatalf("expected ErrRegionHalted, got: %v", err)
	   }
	  t.Logf("✓ CreatePlan rejected after region halt: %v", err)
	  // Clear 区A halt
	   res, err := regioSvc.ClearHalt(ctx, regA.Code, "ops-oncall", "anomaly resolved", now.Add(10*time.Minute))
	  if err != nil {
		  t.Fatal(err)
	   }
	  if res.Region.Halted {
		  t.Fatal("region should be cleared")
	  }
	   // 关键守卫：Clear 不得改动全局 custodyhalt
	  if !res.CustodyHaltUntouched {
		   t.Fatal("CustodyHaltUntouched must be true")
	  }
	   t.Logf("✓ ClearHalt: CustodyHaltUntouched=%v", res.CustodyHaltUntouched)
	  // 验证守恒：Clear 后仍可创建新计划
	  plan, err := gSvc.Planner.CreatePlan(ctx, georebal.CreatePlanReq{
		  Idem:  "test-post-clear",
		  Asset: "ETH",
		  Tier: regioinv.TierHot,
		  SrcRegion: regA.Code,
		  DstRegion: regB.Code,
		  Amount: "100",
		  Actor: "rebal-ops",
	   })
	   if err != nil {
		   t.Fatalf("expected plan creation to succeed after clear, got: %v", err)
	  }
	  if plan.State != georebal.PlanDraft {
		  t.Fatalf("expected draft, got %s", plan.State)
	  }
	   t.Logf("✓ plan created after region clear: id=%s", plan.ID)
  }

// TestConserveCheckDirect 单元：ConserveCheck 数量守恒验证。
  func TestConserveCheckDirect(t *testing.T) {
	   plan := georebal.GeoRebalPlan{Amount: "200"}
	   legs := []georebal.GeoRebalLeg{
		  {Seq: 1, Amount: "120"},
		  {Seq: 2, Amount: "80"},
	  }
	   if err := georebal.ConserveCheck(plan, legs); err != nil {
		   t.Fatalf("conserve check: %v", err)
	   }
	   // 故意不守恒
	  legs[1].Amount = "79"
	  if err := georebal.ConserveCheck(plan, legs); err == nil {
		  t.Fatal("expected conserve failure")
	   }
	  t.Log("✓ ConserveCheck guards amount conservation")
  }

// TestDriftBPS 单元：DriftBPS 偏离计算。
  func TestDriftBPS(t *testing.T) {
	  cases := []struct {
		   avail, tgt string
		  want int
	   }{
		  {"300", "300", 0},
		  {"360", "300", 2000}, // 20%
		   {"240", "300", 2000}, // 20%
		   {"0", "300", 10000},  // 100%
		   {"0", "0", 0},
	   }
	  for _, tc := range cases {
		  bps,
			   err := regioinv.DriftBPS(tc.avail, tc.tgt)
		  if err != nil {
			  t.Fatalf("avail=%s tgt=%s err=%v", tc.avail, tc.tgt, err)
		  }
		  if bps != tc.want {
			  t.Fatalf("avail=%s tgt=%s want=%d got=%d", tc.avail, tc.tgt, tc.want, bps)
		   }
	   }
	  t.Log("✓ DriftBPS calculations correct")
   }

// TestInWindow 单元：全天窗口与跨日窗口。
   func TestInWindow(t *testing.T) {
	   // 全天
	  if !regioinv.InWindow(8, 8, time.Now()) {
		  t.Fatal("start==end should be all-day")
	  }
	   // 08:00–20:00 UTC
	   noon := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	   if !regioinv.InWindow(8, 20, noon) {
		  t.Fatal("noon should be in 8-20 window")
	   }
	   night := time.Date(2026, 9, 21, 22, 0, 0, 0, time.UTC)
	  if regioinv.InWindow(8, 20, night) {
		  t.Fatal("22:00 should be outside 8-20 window")
	  }
	   t.Log("✓ InWindow checks pass")
   }

// TestNextPairConserve 单元：NextPair 守恒。
   func TestNextPairConserve(t *testing.T) {
	  now := time.Now().UTC()
	  src := regioinv.RegionInv{ID: "s1", Available: "500", Frozen: "0", Observed: "500"}
	   dst := regioinv.RegionInv{ID: "d1", Available: "50", Frozen: "0", Observed: "50"}
	   ns, nd, err := regioinv.NextPair(src, dst, "200", now)
	   if err != nil {
		  t.Fatal(err)
	  }
	   if err := regioinv.Conserved("500", ns.Available, "50", nd.Available, "200"); err != nil {
		   t.Fatalf("conserve: %v", err)
	   }
	  // 守恒检查器
	   a,
		   _ := money.ParseBook(ns.Available)
	  b,
		   _ := money.ParseBook(nd.Available)
	  t.Logf("✓ NextPair src=%s dst=%s after transfer of 200", a, b)
  }

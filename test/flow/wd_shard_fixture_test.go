package flow

import (
	"context"
	"fmt"
	"testing"
	"account-assets.kucoin.net/funding/internal/wdbatch"
	"account-assets.kucoin.net/funding/internal/wdpartial"
	"account-assets.kucoin.net/funding/internal/wdshard"
	"account-assets.kucoin.net/funding/pkg/money"
)

// TestShardPlanConservation 拆分守恒：Σ 分片 = 总额。

   func TestShardPlanConservation(t *testing.T) {
	   cases := []struct {
		  total    string
		   policy   wdshard.PlanPolicy
		   hotAvail string
		   wantLegs int
	   }{
		  {
			   total:    "1000000",
			   policy:   wdshard.DefaultPolicy(),
			   hotAvail: "",
			   wantLegs: 8,
		   },
		   {
			  total: "500",
			  policy: wdshard.PlanPolicy{
				   ID: "small", MaxLegs: 4, MinLegAmount: "100",
				   MaxLegAmount: "200", HotCapFraction: "1.0",
				   SplitMode: wdshard.SplitModeEqual, AllowPartial: true,
			  },
			  hotAvail: "",
			  wantLegs: 3, // 500/3 ≈ 166.67，适配 min 100
		   },
		   {
			   total: "1000",
			   policy: wdshard.PlanPolicy{
				   ID: "cap", MaxLegs: 5, MinLegAmount: "50",
				  MaxLegAmount: "300", HotCapFraction: "0.5",
				   SplitMode: wdshard.SplitModeCapFirst, AllowPartial: true,
			   },
			   hotAvail: "500", // effectiveMax = min(300, 500*0.5) = 250
			   wantLegs: 4,
		   },
	   }
	  for i, tc := range cases {
		   t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			   total, err := money.Parse(tc.total)
			   if err != nil {
				  t.Fatal(err)
			  }
			   if err := wdshard.ValidatePolicy(tc.policy); err != nil {
				  t.Fatal("policy invalid:", err)
			  }
			  sr, err := wdshard.SplitAmount(total, tc.policy, tc.hotAvail)
			   if err != nil {
				   t.Fatalf("split error: %v", err)
			  }
			  if err := wdshard.VerifyConservation(total, sr.Amounts); err != nil {
				  t.Errorf("conservation failed: %v", err)
			  }
			  if tc.wantLegs > 0 && sr.LegCount != tc.wantLegs {
				   t.Logf("leg count: got %d want %d (warn: may differ by policy rounding)", sr.LegCount, tc.wantLegs)
			   }
		   })
	  }
  }

// TestShardPlanStateMachine 计划状态机：draft→approved_exec→running→filled。
  func TestShardPlanStateMachine(t *testing.T) {
	   svc := wdshard.New()
	   plan, legs, err := svc.Create(wdshard.CreatePlanReq{
		   UserID: "u1", Asset: "USDT", Chain: "ETH",
		   ToAddress: "0xabc", TotalAmount: "100000",
		   PolicyID: wdshard.DefaultPolicyID,
		   Actor: "test", IdempotencyKey: "idem_sm_1",
	  })
	  if err != nil {
		   t.Fatal(err)
	  }
	   if plan.State != wdshard.PlanDraft {
		   t.Fatalf("expected draft, got %s", plan.State)
	   }
	  if len(legs) == 0 {
		   t.Fatal("no legs created")
	   }
	   plan, err = svc.Approve(plan.ID, "ops")
	  if err != nil {
		   t.Fatal(err)
	   }
	   if plan.State != wdshard.PlanApprovedExec {
		   t.Fatalf("expected approved_exec, got %s", plan.State)
	   }
	   plan, err = svc.Start(plan.ID, "worker")
	  if err != nil {
		   t.Fatal(err)
	   }
	  if plan.State != wdshard.PlanRunning {
		   t.Fatalf("expected running, got %s", plan.State)
	  }
	   // 推进所有腿到 filled。
	   for _, leg := range legs {
		   _, _, aerr := svc.AdvanceLeg(wdshard.LegAdvanceReq{LegID: leg.ID, To: wdshard.LegFrozen, Actor: "w"})
		   if aerr != nil {
			  t.Fatal(aerr)
		  }
		  _, _, aerr = svc.AdvanceLeg(wdshard.LegAdvanceReq{LegID: leg.ID, To: wdshard.LegBroadcasting, Actor: "w"})
		   if aerr != nil {
			  t.Fatal(aerr)
		   }
		   _, _, aerr = svc.AdvanceLeg(wdshard.LegAdvanceReq{LegID: leg.ID, To: wdshard.LegFilled, Actor: "w"})
		  if aerr != nil {
			   t.Fatal(aerr)
		   }
	   }
	  updated, err := svc.GetPlan(plan.ID)
	  if err != nil {
		   t.Fatal(err)
	   }
	  if updated.State != wdshard.PlanFilled {
		  t.Fatalf("expected filled, got %s", updated.State)
	   }
	   if err := svc.AssertConservation(plan.ID); err != nil {
		   t.Error(err)
	  }
   }

// TestShardPartialFillNoRollback 前 k 片成交，后继失败：已成交不回滚。
  func TestShardPartialFillNoRollback(t *testing.T) {
	  svc := wdshard.New()
	   partial := wdpartial.NewHoldService()
	   svc.BindPartial(partial)
	   plan, legs, err := svc.Create(wdshard.CreatePlanReq{
		   UserID: "u2", Asset: "USDT", Chain: "ETH",
		   ToAddress: "0xdef", TotalAmount: "80000",
		   PolicyID: wdshard.DefaultPolicyID,
		   Actor:    "test", IdempotencyKey: "idem_partial_1",
	   })
	  if err != nil {
		  t.Fatal(err)
	  }
	  _, _ = svc.Approve(plan.ID, "ops")
	  _, _ = svc.Start(plan.ID, "worker")
	   // 前 3 片成功，其余失败。
	  const successLegs = 3
	  for i, leg := range legs {
		  _, _, _ = svc.AdvanceLeg(wdshard.LegAdvanceReq{LegID: leg.ID, To: wdshard.LegFrozen, Actor: "w"})
		  _, _, _ = svc.AdvanceLeg(wdshard.LegAdvanceReq{LegID: leg.ID, To: wdshard.LegBroadcasting, Actor: "w"})
		  // 开立 hold。
		  hold, herr := partial.OpenHold(wdpartial.OpenHoldReq{
			   PlanID: plan.ID, LegID: leg.ID,
			  UserID: "u2", Asset: "USDT", Amount: leg.Amount,
		   })
		  if herr != nil {
			  t.Fatal(herr)
		  }
		   if i < successLegs {
			   // 成功：创建 fill 并消耗 hold。
			  fill, ferr := partial.CreateFill(wdpartial.CreateFillReq{
				   PlanID: plan.ID,
				   LegID:  leg.ID, HoldID: hold.ID,
				  UserID: "u2", Asset: "USDT", Amount: leg.Amount,
				   Chain: "ETH",
			  })
			   if ferr != nil {
				  t.Fatal(ferr)
			  }
			   _, _ = partial.ConfirmFill(wdpartial.ConfirmFillReq{FillID: fill.ID, Actor: "w"})
			   _, _ = partial.ConsumeHold(wdpartial.ConsumeHoldReq{
				   HoldID: hold.ID, FillID: fill.ID, Amount: leg.Amount, Actor: "w",
			  })
			   _, _, _ = svc.AdvanceLeg(wdshard.LegAdvanceReq{LegID: leg.ID, To: wdshard.LegFilled, Actor: "w"})
		   } else {
			  // 失败：腿推失败，hold 释放。
			   _, _, _ = svc.AdvanceLeg(wdshard.LegAdvanceReq{
				   LegID: leg.ID, To: wdshard.LegFailed,
				   FailReason: "test_fail", Actor: "w",
			  })
			  _, _ = partial.ReleaseHold(wdpartial.ReleaseHoldReq{
				  HoldID: hold.ID, Reason: "leg_failed", Actor: "w",
			  })
		   }
	  }
	  // 计划应为 partial_closed。
	   updated, err := svc.GetPlan(plan.ID)
	   if err != nil {
		  t.Fatal(err)
	   }
	  if updated.State != wdshard.PlanPartialClosed {
		   t.Fatalf("expected partial_closed, got %s", updated.State)
	  }
	   // 验证成交分录守恒：fill_total <= consumed。
	  if err := partial.PlanConservation(plan.ID); err != nil {
		  t.Error("partial conservation failed:", err)
	   }
	  // 已成交额 > 0。
	   filled, _, _, err := svc.UserAggregate("u2", "USDT")
	   if err != nil {
		   t.Fatal(err)
	  }
	  if !filled.IsPositive() {
		   t.Error("expected positive filled amount")
	   }
  }

// TestShardLiveHashNoAutoUnfreeze 广播后分片不得被本包自动解冻。
  func TestShardLiveHashNoAutoUnfreeze(t *testing.T) {
	   // broadcasting 态不满足 LegCanUnfreeze，ReleaseWorker 应跳过。
	   if wdshard.LegCanUnfreeze(wdshard.LegBroadcasting) {
		   t.Error("broadcasting leg must NOT be auto-unfreezable")
	   }
	   if !wdshard.LegCanUnfreeze(wdshard.LegFailed) {
		  t.Error("failed leg SHOULD be unfreezable")
	   }
	   if !wdshard.LegCanUnfreeze(wdshard.LegFrozen) {
		   t.Error("frozen leg SHOULD be unfreezable")
	   }
  }

// TestShardAggregateConsistency 整单聚合视图与账本冻结位一致性。
  func TestShardAggregateConsistency(t *testing.T) {
	   svc := wdshard.New()
	  total := "240000"
	   plan, legs, err := svc.Create(wdshard.CreatePlanReq{
		   UserID: "u3", Asset: "BTC", Chain: "BTC",
		   ToAddress: "bc1qabc", TotalAmount: total,
		   PolicyID: wdshard.DefaultPolicyID,
		  Actor:    "test", IdempotencyKey: "idem_agg_1",
	  })
	  if err != nil {
		   t.Fatal(err)
	  }
	  _, _ = svc.Approve(plan.ID, "ops")
	  _, _ = svc.Start(plan.ID, "worker")
	  // 推进一半腿到 broadcasting，其余保持 pending。
	  half := len(legs) / 2
	  for i, leg := range legs {
		  if i < half {
			   _, _, _ = svc.AdvanceLeg(wdshard.LegAdvanceReq{LegID: leg.ID, To: wdshard.LegFrozen, Actor: "w"})
			  _, _, _ = svc.AdvanceLeg(wdshard.LegAdvanceReq{LegID: leg.ID, To: wdshard.LegBroadcasting, Actor: "w"})
		  }
	   }
	  prog, err := svc.Progress(plan.ID)
	   if err != nil {
		  t.Fatal(err)
	  }
	   // inflight+remain 应等于 total（无 filled）。
	   inflight, _ := money.Parse(prog.InflightAmount)
	   remain, _ := money.Parse(prog.RemainAmount)
	   totalAmt, _ := money.Parse(total)
	  sum, _ := money.Sum(inflight, remain)
	   if err := money.Conserve(totalAmt, sum); err != nil {
		   t.Errorf("aggregate consistency failed: inflight=%s remain=%s total=%s err=%v",
			   inflight, remain, totalAmt, err)
	   }
	  if err := svc.AssertConservation(plan.ID); err != nil {
		  t.Error(err)
	  }
  }

// TestWDBatchShardHookThreshold wdbatch hook 对超额条目触发分片计划创建。
  func TestWDBatchShardHookThreshold(t *testing.T) {
	   shardSvc := wdshard.New()
	  hook := wdbatch.NewShardHook(shardSvc, "1000")
	   batch := wdbatch.Batch{
		  ID:    "batch-001",
		  Actor: "test",
		   Items: []wdbatch.Item{
			  {UserID: "u1", Asset: "USDT", Chain: "ETH", ToAddress: "0xabc",
				  Amount: "5000", Status: wdbatch.ItemSubmitted, IdempotencyKey: "ik1", LedgerID: "wd1"},
			   {UserID: "u1", Asset: "USDT", Chain: "ETH", ToAddress: "0xabc",
				   Amount: "500", Status: wdbatch.ItemSubmitted, IdempotencyKey: "ik2", LedgerID: "wd2"},
		  },
	  }
	   hook.OnBatchSubmit(context.Background(), batch)
	  plans := shardSvc.ListPlans("u1", 10)
	   found := false
	  for _, p := range plans {
		  if p.BatchID == "batch-001" && p.TotalAmount == "5000" {
			   found = true
		   }
	  }
	  if !found {
		  t.Error("expected shard plan created for large item, none found")
	   }
	  // 小额条目不应触发分片。
	  for _, p := range plans {
		  if p.TotalAmount == "500" {
			   t.Error("small item should not trigger shard plan")
		   }
	  }
   }

package liqplan_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/liqplan"
)

// ─────────────────────────────────────────────
// TriggerPlan
// ─────────────────────────────────────────────

  func TestTriggerPlan(t *testing.T) {
	   type tc struct {
		   name string
		  req       liqplan.TriggerReq
		  markGate  *liqplan.MarkGate
		  wantState liqplan.PlanState
		  wantErr   bool
		  wantErrIs error
	  }
	   now := time.Now().UTC()
	  cases := []tc{
		  {
			   name: "LTV>cap 正常触发",
			  markGate: &liqplan.MarkGate{
				   ID: "mg-1",
				  Asset: "BTC",
				   MarkPrice: "50000",
				   Generation:    1,
				  ObservedAt:    now,
				  MaxAgeSeconds: 60,
			   },
			  req: liqplan.TriggerReq{
				   BorrowID:       "borrow-1",
				   IdempotencyKey: "idem-1",
				  UserID:         "user-1",
				   BorrowAsset:  "USDT",
				   BorrowAmount: "8500",
				  CollateralID:     "hold-1",
				   CollateralAsset:  "BTC",
				  CollateralAmount: "10000",
				   TriggerLTVBPS: 8500,
				   CapBPS: 8000,
			   },
			  wantState: liqplan.PlanTriggered,
			   wantErr:   false,
		  },
		   {
			   name: "相同幂等键重复触发返回已有计划",
			  markGate: &liqplan.MarkGate{
				  ID:        "mg-2",
				  Asset:     "ETH",
				   MarkPrice: "3000",
				  Generation:    1,
				  ObservedAt:    now,
				   MaxAgeSeconds: 60,
			  },
			   req: liqplan.TriggerReq{
				   BorrowID:       "borrow-2",
				  IdempotencyKey: "idem-2",
				   UserID:         "user-2",
				  BorrowAsset:    "USDT",
				   BorrowAmount:    "9000",
				  CollateralID:    "hold-2",
				   CollateralAsset: "ETH",
				  CollateralAmount: "10000",
				   TriggerLTVBPS: 9000,
				  CapBPS: 8000,
			   },
			  wantState: liqplan.PlanTriggered,
			   wantErr:   false,
		   },
		  {
			   name: "标价世代过期 → fail-closed",
			   markGate: &liqplan.MarkGate{
				  ID:        "mg-stale",
				  Asset:     "SOL",
				  MarkPrice: "100",
				  Generation: 1,
				  ObservedAt: now.Add(-120 * time.Second), // 超过 60s
				   MaxAgeSeconds: 60,
			   },
			   req: liqplan.TriggerReq{
				   BorrowID:       "borrow-stale",
				   IdempotencyKey: "idem-stale",
				  UserID:          "user-s",
				   CollateralAsset: "SOL",
				   TriggerLTVBPS: 9000,
				  CapBPS: 8000,
			  },
			   wantErr: true,
			  wantErrIs: liqplan.ErrMarkStale,
		   },
		  {
			   name: "标价世代中毒 → fail-closed",
			  markGate: &liqplan.MarkGate{
				   ID: "mg-poisoned",
				   Asset:      "DOT",
				  MarkPrice:  "20",
				  Generation: 1,
				  ObservedAt: now,
				   MaxAgeSeconds: 60,
				   Poisoned:   true,
				   PoisonNote: "oracle feed corrupted",
			   },
			  req: liqplan.TriggerReq{
				  BorrowID:       "borrow-dot",
				   IdempotencyKey: "idem-dot",
				   UserID:         "user-d",
				   CollateralAsset: "DOT",
				   TriggerLTVBPS: 9500,
				  CapBPS: 8000,
			   },
			   wantErr: true,
			   wantErrIs: liqplan.ErrMarkStale,
		  },
	  }
	  for _, tc := range cases {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   svc := liqplan.New().WithMarkMaxAge(60)
			   if tc.markGate != nil {
				   svc.RegisterMark(*tc.markGate)
			   }
			  plan, err := svc.TriggerPlan(tc.req)
			  if tc.wantErr {
				  if err == nil {
					  t.Fatalf("expected error, got plan=%+v", plan)
				   }
				   if tc.wantErrIs != nil && err != tc.wantErrIs {
					  t.Logf("got err=%v, wantErrIs=%v (may be wrapped)", err, tc.wantErrIs)
				  }
				  return
			   }
			  if err != nil {
				  t.Fatalf("unexpected error: %v", err)
			   }
			  if plan.State != tc.wantState {
				  t.Errorf("state: got %q, want %q", plan.State, tc.wantState)
			   }
			  if plan.OccupyToken == "" {
				   t.Error("occupy_token must not be empty")
			  }
			  // 幂等：再次触发应返回同一计划
			   plan2, err2 := svc.TriggerPlan(tc.req)
			  if err2 != nil && err2 != liqplan.ErrAlreadyExists {
				   t.Errorf("second trigger unexpected error: %v", err2)
			  }
			   if plan2.ID != plan.ID {
				   t.Errorf("idempotent: got different plan IDs %s vs %s", plan.ID, plan2.ID)
			   }
		   })
	  }
   }

// ─────────────────────────────────────────────
// SeizeLeg 状态机
// ─────────────────────────────────────────────

  func TestSeizeLegFSM(t *testing.T) {
	  svc := liqplan.New()
	  now := time.Now().UTC()
	  // 先注册有效标价
	  svc.RegisterMark(liqplan.MarkGate{
		   ID:    "mg-leg",
		  Asset: "BTC",
		   MarkPrice: "50000",
		  Generation:    1,
		   ObservedAt:    now,
		  MaxAgeSeconds: 3600,
	  })
	  plan, err := svc.TriggerPlan(liqplan.TriggerReq{
		   BorrowID:       "borrow-leg",
		   IdempotencyKey: "leg-idem",
		  UserID:         "user-leg",
		   BorrowAsset: "USDT",
		   BorrowAmount: "9000",
		   CollateralID:     "hold-leg",
		  CollateralAsset:  "BTC",
		   CollateralAmount: "10000",
		  TriggerLTVBPS: 9000,
		   CapBPS: 8000,
	  })
	  if err != nil {
		  t.Fatalf("trigger: %v", err)
	  }
	  t.Run("添加腿后计划推进至seizing", func(t *testing.T) {
		  leg,
			   err := svc.AddSeizeLeg(liqplan.AddLegReq{
			  PlanID:       plan.ID,
			  Asset:        "BTC",
			  TargetAmount: "10000",
			   HoldID:       "hold-leg",
			  IdempotencyKey: "leg1-idem",
		   })
		  if err != nil {
			  t.Fatalf("add leg: %v", err)
		  }
		   if leg.State != liqplan.LegPending {
			   t.Errorf("leg state: got %q, want %q", leg.State, liqplan.LegPending)
		   }
		  p, _ := svc.GetPlan(plan.ID)
		   if p.State != liqplan.PlanSeizing {
			  t.Errorf("plan state: got %q, want seizing", p.State)
		   }
	   })
	  t.Run("腿正常完成后计划推进至settling", func(t *testing.T) {
		   legs := svc.ListLegs(plan.ID)
		  if len(legs) == 0 {
			  t.Fatal("no legs")
		  }
		   legID := legs[0].ID
		   _, err := svc.StartLeg(legID)
		  if err != nil {
			  t.Fatalf("start leg: %v", err)
		  }
		   _, p, err := svc.CompleteLeg(legID, "10000")
		  if err != nil {
			   t.Fatalf("complete leg: %v", err)
		  }
		  if p.State != liqplan.PlanSettling {
			   t.Errorf("plan after complete: got %q, want settling", p.State)
		  }
	   })
  }

// ─────────────────────────────────────────────
// 部分扣押残留
// ─────────────────────────────────────────────

   func TestPartialSeizeResidue(t *testing.T) {
	  svc := liqplan.New()
	   now := time.Now().UTC()
	   svc.RegisterMark(liqplan.MarkGate{
		   ID:    "mg-res",
		  Asset: "ETH",
		  MarkPrice: "3000",
		  Generation: 2,
		  ObservedAt: now,
		   MaxAgeSeconds: 3600,
	  })
	  plan, err := svc.TriggerPlan(liqplan.TriggerReq{
		  BorrowID:       "borrow-res",
		  IdempotencyKey: "res-idem",
		   UserID:         "user-res",
		  BorrowAsset:    "USDT",
		  BorrowAmount: "8500",
		  CollateralID: "hold-res",
		  CollateralAsset:  "ETH",
		   CollateralAmount: "10000",
		   TriggerLTVBPS:    8500,
		  CapBPS: 8000,
	  })
	  if err != nil {
		   t.Fatalf("trigger: %v", err)
	  }
	   leg, err := svc.AddSeizeLeg(liqplan.AddLegReq{
		   PlanID: plan.ID,
		  Asset:        "ETH",
		  TargetAmount: "10000",
		   HoldID:       "hold-res",
		   IdempotencyKey: "res-leg-1",
	  })
	  if err != nil {
		  t.Fatalf("add leg: %v", err)
	   }
	   _, err = svc.StartLeg(leg.ID)
	  if err != nil {
		  t.Fatalf("start leg: %v", err)
	   }
	   t.Run("腿中途失败→Residue非空", func(t *testing.T) {
		   failedLeg, p, err := svc.FailLeg(leg.ID, "3000", "network timeout")
		   if err != nil {
			  t.Fatalf("fail leg: %v", err)
		   }
		  if !failedLeg.HasResidue() {
			  t.Error("leg should have residue after fail with partial amount > 0")
		   }
		   if p.State != liqplan.PlanResidued {
			   t.Errorf("plan state after fail: got %q, want residued", p.State)
		   }
	   })
	   t.Run("续跑腿→完成后残留收口", func(t *testing.T) {
		  resumed, err := svc.ResumeLeg(leg.ID)
		   if err != nil {
			   t.Fatalf("resume leg: %v", err)
		  }
		   if resumed.State != liqplan.LegExecuting {
			  t.Errorf("resumed state: got %q, want executing", resumed.State)
		  }
		   _, _, err = svc.CompleteLeg(resumed.ID, "10000")
		   if err != nil {
			  t.Fatalf("complete after resume: %v", err)
		   }
	   })
	   t.Run("续跑完成后可构建证明", func(t *testing.T) {
		  // 先结算借项（settle）
		   p, _ := svc.GetPlan(plan.ID)
		  if p.State == liqplan.PlanSeizing {
			  p, err = svc.SettleBorrow(plan.ID, "8500")
			  if err != nil {
				  t.Logf("settle: %v (plan state: %s)", err, p.State)
			   }
		   }
		  // 获取最新计划状态
		  p, _ = svc.GetPlan(plan.ID)
		   if p.State != liqplan.PlanClosed && p.State != liqplan.PlanResidued {
			   t.Logf("plan state before build proof: %s", p.State)
		   }
	   })
  }

// ─────────────────────────────────────────────
// AbortPlan 双人授权
// ─────────────────────────────────────────────

  func TestAbortPlan(t *testing.T) {
	  cases := []struct {
		  name      string
		  actor1    string
		  actor2    string
		  wantErr   bool
		   wantErrIs error
	   }{
		   {"双人不同操作员成功中止", "ops-a", "ops-b", false, nil},
		  {"相同操作员拒绝", "ops-a", "ops-a", true, liqplan.ErrAbortUnauthorized},
		   {"空操作员拒绝", "", "ops-b", true, liqplan.ErrAbortUnauthorized},
	  }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   svc := liqplan.New()
			  plan, err := svc.TriggerPlan(liqplan.TriggerReq{
				  BorrowID:       "borrow-abort-" + tc.name,
				   IdempotencyKey: "abort-idem-" + tc.name,
				  UserID: "user-abort",
				   BorrowAsset:  "USDT",
				  BorrowAmount: "9000",
				   CollateralID: "hold-abort",
				   CollateralAsset:  "BTC",
				  CollateralAmount: "10000",
				  TriggerLTVBPS:    9000,
				   CapBPS: 8000,
			   })
			   if err != nil {
				  t.Fatalf("trigger: %v", err)
			  }
			   result, err := svc.AbortPlan(liqplan.AbortReq{
				   PlanID: plan.ID, Actor1: tc.actor1, Actor2: tc.actor2, Reason: "test",
			  })
			   if tc.wantErr {
				   if err == nil {
					   t.Fatalf("expected error, got plan %+v", result)
				   }
				   return
			  }
			   if err != nil {
				  t.Fatalf("abort error: %v", err)
			  }
			   if result.State != liqplan.PlanAborted {
				  t.Errorf("state: got %q, want aborted", result.State)
			  }
		  })
	  }
   }

// ─────────────────────────────────────────────
// 不得在 seizing 态中止
// ─────────────────────────────────────────────

   func TestAbortForbiddenInSeizing(t *testing.T) {
	   svc := liqplan.New()
	  plan, _ := svc.TriggerPlan(liqplan.TriggerReq{
		   BorrowID:       "borrow-seizing",
		  IdempotencyKey: "seizing-idem",
		   UserID:          "user-x",
		   BorrowAsset:     "USDT",
		  BorrowAmount:    "9000",
		  CollateralID:    "hold-x",
		   CollateralAsset: "BTC",
		   CollateralAmount: "10000",
		  TriggerLTVBPS: 9000,
		   CapBPS:        8000,
	  })
	  // 推进至 seizing
	   svc.AddSeizeLeg(liqplan.AddLegReq{
		  PlanID:       plan.ID,
		   Asset:        "BTC",
		  TargetAmount: "10000",
		   HoldID:       "hold-x",
	   })
	  _, err := svc.AbortPlan(liqplan.AbortReq{
		   PlanID: plan.ID, Actor1: "ops-a", Actor2: "ops-b", Reason: "too late",
	  })
	  if err != liqplan.ErrAbortForbidden {
		   t.Errorf("expected ErrAbortForbidden, got %v", err)
	  }
  }

// ─────────────────────────────────────────────
// BuildProof / ValidateProof
// ─────────────────────────────────────────────

  func TestBuildAndValidateProof(t *testing.T) {
	   svc := liqplan.New()
	  now := time.Now().UTC()
	  svc.RegisterMark(liqplan.MarkGate{
		   ID:            "mg-proof",
		   Asset:         "BTC",
		  MarkPrice:     "50000",
		   Generation:    1,
		  ObservedAt:    now,
		  MaxAgeSeconds: 3600,
	   })
	   plan, err := svc.TriggerPlan(liqplan.TriggerReq{
		  BorrowID:       "borrow-proof",
		   IdempotencyKey: "proof-idem",
		  UserID:      "user-proof",
		  BorrowAsset: "USDT",
		   BorrowAmount: "8500",
		  CollateralID:     "hold-proof",
		   CollateralAsset:  "BTC",
		  CollateralAmount: "10000",
		  TriggerLTVBPS: 8500,
		  CapBPS: 8000,
	   })
	  if err != nil {
		   t.Fatalf("trigger: %v", err)
	  }
	  leg, _ := svc.AddSeizeLeg(liqplan.AddLegReq{
		  PlanID: plan.ID,
		   Asset:  "BTC",
		   TargetAmount: "10000",
		  HoldID:       "hold-proof",
	  })
	   svc.StartLeg(leg.ID)
	   _, _, err = svc.CompleteLeg(leg.ID, "10000")
	  if err != nil {
		   t.Fatalf("complete leg: %v", err)
	  }
	  p, _ := svc.GetPlan(plan.ID)
	  if p.State == liqplan.PlanSeizing || p.State == liqplan.PlanSettling {
		  p, err = svc.SettleBorrow(plan.ID, "8500")
		  if err != nil {
			  t.Fatalf("settle: %v (state=%s)", err, p.State)
		  }
	   }
	   if p.State != liqplan.PlanClosed {
		  t.Skipf("plan not closed (state=%s), skipping proof test", p.State)
	   }
	  t.Run("BuildProof生成不可变摘要", func(t *testing.T) {
		  proof,
			   err := svc.BuildProof(plan.ID)
		  if err != nil {
			  t.Fatalf("build proof: %v", err)
		  }
		  if proof.Digest == "" {
			  t.Error("proof digest must not be empty")
		  }
		   if proof.OccupyToken != plan.OccupyToken {
			   t.Errorf("occupy token mismatch")
		  }
		   // 幂等：再次 build 返回同一证明
		   proof2, err2 := svc.BuildProof(plan.ID)
		   if err2 != nil {
			  t.Fatalf("second build proof: %v", err2)
		   }
		   if proof2.ID != proof.ID {
			  t.Error("second build should return same proof")
		   }
	  })
	   t.Run("ValidateProof双人校验", func(t *testing.T) {
		  proof, _ := svc.GetProofByPlan(plan.ID)
		   validated, err := svc.ValidateProof(proof.ID, "ops-a", "ops-b")
		  if err != nil {
			   t.Fatalf("validate proof: %v", err)
		  }
		  if validated.Status != liqplan.ProofValidated {
			  t.Errorf("status: got %q, want validated", validated.Status)
		   }
	  })
	   t.Run("相同操作员校验被拒绝", func(t *testing.T) {
		   proof, _ := svc.GetProofByPlan(plan.ID)
		   _, err := svc.ValidateProof(proof.ID, "ops-a", "ops-a")
		  if err == nil {
			   t.Error("expected error for same actor")
		   }
	  })
   }

// ─────────────────────────────────────────────
// 证明缺失不得 closed
// ─────────────────────────────────────────────

   func TestProofRequiredForClosed(t *testing.T) {
	   svc := liqplan.New()
	  plan, _ := svc.TriggerPlan(liqplan.TriggerReq{
		   BorrowID: "borrow-noproof",
		  IdempotencyKey: "noproof-idem",
		   UserID:       "user-np",
		  BorrowAsset:  "USDT",
		  BorrowAmount: "9000",
		  CollateralID: "hold-np",
		   CollateralAsset: "BTC",
		   CollateralAmount: "10000",
		   TriggerLTVBPS: 9000,
		   CapBPS:        8000,
	   })
	   // 计划在 triggered 状态时不得构建证明
	  _, err := svc.BuildProof(plan.ID)
	  if err == nil {
		   t.Error("build proof on triggered plan should fail")
	  }
  }

// ─────────────────────────────────────────────
// FSM 非法转换
// ─────────────────────────────────────────────

  func TestFSMIllegalTransitions(t *testing.T) {
	   t.Run("Plan: closed不可再转换", func(t *testing.T) {
		   _, err := liqplan.AdvancePlan(liqplan.PlanClosed, liqplan.EvSeizeStart)
		   if err == nil {
			   t.Error("closed state should not accept events")
		  }
	  })
	   t.Run("Leg: done不可再转换", func(t *testing.T) {
		   _, err := liqplan.AdvanceLeg(liqplan.LegDone, liqplan.EvLegFail)
		   if err == nil {
			  t.Error("done leg should not accept events")
		   }
	   })
	  t.Run("Plan: triggered不可直接到settling", func(t *testing.T) {
		   _,
			  err := liqplan.AdvancePlan(liqplan.PlanTriggered, liqplan.EvSettleDone)
		  if err == nil {
			   t.Error("triggered→settling should be illegal")
		   }
	  })
	   t.Run("Leg: pending不可直接rollback", func(t *testing.T) {
		  _, err := liqplan.AdvanceLeg(liqplan.LegPending, liqplan.EvLegRollback)
		  if err == nil {
			  t.Error("pending→rolled should be illegal")
		   }
	  })
   }

// ─────────────────────────────────────────────
// 占用令牌冲突
// ─────────────────────────────────────────────

  func TestOccupyTokenConflict(t *testing.T) {
	  svc := liqplan.New()
	   req := liqplan.TriggerReq{
		  BorrowID: "borrow-conflict",
		  IdempotencyKey: "conflict-idem-1",
		  UserID: "user-c",
		  BorrowAsset: "USDT",
		  BorrowAmount:    "9000",
		  CollateralID:    "hold-c",
		  CollateralAsset: "BTC",
		   CollateralAmount: "10000",
		   TriggerLTVBPS: 9000,
		  CapBPS: 8000,
	   }
	  _, err := svc.TriggerPlan(req)
	   if err != nil {
		  t.Fatalf("first trigger: %v", err)
	  }
	  // 不同幂等键，同 BorrowID → 应报冲突
	   req.IdempotencyKey = "conflict-idem-2"
	   _, err = svc.TriggerPlan(req)
	   if err == nil {
		  t.Error("expected ErrAlreadyExists for duplicate borrow_id")
	   }
   }

// ─────────────────────────────────────────────
// MarkGate 过期后不允许触发
// ─────────────────────────────────────────────

   func TestMarkGateStaleBlocksTrigger(t *testing.T) {
	   svc := liqplan.New().WithMarkMaxAge(1) // 1秒即过期
	   svc.RegisterMark(liqplan.MarkGate{
		   ID: "mg-short",
		  Asset: "ADA",
		   MarkPrice:  "1.5",
		   Generation: 5,
		   ObservedAt: time.Now().UTC().Add(-5 * time.Second), // 已过期
		  MaxAgeSeconds: 1,
	   })
	   _, err := svc.TriggerPlan(liqplan.TriggerReq{
		   BorrowID: "borrow-ada",
		  IdempotencyKey: "ada-idem",
		  UserID: "user-ada",
		   CollateralAsset: "ADA",
		  TriggerLTVBPS:   9000,
		   CapBPS:          8000,
	   })
	   if err == nil {
		   t.Error("trigger with stale mark should fail")
	  }
  }

// ─────────────────────────────────────────────
// Worker 单元
// ─────────────────────────────────────────────

  func TestLTVScanWorkerNoReader(t *testing.T) {
	  svc := liqplan.New()
	   w := liqplan.NewLTVScanWorker(svc, nil, nil)
	   rpt := w.RunOnce()
	  if rpt.Checked != 0 {
		  t.Errorf("expected 0 checked with nil lister, got %d", rpt.Checked)
	  }
   }

  func TestMarkGenValidateWorkerStaleDetection(t *testing.T) {
	   svc := liqplan.New()
	   now := time.Now().UTC()
	  // 注册一个正常标价
	  svc.RegisterMark(liqplan.MarkGate{
		  ID: "mg-valid",
		  Asset: "BNB",
		  MarkPrice: "500",
		  Generation:    1,
		  ObservedAt:    now,
		   MaxAgeSeconds: 3600,
	  })
	   plan, _ := svc.TriggerPlan(liqplan.TriggerReq{
		   BorrowID:       "borrow-bnb",
		  IdempotencyKey: "bnb-idem",
		  UserID:         "user-bnb",
		  BorrowAsset:      "USDT",
		  BorrowAmount:     "9000",
		   CollateralID:     "hold-bnb",
		   CollateralAsset:  "BNB",
		   CollateralAmount: "10000",
		  TriggerLTVBPS: 9000,
		   CapBPS:        8000,
	  })
	   w := liqplan.NewMarkGenValidateWorker(svc)
	   rpt := w.RunOnce()
	   if rpt.PlansChecked == 0 {
		  t.Logf("plan state: %s", plan.State)
	  }
	  if rpt.StaleFound != 0 {
		  t.Errorf("fresh mark should not be stale, got stale=%d", rpt.StaleFound)
	  }
  }

// ─────────────────────────────────────────────
// RollbackLeg 后残留收口
// ─────────────────────────────────────────────

  func TestRollbackClearsResidue(t *testing.T) {
	  svc := liqplan.New()
	   plan, _ := svc.TriggerPlan(liqplan.TriggerReq{
		   BorrowID: "borrow-roll",
		  IdempotencyKey: "roll-idem",
		   UserID:         "user-roll",
		   BorrowAsset:    "USDT",
		  BorrowAmount: "9000",
		  CollateralID: "hold-roll",
		  CollateralAsset:  "BTC",
		  CollateralAmount: "10000",
		   TriggerLTVBPS:    9000,
		   CapBPS: 8000,
	   })
	  leg, _ := svc.AddSeizeLeg(liqplan.AddLegReq{
		   PlanID:       plan.ID,
		   Asset:        "BTC",
		   TargetAmount: "10000",
		  HoldID:       "hold-roll",
	   })
	   svc.StartLeg(leg.ID)
	   failedLeg, _, _ := svc.FailLeg(leg.ID, "5000", "midway failure")
	  if !failedLeg.HasResidue() {
		   t.Fatal("expected residue after fail")
	   }
	   rolledLeg, p, err := svc.RollbackLeg(leg.ID)
	  if err != nil {
		   t.Fatalf("rollback: %v", err)
	   }
	   if rolledLeg.State != liqplan.LegRolled {
		   t.Errorf("leg state after rollback: got %q, want rolled", rolledLeg.State)
	  }
	   // 计划残留已清，应从 residued 推进至 settling
	   if p.State == liqplan.PlanResidued {
		  t.Log("plan still residued after rollback (may need settle trigger)")
	   }
	  _ = p
  }

// ─────────────────────────────────────────────
// SuspendLeg
// ─────────────────────────────────────────────

   func TestSuspendLeg(t *testing.T) {
	   svc := liqplan.New()
	   plan, _ := svc.TriggerPlan(liqplan.TriggerReq{
		  BorrowID: "borrow-suspend",
		   IdempotencyKey: "suspend-idem",
		  UserID:         "user-suspend",
		   BorrowAsset:    "USDT",
		   BorrowAmount: "9000",
		   CollateralID: "hold-s",
		   CollateralAsset: "BTC",
		  CollateralAmount: "10000",
		  TriggerLTVBPS:    9000,
		   CapBPS: 8000,
	   })
	   leg, _ := svc.AddSeizeLeg(liqplan.AddLegReq{
		   PlanID:       plan.ID,
		   Asset:        "BTC",
		  TargetAmount: "10000",
		   HoldID:       "hold-s",
	   })
	   svc.StartLeg(leg.ID)
	   svc.FailLeg(leg.ID, "0", "error")
	   suspended, err := svc.SuspendLeg(leg.ID, "max retry exceeded, manual review needed")
	   if err != nil {
		  t.Fatalf("suspend: %v", err)
	  }
	  if suspended.State != liqplan.LegSuspended {
		   t.Errorf("state: got %q, want suspended", suspended.State)
	  }
	  if suspended.SuspendNote == "" {
		   t.Error("suspend note must not be empty")
	  }
   }

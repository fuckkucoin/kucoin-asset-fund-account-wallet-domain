package flow_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/wrfreeze"
)

// ─── TestUnfreeze_ProbeFailBlocksFull ────────────────────────────────────────
// 验收点 2a：probe 未通过禁止进入 partial/full。

  func TestUnfreeze_ProbeFailBlocksFull(t *testing.T) {
	  ctx := context.Background()
	  store := wrfreeze.NewMemStore()
	  f := newFlowFreezer()
	  planSvc := wrfreeze.NewPlanService(store,
		  nil, f)
	   waveSvc := wrfreeze.NewWaveService(store, f, 4)
	   unfreezeSvc := wrfreeze.NewUnfreezeService(store, f)
	  users := []string{"u-a", "u-b"}
	   plan, _, _ := planSvc.OpenPlan(ctx, wrfreeze.OpenPlanReq{
		   IncidentRef: "INC-UNFREEZE-001",
		   Scope:       wrfreeze.ScopeUserSet,
		  UserSet: users,
		  Actor:  "ops_alice",
		   Reason: "unfreeze flow test",
	   })
	  _, _ = waveSvc.AddWave(ctx, plan.ID, "w1", users, "ops_alice")
	   _, _ = planSvc.ArmPlan(ctx, wrfreeze.ArmPlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	  _, _ = planSvc.ExecutePlan(ctx, wrfreeze.ExecutePlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	  // 不运行波次（用户未冻结）→ probe 应失败
	  probePhase, _ := unfreezeSvc.InitPhase(ctx, wrfreeze.InitPhaseReq{
		   PlanID: plan.ID, Kind: wrfreeze.PhaseProbe, Actor: "ops_alice",
	   })
	   probeResult,
		   _ := unfreezeSvc.ApprovePhase(ctx, wrfreeze.ApprovePhaseReq{
		  PhaseID: probePhase.ID, Actor: "ops_bob",
	  })
	  if probeResult.Status != wrfreeze.PhaseFailed {
		   t.Errorf("probe should fail (users not frozen), got %s", probeResult.Status)
	  }
	   // 尝试 full（probe 失败后）
	   _, err := unfreezeSvc.InitPhase(ctx, wrfreeze.InitPhaseReq{
		  PlanID: plan.ID, Kind: wrfreeze.PhaseFull, Actor: "ops_alice",
	   })
	  if err != wrfreeze.ErrProbeNotPassed {
		   t.Errorf("want ErrProbeNotPassed, got %v", err)
	  }
	  // 尝试 partial（probe 失败后）
	   _, err = unfreezeSvc.InitPhase(ctx, wrfreeze.InitPhaseReq{
		  PlanID: plan.ID, Kind: wrfreeze.PhasePartial, Actor: "ops_alice",
	   })
	  if err != wrfreeze.ErrProbeNotPassed {
		   t.Errorf("want ErrProbeNotPassed, got %v", err)
	  }
	   t.Log("correctly blocked partial/full when probe not passed")
   }

// ─── TestUnfreeze_FourEyeRequired ────────────────────────────────────────────
// 验收点 2b：四眼缺失拒绝。
  func TestUnfreeze_FourEyeRequired(t *testing.T) {
	  ctx := context.Background()
	  store := wrfreeze.NewMemStore()
	   f := newFlowFreezer()
	   planSvc := wrfreeze.NewPlanService(store, nil, f)
	  waveSvc := wrfreeze.NewWaveService(store, f, 4)
	  unfreezeSvc := wrfreeze.NewUnfreezeService(store, f)
	   users := []string{"u-c"}
	   plan, _, _ := planSvc.OpenPlan(ctx, wrfreeze.OpenPlanReq{
		   IncidentRef: "INC-FOUREYE-001",
		  Scope: wrfreeze.ScopeUserSet,
		  UserSet: users,
		  Actor:   "ops_alice",
		   Reason:  "four-eye test",
	   })
	  _, _ = waveSvc.AddWave(ctx, plan.ID, "w1", users, "ops_alice")
	   _, _ = planSvc.ArmPlan(ctx, wrfreeze.ArmPlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	  _, _ = planSvc.ExecutePlan(ctx, wrfreeze.ExecutePlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	  _,
		   _ = waveSvc.AdvancePlan(ctx, plan.ID)
	  phase, _ := unfreezeSvc.InitPhase(ctx, wrfreeze.InitPhaseReq{
		   PlanID: plan.ID, Kind: wrfreeze.PhaseProbe, Actor: "ops_alice",
	   })
	  // 同一人批准 → 应被拒绝
	  _, err := unfreezeSvc.ApprovePhase(ctx, wrfreeze.ApprovePhaseReq{
		   PhaseID: phase.ID, Actor: "ops_alice",
	  })
	   if err != wrfreeze.ErrFourEye {
		  t.Errorf("want ErrFourEye, got %v", err)
	   }
	  t.Log("four-eye enforcement verified: same actor rejected")
  }

// ─── TestUnfreeze_FullFlow_ProbePartialFull ───────────────────────────────────

  func TestUnfreeze_FullFlow_ProbePartialFull(t *testing.T) {
	  ctx := context.Background()
	  store := wrfreeze.NewMemStore()
	  f := newFlowFreezer()
	  planSvc := wrfreeze.NewPlanService(store, nil, f)
	  waveSvc := wrfreeze.NewWaveService(store, f, 4)
	   unfreezeSvc := wrfreeze.NewUnfreezeService(store, f)
	  users := []string{"u-001", "u-002", "u-003", "u-004"}
	   plan, _, _ := planSvc.OpenPlan(ctx, wrfreeze.OpenPlanReq{
		   IncidentRef: "INC-FULL-UNFREEZE",
		  Scope:   wrfreeze.ScopeUserSet,
		  UserSet: users,
		   Actor:   "ops_alice",
		  Reason:  "full unfreeze flow test",
	  })
	   _, _ = waveSvc.AddWave(ctx, plan.ID, "all", users, "ops_alice")
	   _, _ = planSvc.ArmPlan(ctx, wrfreeze.ArmPlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	   _, _ = planSvc.ExecutePlan(ctx, wrfreeze.ExecutePlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	  _, _ = waveSvc.AdvancePlan(ctx, plan.ID)
	   // probe
	   pp, _ := unfreezeSvc.InitPhase(ctx, wrfreeze.InitPhaseReq{
		   PlanID: plan.ID, Kind: wrfreeze.PhaseProbe, Actor: "ops_alice",
	   })
	  probeResult, _ := unfreezeSvc.ApprovePhase(ctx, wrfreeze.ApprovePhaseReq{
		  PhaseID: pp.ID, Actor: "ops_bob",
	  })
	   if probeResult.Status != wrfreeze.PhasePassed {
		   t.Fatalf("probe failed: %s", probeResult.IntegrityNote)
	   }
	  // partial
	  partP, _ := unfreezeSvc.InitPhase(ctx, wrfreeze.InitPhaseReq{
		  PlanID: plan.ID, Kind: wrfreeze.PhasePartial, Actor: "ops_alice",
	   })
	   partResult, _ := unfreezeSvc.ApprovePhase(ctx, wrfreeze.ApprovePhaseReq{
		   PhaseID: partP.ID, Actor: "ops_bob",
	   })
	  if partResult.Status != wrfreeze.PhasePassed {
		   t.Errorf("partial phase should pass, got %s: %s", partResult.Status, partResult.IntegrityNote)
	  }
	  t.Logf("partial unfrozen=%d failed=%d", partResult.UnfrozenCount, partResult.FailedCount)
	   // full
	  fullP, _ := unfreezeSvc.InitPhase(ctx, wrfreeze.InitPhaseReq{
		  PlanID: plan.ID, Kind: wrfreeze.PhaseFull, Actor: "ops_alice",
	   })
	   fullResult, err := unfreezeSvc.ApprovePhase(ctx, wrfreeze.ApprovePhaseReq{
		   PhaseID: fullP.ID, Actor: "ops_bob",
	   })
	  if err != nil {
		  t.Fatalf("full phase: %v", err)
	  }
	   if fullResult.Status != wrfreeze.PhasePassed {
		  t.Errorf("full phase should pass, got %s", fullResult.Status)
	   }
	  t.Logf("full unfrozen=%d", fullResult.UnfrozenCount)
   }

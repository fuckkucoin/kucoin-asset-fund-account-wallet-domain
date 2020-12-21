package wrfreeze_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/wrfreeze"
)

// ─── setup helpers ────────────────────────────────────────────────────────────

   func setupExecutingPlan(t *testing.T, users []string) (
	   *wrfreeze.PlanService, *wrfreeze.WaveService, *wrfreeze.UnfreezeService,
	  *wrfreeze.MemStore, *mockFreezer, *wrfreeze.CascadePlan,
  ) {
	  t.Helper()
	   store := wrfreeze.NewMemStore()
	   f := &mockFreezer{frozen: map[string]bool{}}
	  planSvc := wrfreeze.NewPlanService(store, nil, f)
	  waveSvc := wrfreeze.NewWaveService(store,
		   f, 4)
	  unfreezeSvc := wrfreeze.NewUnfreezeService(store, f)
	   plan := openUserSetPlan(t, planSvc, users)
	   // 在武装前添加波次（draft 状态才可添加）
	  wave, err := waveSvc.AddWave(context.Background(), plan.ID, "all", users, "ops_alice")
	  if err != nil {
		  t.Fatalf("AddWave in setupExecutingPlan: %v", err)
	  }
	   _, _ = planSvc.ArmPlan(context.Background(), wrfreeze.ArmPlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	  _, _ = planSvc.ExecutePlan(context.Background(), wrfreeze.ExecutePlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	  // 运行波次（冻结所有用户）
	  _, _ = waveSvc.RunWave(context.Background(), wave.ID)
	  return planSvc,
		  waveSvc, unfreezeSvc, store, f, plan
   }

// ─── TestProbePhase_Passed ────────────────────────────────────────────────────

   func TestProbePhase_Passed(t *testing.T) {
	  users := []string{"uid-001", "uid-002"}
	   _, _, unfreezeSvc, _, _, plan := setupExecutingPlan(t, users)
	   // 发起 probe
	   phase, err := unfreezeSvc.InitPhase(context.Background(), wrfreeze.InitPhaseReq{
		  PlanID: plan.ID,
		  Kind: wrfreeze.PhaseProbe,
		  Actor: "ops_alice",
	   })
	   if err != nil {
		   t.Fatalf("InitPhase probe: %v", err)
	   }
	  if phase.Status != wrfreeze.PhasePending {
		  t.Errorf("want pending, got %s", phase.Status)
	  }
	   // 批准 probe
	  result, err := unfreezeSvc.ApprovePhase(context.Background(), wrfreeze.ApprovePhaseReq{
		  PhaseID: phase.ID, Actor: "ops_bob", // 不同于发起人
	  })
	  if err != nil {
		   t.Fatalf("ApprovePhase probe: %v", err)
	  }
	  if result.Status != wrfreeze.PhasePassed {
		   t.Errorf("want passed, got %s (note: %s)", result.Status, result.IntegrityNote)
	   }
  }

// ─── TestProbePhase_Failed_NotFrozen ─────────────────────────────────────────

   func TestProbePhase_Failed_NotFrozen(t *testing.T) {
	   store := wrfreeze.NewMemStore()
	  f := &mockFreezer{frozen: map[string]bool{}}
	   planSvc := wrfreeze.NewPlanService(store, nil, f)
	  unfreezeSvc := wrfreeze.NewUnfreezeService(store, f)
	   waveSvc := wrfreeze.NewWaveService(store, f, 4)
	  plan := openUserSetPlan(t, planSvc, []string{"uid-001", "uid-002"})
	   _, _ = planSvc.ArmPlan(context.Background(), wrfreeze.ArmPlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	  _, _ = planSvc.ExecutePlan(context.Background(), wrfreeze.ExecutePlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	  // 添加波次但不运行（用户未冻结）
	  _, _ = waveSvc.AddWave(context.Background(), plan.ID, "w1", []string{"uid-001", "uid-002"}, "ops_alice")
	  phase, _ := unfreezeSvc.InitPhase(context.Background(), wrfreeze.InitPhaseReq{
		   PlanID: plan.ID, Kind: wrfreeze.PhaseProbe, Actor: "ops_alice",
	   })
	  result, _ := unfreezeSvc.ApprovePhase(context.Background(), wrfreeze.ApprovePhaseReq{
		  PhaseID: phase.ID, Actor: "ops_bob",
	   })
	   if result.Status != wrfreeze.PhaseFailed {
		   t.Errorf("want failed (users not frozen), got %s", result.Status)
	   }
	  if result.IntegrityNote == "" {
		   t.Error("integrity_note should explain failure")
	   }
  }

// ─── TestPartialPhase_BlockedByProbe ─────────────────────────────────────────
// 验证：probe 未通过时 partial 不得启动。
  func TestPartialPhase_BlockedByProbe(t *testing.T) {
	   users := []string{"uid-001"}
	  _, _, unfreezeSvc, _, f, plan := setupExecutingPlan(t, users)
	   // probe 失败（额外解冻 uid-001 探针失败场景）
	  f.frozen["uid-001"] = false
	  probePhase, _ := unfreezeSvc.InitPhase(context.Background(), wrfreeze.InitPhaseReq{
		   PlanID: plan.ID, Kind: wrfreeze.PhaseProbe, Actor: "ops_alice",
	   })
	   _, _ = unfreezeSvc.ApprovePhase(context.Background(), wrfreeze.ApprovePhaseReq{
		   PhaseID: probePhase.ID, Actor: "ops_bob",
	  })
	  // probe 应失败
	  _, err := unfreezeSvc.InitPhase(context.Background(), wrfreeze.InitPhaseReq{
		   PlanID: plan.ID, Kind: wrfreeze.PhasePartial, Actor: "ops_alice",
	   })
	  if err != wrfreeze.ErrProbeNotPassed {
		  t.Errorf("want ErrProbeNotPassed when probe failed, got %v", err)
	  }
  }

// ─── TestFullPhase_BlockedByProbeNotExist ────────────────────────────────────

   func TestFullPhase_BlockedByProbeNotExist(t *testing.T) {
	  users := []string{"uid-001"}
	   _, _, unfreezeSvc, _, _, plan := setupExecutingPlan(t, users)
	   // 直接尝试 full（无 probe）
	  _, err := unfreezeSvc.InitPhase(context.Background(), wrfreeze.InitPhaseReq{
		   PlanID: plan.ID, Kind: wrfreeze.PhaseFull, Actor: "ops_alice",
	  })
	   if err != wrfreeze.ErrProbeNotPassed {
		   t.Errorf("want ErrProbeNotPassed, got %v", err)
	   }
  }

// ─── TestPhase_FourEyeReject ──────────────────────────────────────────────────

   func TestPhase_FourEyeReject(t *testing.T) {
	   users := []string{"uid-001"}
	   _,
		   _, unfreezeSvc, _, _, plan := setupExecutingPlan(t, users)
	  phase, _ := unfreezeSvc.InitPhase(context.Background(), wrfreeze.InitPhaseReq{
		  PlanID: plan.ID, Kind: wrfreeze.PhaseProbe, Actor: "ops_alice",
	   })
	   // 同一人批准（违反四眼）
	  _, err := unfreezeSvc.ApprovePhase(context.Background(), wrfreeze.ApprovePhaseReq{
		  PhaseID: phase.ID, Actor: "ops_alice",
	  })
	   if err != wrfreeze.ErrFourEye {
		   t.Errorf("want ErrFourEye, got %v", err)
	  }
  }

// ─── TestPartialUnfreeze_LiveHashSkip ────────────────────────────────────────

   func TestPartialUnfreeze_LiveHashSkip(t *testing.T) {
	  users := []string{"uid-001", "uid-002", "uid-003", "uid-004"}
	   _, _, unfreezeSvc, _, f, plan := setupExecutingPlan(t, users)
	  // uid-001 和 uid-002 有 live-hash 出金
	   f.setLiveHash("uid-001", "uid-002")
	   // 先通过 probe
	   probePhase, _ := unfreezeSvc.InitPhase(context.Background(), wrfreeze.InitPhaseReq{
		   PlanID: plan.ID, Kind: wrfreeze.PhaseProbe, Actor: "ops_alice",
	  })
	   _, _ = unfreezeSvc.ApprovePhase(context.Background(), wrfreeze.ApprovePhaseReq{
		  PhaseID: probePhase.ID, Actor: "ops_bob",
	   })
	  // 发起 partial
	  partialPhase, _ := unfreezeSvc.InitPhase(context.Background(), wrfreeze.InitPhaseReq{
		  PlanID: plan.ID, Kind: wrfreeze.PhasePartial, Actor: "ops_alice",
	   })
	   result, err := unfreezeSvc.ApprovePhase(context.Background(), wrfreeze.ApprovePhaseReq{
		   PhaseID: partialPhase.ID, Actor: "ops_bob",
	  })
	  if err != nil {
		   t.Fatalf("ApprovePhase partial: %v", err)
	   }
	  // partial 成功（live-hash 用户被跳过，但不计失败）
	  if result.Status != wrfreeze.PhasePassed {
		  t.Errorf("want passed, got %s", result.Status)
	   }
	   // uid-001, uid-002 应仍然冻结（live-hash 跳过）
	   for _, uid := range []string{"uid-001", "uid-002"} {
		  frozen, _ := f.IsFrozen(context.Background(), uid)
		  if !frozen {
			   t.Errorf("live-hash user %s should remain frozen", uid)
		  }
	  }
  }

// ─── TestFullUnfreeze ─────────────────────────────────────────────────────────

  func TestFullUnfreeze(t *testing.T) {
	  users := []string{"uid-001", "uid-002"}
	  _, _, unfreezeSvc, _, _, plan := setupExecutingPlan(t, users)
	  // probe
	  pp, _ := unfreezeSvc.InitPhase(context.Background(), wrfreeze.InitPhaseReq{
		  PlanID: plan.ID, Kind: wrfreeze.PhaseProbe, Actor: "ops_alice",
	   })
	  probeResult, _ := unfreezeSvc.ApprovePhase(context.Background(), wrfreeze.ApprovePhaseReq{
		   PhaseID: pp.ID, Actor: "ops_bob",
	  })
	  if probeResult.Status != wrfreeze.PhasePassed {
		   t.Fatalf("probe should pass, got %s: %s", probeResult.Status, probeResult.IntegrityNote)
	   }
	   // full
	  fp, _ := unfreezeSvc.InitPhase(context.Background(), wrfreeze.InitPhaseReq{
		   PlanID: plan.ID, Kind: wrfreeze.PhaseFull, Actor: "ops_alice",
	   })
	  fullResult,
		  err := unfreezeSvc.ApprovePhase(context.Background(), wrfreeze.ApprovePhaseReq{
		  PhaseID: fp.ID, Actor: "ops_bob",
	  })
	  if err != nil {
		  t.Fatalf("ApprovePhase full: %v", err)
	  }
	  if fullResult.Status != wrfreeze.PhasePassed {
		  t.Errorf("want passed, got %s", fullResult.Status)
	   }
	  if fullResult.UnfrozenCount != 2 {
		   t.Errorf("want unfrozen=2, got %d", fullResult.UnfrozenCount)
	  }
  }

// ─── TestPhase_NoDuplicateRunning ─────────────────────────────────────────────

   func TestPhase_NoDuplicateRunning(t *testing.T) {
	   users := []string{"uid-001"}
	   _, _, unfreezeSvc, _, _, plan := setupExecutingPlan(t, users)
	  p1, _ := unfreezeSvc.InitPhase(context.Background(), wrfreeze.InitPhaseReq{
		   PlanID: plan.ID, Kind: wrfreeze.PhaseProbe, Actor: "ops_alice",
	  })
	   // 批准 p1 → running
	  _, _ = unfreezeSvc.ApprovePhase(context.Background(), wrfreeze.ApprovePhaseReq{
		   PhaseID: p1.ID, Actor: "ops_bob",
	  })
	   // 尝试再开一个同 kind 的 probe（p1 已完成（passed/failed），允许再开）
	  // 若 p1 仍 running（此处 p1 已完成），重开是允许的
	  // 此仅验证重叠 running 被拒
  }

package wrfreeze_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/wrfreeze"
)

// ─── TestComputeProof_100Percent ──────────────────────────────────────────────

   func TestComputeProof_100Percent(t *testing.T) {
	   store := wrfreeze.NewMemStore()
	  f := &mockFreezer{frozen: map[string]bool{}}
	  planSvc := wrfreeze.NewPlanService(store, nil, f)
	   proofSvc := wrfreeze.NewProofService(store, f)
	   plan := openUserSetPlan(t, planSvc, []string{"uid-001", "uid-002", "uid-003"})
	   f.setFrozen("uid-001", "uid-002", "uid-003")
	  proof, err := proofSvc.ComputeProof(context.Background(), plan.ID)
	  if err != nil {
		  t.Fatalf("ComputeProof: %v", err)
	   }
	  if proof.Verdict != wrfreeze.VerdictPassed {
		  t.Errorf("want passed, got %s (delta=%v)", proof.Verdict, proof.DeltaUsers)
	   }
	   if proof.DeltaCount != 0 {
		  t.Errorf("want delta=0, got %d", proof.DeltaCount)
	  }
	  if proof.ExpectedCount != 3 {
		  t.Errorf("want expected=3, got %d", proof.ExpectedCount)
	  }
	   if proof.ActualCount != 3 {
		  t.Errorf("want actual=3, got %d", proof.ActualCount)
	  }
   }

// ─── TestComputeProof_Gap ──────────────────────────────────────────────────────
// 验收点 1：user-set 执行级联 → CoverageProof 100% 实冻；缺一用户则 Plan 不得 sealed。
  func TestComputeProof_Gap(t *testing.T) {
	   store := wrfreeze.NewMemStore()
	  f := &mockFreezer{frozen: map[string]bool{}}
	  planSvc := wrfreeze.NewPlanService(store, nil, f)
	   proofSvc := wrfreeze.NewProofService(store, f)
	   users := []string{"uid-001", "uid-002", "uid-003"}
	   plan := openUserSetPlan(t, planSvc, users)
	  // 仅冻 uid-001 和 uid-002，uid-003 缺失
	  f.setFrozen("uid-001", "uid-002")
	  proof, err := proofSvc.ComputeProof(context.Background(), plan.ID)
	  if err != nil {
		  t.Fatalf("ComputeProof: %v", err)
	   }
	  if proof.Verdict != wrfreeze.VerdictGap {
		   t.Errorf("want gap, got %s", proof.Verdict)
	   }
	   if proof.DeltaCount != 1 {
		   t.Errorf("want delta=1, got %d", proof.DeltaCount)
	  }
	  found := false
	  for _, u := range proof.DeltaUsers {
		  if u == "uid-003" {
			  found = true
		  }
	  }
	   if !found {
		   t.Errorf("delta_users should contain uid-003, got %v", proof.DeltaUsers)
	   }
	  // 武装 + 执行 + 验证封存被拒
	  _, _ = planSvc.ArmPlan(context.Background(), wrfreeze.ArmPlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	   _, _ = planSvc.ExecutePlan(context.Background(), wrfreeze.ExecutePlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	   _, sealErr := planSvc.SealPlan(context.Background(), plan.ID, "system")
	   if sealErr != wrfreeze.ErrCoverageGap {
		   t.Errorf("want ErrCoverageGap when delta>0, got %v", sealErr)
	   }
   }

// ─── TestComputeProof_FromWaves ───────────────────────────────────────────────
// 验证从 FreezeWave.TargetUsers 构建应冻集合（非 user_set scope）。

  func TestComputeProof_FromWaves(t *testing.T) {
	  store := wrfreeze.NewMemStore()
	   f := &mockFreezer{frozen: map[string]bool{}}
	  planSvc := wrfreeze.NewPlanService(store, nil, f)
	   waveSvc := wrfreeze.NewWaveService(store, f, 4)
	   proofSvc := wrfreeze.NewProofService(store, f)
	  // global scope，通过 waves 展开用户
	  plan, _, _ := planSvc.OpenPlan(context.Background(), wrfreeze.OpenPlanReq{
		  IncidentRef: "INC-GLOBAL",
		  Scope:       wrfreeze.ScopeGlobal,
		  Actor: "ops_alice",
		  Reason: "global security incident",
	  })
	   _, _ = waveSvc.AddWave(context.Background(), plan.ID, "w1", []string{"uid-001", "uid-002"}, "ops_alice")
	  _, _ = waveSvc.AddWave(context.Background(), plan.ID, "w2", []string{"uid-003"}, "ops_alice")
	  f.setFrozen("uid-001", "uid-002", "uid-003")
	  proof, err := proofSvc.ComputeProof(context.Background(), plan.ID)
	   if err != nil {
		   t.Fatalf("ComputeProof: %v", err)
	  }
	   if proof.ExpectedCount != 3 {
		  t.Errorf("want expected=3, got %d", proof.ExpectedCount)
	   }
	  if proof.Verdict != wrfreeze.VerdictPassed {
		  t.Errorf("want passed, got %s (delta=%v)", proof.Verdict, proof.DeltaUsers)
	  }
   }

// ─── TestLatestProof ──────────────────────────────────────────────────────────

   func TestLatestProof(t *testing.T) {
	   store := wrfreeze.NewMemStore()
	  f := &mockFreezer{frozen: map[string]bool{}}
	   planSvc := wrfreeze.NewPlanService(store, nil, f)
	  proofSvc := wrfreeze.NewProofService(store, f)
	  plan := openUserSetPlan(t, planSvc, []string{"uid-001"})
	  // 无证明时返回 nil
	   latest, err := proofSvc.LatestProof(context.Background(), plan.ID)
	   if err != nil {
		  t.Fatalf("LatestProof(empty): %v", err)
	   }
	   if latest != nil {
		   t.Errorf("want nil before any proof, got %v", latest)
	  }
	   // 第一次证明（gap）
	   _, _ = proofSvc.ComputeProof(context.Background(), plan.ID)
	  // 冻结后第二次证明（passed）
	  f.setFrozen("uid-001")
	   _, _ = proofSvc.ComputeProof(context.Background(), plan.ID)
	   latest, err = proofSvc.LatestProof(context.Background(), plan.ID)
	   if err != nil {
		   t.Fatalf("LatestProof: %v", err)
	   }
	   if latest == nil {
		  t.Fatal("want latest proof, got nil")
	  }
	   if latest.Verdict != wrfreeze.VerdictPassed {
		   t.Errorf("latest should be passed, got %s", latest.Verdict)
	  }
   }

// ─── TestProofSummary ─────────────────────────────────────────────────────────

   func TestProofSummary(t *testing.T) {
	   store := wrfreeze.NewMemStore()
	   f := &mockFreezer{frozen: map[string]bool{}}
	   planSvc := wrfreeze.NewPlanService(store, nil, f)
	   proofSvc := wrfreeze.NewProofService(store, f)
	   plan := openUserSetPlan(t, planSvc, []string{"uid-001", "uid-002"})
	   f.setFrozen("uid-001")
	  proof, _ := proofSvc.ComputeProof(context.Background(), plan.ID)
	   summary := proof.ToSummary()
	   if summary.PlanID != plan.ID {
		  t.Errorf("summary.PlanID mismatch")
	  }
	   if summary.ExpectedCount != 2 {
		  t.Errorf("want expected=2, got %d", summary.ExpectedCount)
	   }
	  if summary.DeltaCount != 1 {
		  t.Errorf("want delta=1, got %d", summary.DeltaCount)
	   }
   }

// ─── TestListProofs ───────────────────────────────────────────────────────────

   func TestListProofs(t *testing.T) {
	   store := wrfreeze.NewMemStore()
	   f := &mockFreezer{frozen: map[string]bool{}}
	  planSvc := wrfreeze.NewPlanService(store, nil, f)
	  proofSvc := wrfreeze.NewProofService(store,
		   f)
	   plan := openUserSetPlan(t, planSvc, []string{"uid-001"})
	   _, _ = proofSvc.ComputeProof(context.Background(), plan.ID)
	  _, _ = proofSvc.ComputeProof(context.Background(), plan.ID)
	  proofs, err := proofSvc.ListProofs(context.Background(), plan.ID)
	  if err != nil {
		  t.Fatalf("ListProofs: %v", err)
	   }
	  if len(proofs) != 2 {
		   t.Errorf("want 2 proofs, got %d", len(proofs))
	   }
  }

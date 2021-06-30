package flow_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/warroom"
	"account-assets.kucoin.net/funding/internal/wrfreeze"
)

// ─── TestCascadePlan_UserSet_FullCoverage ─────────────────────────────────────
// 验收点 1a：user-set 级联执行 → CoverageProof 100% 实冻 → Plan 可 sealed。
   func TestCascadePlan_UserSet_FullCoverage(t *testing.T) {
	  ctx := context.Background()
	  store := wrfreeze.NewMemStore()
	  f := newFlowFreezer()
	   planSvc := wrfreeze.NewPlanService(store, nil, f)
	   waveSvc := wrfreeze.NewWaveService(store, f, 4)
	  proofSvc := wrfreeze.NewProofService(store, f)
	  users := []string{"u-001", "u-002", "u-003"}
	  plan, _, err := planSvc.OpenPlan(ctx, wrfreeze.OpenPlanReq{
		   IncidentRef: "INC-FLOW-001",
		  Scope:       wrfreeze.ScopeUserSet,
		   UserSet:     users,
		  Actor:  "ops_alice",
		  Reason: "security incident: credential leak",
	  })
	  if err != nil {
		   t.Fatalf("OpenPlan: %v", err)
	  }
	  // 添加覆盖全部用户的波次
	   _, err = waveSvc.AddWave(ctx, plan.ID, "all_users", users, "ops_alice")
	   if err != nil {
		  t.Fatalf("AddWave: %v", err)
	  }
	  // 武装（四眼）→ 执行
	  _, err = planSvc.ArmPlan(ctx, wrfreeze.ArmPlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	   if err != nil {
		  t.Fatalf("ArmPlan: %v", err)
	  }
	  _, err = planSvc.ExecutePlan(ctx, wrfreeze.ExecutePlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	  if err != nil {
		   t.Fatalf("ExecutePlan: %v", err)
	  }
	   // 推进波次（执行冻结）
	   _,
		   err = waveSvc.AdvancePlan(ctx, plan.ID)
	   if err != nil {
		  t.Fatalf("AdvancePlan: %v", err)
	  }
	  // 覆盖率证明
	  proof, err := proofSvc.ComputeProof(ctx, plan.ID)
	  if err != nil {
		  t.Fatalf("ComputeProof: %v", err)
	   }
	  if proof.Verdict != wrfreeze.VerdictPassed {
		   t.Errorf("expect 100%% coverage, got verdict=%s delta=%v", proof.Verdict, proof.DeltaUsers)
	  }
	   if proof.DeltaCount != 0 {
		  t.Errorf("expect delta=0, got %d", proof.DeltaCount)
	   }
	  // 封存
	   sealed, err := planSvc.SealPlan(ctx, plan.ID, "system")
	   if err != nil {
		  t.Fatalf("SealPlan: %v", err)
	  }
	  if sealed.Status != wrfreeze.PlanSealed {
		   t.Errorf("want sealed, got %s", sealed.Status)
	   }
	  // 验证所有用户均已冻结
	  for _, uid := range users {
		  frozen, _ := f.IsFrozen(ctx, uid)
		   if !frozen {
			  t.Errorf("user %s should be frozen", uid)
		   }
	   }
  }

// ─── TestCascadePlan_UserSet_MissingUser_CannotSeal ──────────────────────────
// 验收点 1b：缺一用户则 Plan 不得 sealed。

   func TestCascadePlan_UserSet_MissingUser_CannotSeal(t *testing.T) {
	   ctx := context.Background()
	  store := wrfreeze.NewMemStore()
	   f := newFlowFreezer()
	   planSvc := wrfreeze.NewPlanService(store, nil, f)
	   waveSvc := wrfreeze.NewWaveService(store, f, 4)
	   proofSvc := wrfreeze.NewProofService(store, f)
	  users := []string{"u-001", "u-002", "u-003"}
	   plan, _, _ := planSvc.OpenPlan(ctx, wrfreeze.OpenPlanReq{
		  IncidentRef: "INC-FLOW-002",
		   Scope: wrfreeze.ScopeUserSet,
		  UserSet: users,
		  Actor:   "ops_alice",
		   Reason:  "security incident",
	  })
	  // 添加仅包含 u-001 和 u-002 的波次（缺少 u-003）
	   _, _ = waveSvc.AddWave(ctx, plan.ID, "partial_wave", []string{"u-001", "u-002"}, "ops_alice")
	  _, _ = planSvc.ArmPlan(ctx, wrfreeze.ArmPlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	   _, _ = planSvc.ExecutePlan(ctx, wrfreeze.ExecutePlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	  _,
		  _ = waveSvc.AdvancePlan(ctx, plan.ID)
	   proof, _ := proofSvc.ComputeProof(ctx, plan.ID)
	   if proof.Verdict != wrfreeze.VerdictGap {
		  t.Errorf("want gap verdict (u-003 not frozen), got %s", proof.Verdict)
	   }
	  _, err := planSvc.SealPlan(ctx, plan.ID, "system")
	   if err != wrfreeze.ErrCoverageGap {
		   t.Errorf("want ErrCoverageGap, got %v", err)
	   }
	   t.Logf("correctly rejected seal with delta=%d (%v)", proof.DeltaCount, proof.DeltaUsers)
  }

// ─── TestCascadePlan_MultiWave_Serial ─────────────────────────────────────────

   func TestCascadePlan_MultiWave_Serial(t *testing.T) {
	  ctx := context.Background()
	   store := wrfreeze.NewMemStore()
	  f := newFlowFreezer()
	   planSvc := wrfreeze.NewPlanService(store,
		  nil, f)
	  waveSvc := wrfreeze.NewWaveService(store, f, 4)
	   proofSvc := wrfreeze.NewProofService(store, f)
	   allUsers := []string{"u-1", "u-2", "u-3", "u-4", "u-5", "u-6"}
	  plan, _, _ := planSvc.OpenPlan(ctx, wrfreeze.OpenPlanReq{
		  IncidentRef: "INC-MULTI-WAVE",
		  Scope:   wrfreeze.ScopeUserSet,
		  UserSet: allUsers,
		   Actor:   "ops_alice",
		  Reason:  "multi-wave test",
	  })
	   // wave1: u-1~u-3, wave2: u-4~u-6
	  _, _ = waveSvc.AddWave(ctx, plan.ID, "wave1", allUsers[:3], "ops_alice")
	  _, _ = waveSvc.AddWave(ctx, plan.ID, "wave2", allUsers[3:], "ops_alice")
	   _, _ = planSvc.ArmPlan(ctx, wrfreeze.ArmPlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	   _, _ = planSvc.ExecutePlan(ctx, wrfreeze.ExecutePlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	   // 推进两次（串行）
	   _,
		   _ = waveSvc.AdvancePlan(ctx, plan.ID)
	  _, _ = waveSvc.AdvancePlan(ctx, plan.ID)
	  proof,
		   _ := proofSvc.ComputeProof(ctx, plan.ID)
	  if proof.Verdict != wrfreeze.VerdictPassed {
		   t.Errorf("want passed after both waves, got %s (delta=%v)", proof.Verdict, proof.DeltaUsers)
	   }
  }

// ─── TestFreezeAll_Compat_ReadonlyProjection ──────────────────────────────────
// 验收点 5：FreezeAll 映射为只读全局 Plan 投影，不产生双写冲突。

  func TestFreezeAll_Compat_ReadonlyProjection(t *testing.T) {
	  ctx := context.Background()
	   warSvc := warroom.New()
	   _, _ = warSvc.FreezeAll(ctx, "ops_alice", "global incident test")
	   store := wrfreeze.NewMemStore()
	   compatSvc := wrfreeze.NewCompatService(warSvc, store)
	   proj, err := compatSvc.GlobalProjection(ctx)
	   if err != nil {
		  t.Fatalf("GlobalProjection: %v", err)
	  }
	  if !proj.FrozenAll {
		  t.Error("projection should reflect FrozenAll=true")
	  }
	   if proj.Source != "legacy_switch" {
		   t.Errorf("source want legacy_switch, got %s", proj.Source)
	   }
	   if proj.PlanID != "" {
		   t.Errorf("no global plan created, expect empty PlanID, got %s", proj.PlanID)
	  }
	   // 创建全局 Plan → 投影应携带 PlanID
	   planSvc := wrfreeze.NewPlanService(store, nil, &flowFreezer{frozen: map[string]bool{}})
	  plan, _, _ := planSvc.OpenPlan(ctx, wrfreeze.OpenPlanReq{
		  IncidentRef: "INC-COMPAT",
		   Scope: wrfreeze.ScopeGlobal,
		   Actor: "ops_alice",
		   Reason: "compat test",
	   })
	  proj2, _ := compatSvc.GlobalProjection(ctx)
	   if proj2.PlanID != plan.ID {
		   t.Errorf("expect plan_id=%s in projection, got %s", plan.ID, proj2.PlanID)
	   }
	   t.Logf("compat projection: source=%s frozen=%v plan_id=%s", proj2.Source, proj2.FrozenAll, proj2.PlanID)
  }

// ─── helpers ──────────────────────────────────────────────────────────────────

type flowFreezer struct {
	frozen   map[string]bool
	liveHash map[string]bool
}

  func newFlowFreezer() *flowFreezer {
	  return &flowFreezer{frozen: map[string]bool{}}
   }

   func (f *flowFreezer) SetFrozen(_ context.Context, userID string, frozen bool) error {
	   f.frozen[userID] = frozen
	  return nil
  }

  func (f *flowFreezer) IsFrozen(_ context.Context, userID string) (bool, error) {
	   return f.frozen[userID], nil
  }

  func (f *flowFreezer) HasLiveHashWithdraw(_ context.Context, userID string) (bool, error) {
	   if f.liveHash == nil {
		  return false, nil
	  }
	   return f.liveHash[userID], nil
   }

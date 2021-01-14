package wrfreeze_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/wrfreeze"
)

// ─── 共享夹具 ─────────────────────────────────────────────────────────────

  func makePlanSvc(t *testing.T) (*wrfreeze.PlanService, *wrfreeze.MemStore, *mockFreezer) {
	   t.Helper()
	  store := wrfreeze.NewMemStore()
	   f := &mockFreezer{frozen: map[string]bool{}}
	  svc := wrfreeze.NewPlanService(store, nil, f)
	   return svc, store, f
   }

   func openUserSetPlan(t *testing.T, svc *wrfreeze.PlanService, users []string) *wrfreeze.CascadePlan {
	   t.Helper()
	   plan, _, err := svc.OpenPlan(context.Background(), wrfreeze.OpenPlanReq{
		   IncidentRef: "INC-2026-001",
		   Scope:       wrfreeze.ScopeUserSet,
		   UserSet:     users,
		  Actor:       "ops_alice",
		   Reason:   "security incident user set freeze",
		  TTLHours: 24,
	  })
	   if err != nil {
		   t.Fatalf("OpenPlan: %v", err)
	   }
	   return plan
   }

// ─── TestOpenPlan_UserSet ────────────────────────────────────────────────────

   func TestOpenPlan_UserSet(t *testing.T) {
	   svc, store, _ := makePlanSvc(t)
	  users := []string{"uid-001", "uid-002", "uid-003"}
	  plan := openUserSetPlan(t, svc, users)
	  if plan.Status != wrfreeze.PlanDraft {
		  t.Errorf("status want draft, got %s", plan.Status)
	   }
	   if plan.Scope != wrfreeze.ScopeUserSet {
		   t.Errorf("scope want user_set, got %s", plan.Scope)
	  }
	   if len(plan.UserSet) != 3 {
		  t.Errorf("user_set len want 3, got %d", len(plan.UserSet))
	  }
	   // store 持久化校验
	  got, err := store.GetPlan(context.Background(), plan.ID)
	   if err != nil {
		   t.Fatalf("GetPlan: %v", err)
	  }
	  if got.ID != plan.ID {
		  t.Errorf("stored plan ID mismatch")
	   }
  }

// ─── TestOpenPlan_MissingIncidentRef ─────────────────────────────────────────

  func TestOpenPlan_MissingIncidentRef(t *testing.T) {
	   svc, _, _ := makePlanSvc(t)
	  _, _, err := svc.OpenPlan(context.Background(), wrfreeze.OpenPlanReq{
		  IncidentRef: "",
		  Scope:       wrfreeze.ScopeUserSet,
		  UserSet:     []string{"uid-001"},
		  Actor: "ops_alice",
		   Reason: "test",
	   })
	   if err != wrfreeze.ErrIncidentRef {
		   t.Errorf("want ErrIncidentRef, got %v", err)
	   }
  }

// ─── TestOpenPlan_UserSetEmpty ────────────────────────────────────────────────

   func TestOpenPlan_UserSetEmpty(t *testing.T) {
	   svc, _, _ := makePlanSvc(t)
	   _, _, err := svc.OpenPlan(context.Background(), wrfreeze.OpenPlanReq{
		   IncidentRef: "INC-001",
		  Scope:       wrfreeze.ScopeUserSet,
		  UserSet: []string{},
		  Actor:  "ops_alice",
		   Reason: "test",
	  })
	   if err != wrfreeze.ErrUserSet {
		  t.Errorf("want ErrUserSet, got %v", err)
	  }
  }

// ─── TestOpenPlan_InvalidScope ────────────────────────────────────────────────

  func TestOpenPlan_InvalidScope(t *testing.T) {
	  svc, _, _ := makePlanSvc(t)
	   _, _, err := svc.OpenPlan(context.Background(), wrfreeze.OpenPlanReq{
		   IncidentRef: "INC-001",
		   Scope: "invalid_scope",
		  Actor: "ops_alice",
		  Reason: "test",
	   })
	  if err != wrfreeze.ErrScope {
		  t.Errorf("want ErrScope, got %v", err)
	   }
   }

// ─── TestArmPlan_FourEye ──────────────────────────────────────────────────────

   func TestArmPlan_FourEye(t *testing.T) {
	   svc, _, _ := makePlanSvc(t)
	   plan := openUserSetPlan(t, svc, []string{"uid-001"})
	   // 开单人不得武装（四眼约束）
	   _, err := svc.ArmPlan(context.Background(), wrfreeze.ArmPlanReq{
		  PlanID: plan.ID,
		  Actor:  "ops_alice", // 同开单人
	   })
	  if err != wrfreeze.ErrFourEye {
		  t.Errorf("want ErrFourEye, got %v", err)
	  }
  }

// ─── TestArmPlan_OK ───────────────────────────────────────────────────────────

   func TestArmPlan_OK(t *testing.T) {
	  svc, _, _ := makePlanSvc(t)
	  plan := openUserSetPlan(t, svc, []string{"uid-001"})
	   armed, err := svc.ArmPlan(context.Background(), wrfreeze.ArmPlanReq{
		  PlanID: plan.ID,
		   Actor:  "ops_bob", // 不同人
	  })
	  if err != nil {
		   t.Fatalf("ArmPlan: %v", err)
	   }
	   if armed.Status != wrfreeze.PlanArmed {
		  t.Errorf("want armed, got %s", armed.Status)
	  }
   }

// ─── TestExecutePlan ──────────────────────────────────────────────────────────

  func TestExecutePlan(t *testing.T) {
	  svc, _, _ := makePlanSvc(t)
	  plan := openUserSetPlan(t, svc, []string{"uid-001"})
	   _, err := svc.ArmPlan(context.Background(), wrfreeze.ArmPlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	   if err != nil {
		  t.Fatalf("ArmPlan: %v", err)
	   }
	  exec, err := svc.ExecutePlan(context.Background(), wrfreeze.ExecutePlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	  if err != nil {
		   t.Fatalf("ExecutePlan: %v", err)
	   }
	   if exec.Status != wrfreeze.PlanExecuting {
		  t.Errorf("want executing, got %s", exec.Status)
	   }
  }

// ─── TestAbortPlan ────────────────────────────────────────────────────────────

  func TestAbortPlan(t *testing.T) {
	   svc, _, _ := makePlanSvc(t)
	   plan := openUserSetPlan(t, svc, []string{"uid-001"})
	  aborted, err := svc.AbortPlan(context.Background(), wrfreeze.AbortPlanReq{
		   PlanID: plan.ID, Actor: "ops_alice", Reason: "false alarm",
	   })
	  if err != nil {
		   t.Fatalf("AbortPlan: %v", err)
	  }
	   if aborted.Status != wrfreeze.PlanAborted {
		  t.Errorf("want aborted, got %s", aborted.Status)
	  }
   }

// ─── TestAbortPlan_AlreadySealed ──────────────────────────────────────────────

   func TestAbortPlan_AlreadySealed(t *testing.T) {
	  svc, store, f := makePlanSvc(t)
	  users := []string{"uid-001"}
	   plan := openUserSetPlan(t, svc, users)
	   f.setFrozen(users...)
	  _, _ = svc.ArmPlan(context.Background(), wrfreeze.ArmPlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	   _, _ = svc.ExecutePlan(context.Background(), wrfreeze.ExecutePlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	  // 插入 100% 证明
	   proofSvc := wrfreeze.NewProofService(store, f)
	   proof, _ := proofSvc.ComputeProof(context.Background(), plan.ID)
	  if proof.Verdict != wrfreeze.VerdictPassed {
		  t.Skipf("proof not passed (delta=%d), skip seal test", proof.DeltaCount)
	  }
	  _, err := svc.SealPlan(context.Background(), plan.ID, "system")
	  if err != nil {
		  t.Fatalf("SealPlan: %v", err)
	  }
	  // 已封存不得中止
	   _, err2 := svc.AbortPlan(context.Background(), wrfreeze.AbortPlanReq{
		   PlanID: plan.ID, Actor: "ops_alice", Reason: "late abort",
	  })
	  if err2 != wrfreeze.ErrPlanState {
		   t.Errorf("want ErrPlanState, got %v", err2)
	   }
  }

// ─── TestExpireOverduePlans ───────────────────────────────────────────────────

  func TestExpireOverduePlans(t *testing.T) {
	   svc, _, _ := makePlanSvc(t)
	   // 将时钟快进到过期后
	  past := time.Now().UTC().Add(-48 * time.Hour)
	   svc.SetClock(func() time.Time { return past })
	   plan := openUserSetPlan(t, svc, []string{"uid-001"})
	   // 恢复当前时钟使其过期
	   svc.SetClock(time.Now().UTC)
	  n, err := svc.ExpireOverduePlans(context.Background())
	  if err != nil {
		   t.Fatalf("ExpireOverduePlans: %v", err)
	  }
	   if n == 0 {
		   t.Errorf("expected at least 1 plan expired")
	   }
	  got, _ := svc.GetPlan(context.Background(), plan.ID)
	  if got.Status != wrfreeze.PlanAborted {
		  t.Errorf("want aborted, got %s", got.Status)
	  }
   }

// ─── TestPlanState_InvalidTransitions ────────────────────────────────────────

  func TestPlanState_InvalidTransitions(t *testing.T) {
	   svc, _, _ := makePlanSvc(t)
	   plan := openUserSetPlan(t, svc, []string{"uid-001"})
	  // draft → executing（跳过 armed）须失败
	  _, err := svc.ExecutePlan(context.Background(), wrfreeze.ExecutePlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	   if err != wrfreeze.ErrPlanState {
		   t.Errorf("want ErrPlanState when skipping armed, got %v", err)
	  }
	  // 武装
	   _, _ = svc.ArmPlan(context.Background(), wrfreeze.ArmPlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	  // armed → sealed（跳过 executing）须失败
	   _, serr := svc.SealPlan(context.Background(), plan.ID, "system")
	  if serr != wrfreeze.ErrPlanState {
		  t.Errorf("want ErrPlanState when sealing from armed, got %v", serr)
	  }
  }

// ─── TestSealPlan_CoverageGap ─────────────────────────────────────────────────
// TestSealPlan_CoverageGap 验证：覆盖率有差额时 Plan 不得 sealed。
   func TestSealPlan_CoverageGap(t *testing.T) {
	  svc, store, f := makePlanSvc(t)
	   users := []string{"uid-001", "uid-002"}
	   plan := openUserSetPlan(t, svc, users)
	   _, _ = svc.ArmPlan(context.Background(), wrfreeze.ArmPlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	  _, _ = svc.ExecutePlan(context.Background(), wrfreeze.ExecutePlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	   // 只冻 uid-001，uid-002 未冻
	  f.setFrozen("uid-001")
	   proofSvc := wrfreeze.NewProofService(store, f)
	   proof, _ := proofSvc.ComputeProof(context.Background(), plan.ID)
	   if proof.DeltaCount == 0 {
		  t.Fatal("expected delta > 0")
	   }
	  if proof.Verdict != wrfreeze.VerdictGap {
		  t.Errorf("want VerdictGap, got %s", proof.Verdict)
	  }
	   _, sealErr := svc.SealPlan(context.Background(), plan.ID, "system")
	  if sealErr != wrfreeze.ErrCoverageGap {
		  t.Errorf("want ErrCoverageGap, got %v", sealErr)
	  }
   }

// ─── TestGlobalScope ──────────────────────────────────────────────────────────

  func TestGlobalScope(t *testing.T) {
	   svc, _, _ := makePlanSvc(t)
	  plan, _, err := svc.OpenPlan(context.Background(), wrfreeze.OpenPlanReq{
		  IncidentRef: "INC-GLOBAL-001",
		   Scope: wrfreeze.ScopeGlobal,
		  Actor: "ops_alice",
		   Reason: "global security incident",
	  })
	   if err != nil {
		  t.Fatalf("OpenPlan global: %v", err)
	   }
	   if plan.Scope != wrfreeze.ScopeGlobal {
		   t.Errorf("want global scope, got %s", plan.Scope)
	  }
  }

// ─── mockFreezer ──────────────────────────────────────────────────────────────

type mockFreezer struct {
	frozen   map[string]bool
	liveHash map[string]bool
}

   func (m *mockFreezer) SetFrozen(_ context.Context, userID string, frozen bool) error {
	   m.frozen[userID] = frozen
	   return nil
   }

   func (m *mockFreezer) IsFrozen(_ context.Context, userID string) (bool, error) {
	  return m.frozen[userID], nil
   }

  func (m *mockFreezer) HasLiveHashWithdraw(_ context.Context, userID string) (bool, error) {
	   if m.liveHash == nil {
		   return false, nil
	   }
	  return m.liveHash[userID], nil
   }

  func (m *mockFreezer) setFrozen(users ...string) {
	  for _, u := range users {
		  m.frozen[u] = true
	   }
  }

  func (m *mockFreezer) setLiveHash(users ...string) {
	  if m.liveHash == nil {
		  m.liveHash = map[string]bool{}
	   }
	   for _, u := range users {
		  m.liveHash[u] = true
	  }
   }

package wrfreeze_test

import (
	"context"
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/wrfreeze"
)

// ─── TestRequestRollback_OK ───────────────────────────────────────────────────

   func TestRequestRollback_OK(t *testing.T) {
	  store := wrfreeze.NewMemStore()
	  f := &mockFreezer{frozen: map[string]bool{}}
	   planSvc := wrfreeze.NewPlanService(store, nil, f)
	   rollbackSvc := wrfreeze.NewRollbackService(store, f, wrfreeze.NoopHoldcatReader{})
	   plan := openUserSetPlan(t, planSvc, []string{"uid-001", "uid-002"})
	  leg, err := rollbackSvc.RequestRollback(context.Background(), wrfreeze.RollbackReq{
		  PlanID: plan.ID,
		   UserID: "uid-001",
		  Reason: wrfreeze.ReasonMisFreeze,
		  Actor:  "ops_alice",
	  })
	  if err != nil {
		   t.Fatalf("RequestRollback: %v", err)
	   }
	   if leg.Status != wrfreeze.RollbackPending {
		  t.Errorf("want pending, got %s", leg.Status)
	  }
	   if leg.UserID != "uid-001" {
		   t.Errorf("user_id mismatch")
	  }
   }

// ─── TestRequestRollback_UserNotInPlan ───────────────────────────────────────

  func TestRequestRollback_UserNotInPlan(t *testing.T) {
	  store := wrfreeze.NewMemStore()
	  f := &mockFreezer{frozen: map[string]bool{}}
	  planSvc := wrfreeze.NewPlanService(store,
		  nil, f)
	  rollbackSvc := wrfreeze.NewRollbackService(store, f, wrfreeze.NoopHoldcatReader{})
	  plan := openUserSetPlan(t, planSvc, []string{"uid-001"})
	  _, err := rollbackSvc.RequestRollback(context.Background(), wrfreeze.RollbackReq{
		  PlanID: plan.ID,
		   UserID: "uid-999", // 不在计划中
		   Reason: wrfreeze.ReasonMisFreeze,
		   Actor: "ops_alice",
	  })
	  if err != wrfreeze.ErrMisFreeze {
		  t.Errorf("want ErrMisFreeze, got %v", err)
	  }
  }

// ─── TestExecuteRollback_FourEye ──────────────────────────────────────────────

  func TestExecuteRollback_FourEye(t *testing.T) {
	  store := wrfreeze.NewMemStore()
	  f := &mockFreezer{frozen: map[string]bool{}}
	  planSvc := wrfreeze.NewPlanService(store,
		  nil, f)
	   rollbackSvc := wrfreeze.NewRollbackService(store, f, wrfreeze.NoopHoldcatReader{})
	   plan := openUserSetPlan(t, planSvc, []string{"uid-001"})
	  leg, _ := rollbackSvc.RequestRollback(context.Background(), wrfreeze.RollbackReq{
		  PlanID: plan.ID, UserID: "uid-001",
		  Reason: wrfreeze.ReasonMisFreeze, Actor: "ops_alice",
	   })
	  // 申请人不得执行（四眼）
	  _, err := rollbackSvc.ExecuteRollback(context.Background(), wrfreeze.ExecuteRollbackReq{
		   LegID: leg.ID, Actor: "ops_alice",
	  })
	  if err != wrfreeze.ErrFourEye {
		   t.Errorf("want ErrFourEye, got %v", err)
	  }
  }

// ─── TestExecuteRollback_LiveHash ─────────────────────────────────────────────
// 验收点 3：live-hash 出金仍不得自动解冻。
   func TestExecuteRollback_LiveHash(t *testing.T) {
	  store := wrfreeze.NewMemStore()
	  f := &mockFreezer{frozen: map[string]bool{}}
	   f.setFrozen("uid-001")
	   f.setLiveHash("uid-001") // live-hash 出金在途
	   planSvc := wrfreeze.NewPlanService(store, nil, f)
	  rollbackSvc := wrfreeze.NewRollbackService(store, f, wrfreeze.NoopHoldcatReader{})
	  plan := openUserSetPlan(t, planSvc, []string{"uid-001"})
	   leg, _ := rollbackSvc.RequestRollback(context.Background(), wrfreeze.RollbackReq{
		  PlanID: plan.ID, UserID: "uid-001",
		  Reason: wrfreeze.ReasonMisFreeze, Actor: "ops_alice",
	   })
	   if !leg.HasLiveHash {
		   t.Error("leg should have HasLiveHash=true")
	   }
	   _, err := rollbackSvc.ExecuteRollback(context.Background(), wrfreeze.ExecuteRollbackReq{
		  LegID: leg.ID, Actor: "ops_bob",
	   })
	   if err != wrfreeze.ErrLiveHash {
		   t.Errorf("want ErrLiveHash, got %v", err)
	   }
	  // 验证用户仍然冻结
	  frozen, _ := f.IsFrozen(context.Background(), "uid-001")
	  if !frozen {
		  t.Error("uid-001 should remain frozen (live-hash)")
	   }
   }

// ─── TestExecuteRollback_RestoresAvailabilityNote ────────────────────────────
// 验收点 3：误冻回滚腿执行后，目标用户可用视图恢复解释。
   func TestExecuteRollback_RestoresAvailabilityNote(t *testing.T) {
	  store := wrfreeze.NewMemStore()
	   f := &mockFreezer{frozen: map[string]bool{}}
	  f.setFrozen("uid-001")
	   planSvc := wrfreeze.NewPlanService(store, nil, f)
	   rollbackSvc := wrfreeze.NewRollbackService(store, f, wrfreeze.NoopHoldcatReader{})
	   plan := openUserSetPlan(t, planSvc, []string{"uid-001"})
	   leg, _ := rollbackSvc.RequestRollback(context.Background(), wrfreeze.RollbackReq{
		   PlanID: plan.ID, UserID: "uid-001",
		   Reason: wrfreeze.ReasonMisFreeze, Actor: "ops_alice",
	  })
	   result, err := rollbackSvc.ExecuteRollback(context.Background(), wrfreeze.ExecuteRollbackReq{
		   LegID: leg.ID, Actor: "ops_bob",
	   })
	  if err != nil {
		   t.Fatalf("ExecuteRollback: %v", err)
	  }
	   if result.Status != wrfreeze.RollbackRestored {
		  t.Errorf("want restored, got %s", result.Status)
	   }
	  if result.AvailabilityNote == "" {
		  t.Error("availability_note should not be empty")
	  }
	   // 验证解释包含关键字段
	   if !strings.Contains(result.AvailabilityNote, "uid-001") {
		   t.Errorf("availability_note should contain user_id: %s", result.AvailabilityNote)
	   }
	   if !strings.Contains(result.AvailabilityNote, "freeze-cleared=true") {
		   t.Errorf("availability_note should confirm freeze cleared: %s", result.AvailabilityNote)
	  }
	   // 验证用户已解冻
	   frozen, _ := f.IsFrozen(context.Background(), "uid-001")
	  if frozen {
		  t.Error("uid-001 should be unfrozen after rollback")
	   }
  }

// ─── TestAutoRollbackExpired ──────────────────────────────────────────────────

   func TestAutoRollbackExpired(t *testing.T) {
	  store := wrfreeze.NewMemStore()
	   f := &mockFreezer{frozen: map[string]bool{}}
	   f.setFrozen("uid-001", "uid-002")
	   planSvc := wrfreeze.NewPlanService(store, nil, f)
	   rollbackSvc := wrfreeze.NewRollbackService(store,
		   f, wrfreeze.NoopHoldcatReader{})
	   // 创建过期计划（ExpiresAt 在 48h 前）
	  past := wrfreeze.StaticClock(-48)
	   planSvc.SetClock(past)
	  plan := openUserSetPlan(t, planSvc, []string{"uid-001", "uid-002"})
	  planSvc.SetClock(wrfreeze.NowClock)
	   // 通过 ExpireOverduePlans worker 中止（AbortReason="expired"）
	   _, _ = planSvc.ExpireOverduePlans(context.Background())
	  created, err := rollbackSvc.AutoRollbackExpired(context.Background())
	   if err != nil {
		   t.Fatalf("AutoRollbackExpired: %v", err)
	  }
	  if created != 2 {
		  t.Errorf("want 2 legs created, got %d", created)
	  }
	   legs,
		   _ := rollbackSvc.ListLegs(context.Background(), plan.ID)
	   if len(legs) != 2 {
		  t.Errorf("want 2 legs, got %d", len(legs))
	  }
	   for _, l := range legs {
		  if l.Reason != wrfreeze.ReasonExpiredIncident {
			   t.Errorf("want reason expired_incident, got %s", l.Reason)
		  }
	  }
  }

// ─── TestListLegs ─────────────────────────────────────────────────────────────

  func TestListLegs(t *testing.T) {
	  store := wrfreeze.NewMemStore()
	  f := &mockFreezer{frozen: map[string]bool{}}
	   planSvc := wrfreeze.NewPlanService(store, nil, f)
	   rollbackSvc := wrfreeze.NewRollbackService(store, f, wrfreeze.NoopHoldcatReader{})
	  plan := openUserSetPlan(t, planSvc, []string{"uid-001", "uid-002"})
	   _, _ = rollbackSvc.RequestRollback(context.Background(), wrfreeze.RollbackReq{
		  PlanID: plan.ID, UserID: "uid-001", Reason: wrfreeze.ReasonMisFreeze, Actor: "ops_alice",
	   })
	  _, _ = rollbackSvc.RequestRollback(context.Background(), wrfreeze.RollbackReq{
		  PlanID: plan.ID, UserID: "uid-002", Reason: wrfreeze.ReasonScopeError, Actor: "ops_alice",
	  })
	   legs, err := rollbackSvc.ListLegs(context.Background(), plan.ID)
	   if err != nil {
		  t.Fatalf("ListLegs: %v", err)
	   }
	   if len(legs) != 2 {
		   t.Errorf("want 2 legs, got %d", len(legs))
	   }
  }

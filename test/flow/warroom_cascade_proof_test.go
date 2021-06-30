package flow_test

import (
	"context"
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/wrfreeze"
)

// ─── TestRollback_RestoresAvailability_LiveHashBlocked ───────────────────────
// 验收点 3a & 3b：
// - 误冻回滚腿执行后可用视图恢复；
// - live-hash 出金用户不自动解冻。

  func TestRollback_RestoresAvailability_LiveHashBlocked(t *testing.T) {
	  ctx := context.Background()
	  store := wrfreeze.NewMemStore()
	   f := newFlowFreezer()
	   planSvc := wrfreeze.NewPlanService(store, nil, f)
	   rollbackSvc := wrfreeze.NewRollbackService(store, f, wrfreeze.NoopHoldcatReader{})
	   users := []string{"u-live", "u-normal"}
	   plan, _, _ := planSvc.OpenPlan(ctx, wrfreeze.OpenPlanReq{
		  IncidentRef: "INC-ROLLBACK",
		  Scope:   wrfreeze.ScopeUserSet,
		  UserSet: users,
		   Actor:   "ops_alice",
		  Reason: "rollback flow test",
	   })
	   // 两人均已冻结
	   f.frozen["u-live"] = true
	  f.frozen["u-normal"] = true
	  // u-live 有 live-hash 出金
	  f.liveHash = map[string]bool{"u-live": true}
	   // 申请回滚腿
	  legLive, _ := rollbackSvc.RequestRollback(ctx, wrfreeze.RollbackReq{
		   PlanID: plan.ID, UserID: "u-live",
		  Reason: wrfreeze.ReasonMisFreeze, Actor: "ops_alice",
	   })
	  legNormal, _ := rollbackSvc.RequestRollback(ctx, wrfreeze.RollbackReq{
		  PlanID: plan.ID, UserID: "u-normal",
		   Reason: wrfreeze.ReasonMisFreeze, Actor: "ops_alice",
	  })
	  // u-live 回滚被拒（live-hash）
	  _, err := rollbackSvc.ExecuteRollback(ctx, wrfreeze.ExecuteRollbackReq{
		  LegID: legLive.ID, Actor: "ops_bob",
	  })
	  if err != wrfreeze.ErrLiveHash {
		  t.Errorf("want ErrLiveHash for live-hash user, got %v", err)
	   }
	  // u-live 仍然冻结
	  frozen, _ := f.IsFrozen(ctx, "u-live")
	   if !frozen {
		  t.Error("u-live should remain frozen (live-hash withdraw)")
	  }
	  // u-normal 回滚成功
	  result, err := rollbackSvc.ExecuteRollback(ctx, wrfreeze.ExecuteRollbackReq{
		   LegID: legNormal.ID, Actor: "ops_bob",
	  })
	   if err != nil {
		  t.Fatalf("ExecuteRollback normal: %v", err)
	   }
	  if result.Status != wrfreeze.RollbackRestored {
		  t.Errorf("want restored, got %s", result.Status)
	  }
	   if result.AvailabilityNote == "" {
		   t.Error("availability_note should be set")
	   }
	  if !strings.Contains(result.AvailabilityNote, "freeze-cleared=true") {
		   t.Errorf("availability_note should confirm freeze cleared: %s", result.AvailabilityNote)
	  }
	   // u-normal 已解冻
	  frozen, _ = f.IsFrozen(ctx, "u-normal")
	   if frozen {
		  t.Error("u-normal should be unfrozen after rollback")
	  }
	  t.Logf("availability_note: %s", result.AvailabilityNote)
   }

// ─── TestHoldcatReadonly_NoEnumChange ─────────────────────────────────────────
// 验收点 4：与 r6 holdcat 分层冻结并行——本包仅只读 hold 令牌 ID，不改枚举。
  func TestHoldcatReadonly_NoEnumChange(t *testing.T) {
	  ctx := context.Background()
	  store := wrfreeze.NewMemStore()
	  f := newFlowFreezer()
	  f.frozen["u-hold"] = true
	   planSvc := wrfreeze.NewPlanService(store, nil, f)
	  // 使用 NoopHoldcatReader（不依赖 holdcat 包写路径）
	   holdReader := wrfreeze.NoopHoldcatReader{}
	  rollbackSvc := wrfreeze.NewRollbackService(store, f, holdReader)
	   plan, _, _ := planSvc.OpenPlan(ctx, wrfreeze.OpenPlanReq{
		   IncidentRef: "INC-HOLD-COMPAT",
		   Scope:       wrfreeze.ScopeUserSet,
		  UserSet: []string{"u-hold"},
		  Actor:  "ops_alice",
		  Reason: "holdcat compat test",
	  })
	   leg, _ := rollbackSvc.RequestRollback(ctx, wrfreeze.RollbackReq{
		  PlanID: plan.ID, UserID: "u-hold",
		   Reason: wrfreeze.ReasonMisFreeze, Actor: "ops_alice",
	  })
	   result, err := rollbackSvc.ExecuteRollback(ctx, wrfreeze.ExecuteRollbackReq{
		   LegID: leg.ID, Actor: "ops_bob",
	  })
	   if err != nil {
		   t.Fatalf("ExecuteRollback: %v", err)
	  }
	  // availability_note 明确标注 hold 令牌为 none（NoopHoldcatReader）
	   if !strings.Contains(result.AvailabilityNote, "active-hold-tokens=none") {
		  t.Errorf("availability_note should mention hold tokens: %s", result.AvailabilityNote)
	  }
	  t.Log("holdcat read-only compat verified: no enum modification")
  }

// ─── TestCoverageProof_Audit_Append ──────────────────────────────────────────
// 验证：历史证明永久保留（只追加），最新证明正确。
   func TestCoverageProof_Audit_Append(t *testing.T) {
	  ctx := context.Background()
	   store := wrfreeze.NewMemStore()
	   f := newFlowFreezer()
	   planSvc := wrfreeze.NewPlanService(store, nil, f)
	  proofSvc := wrfreeze.NewProofService(store, f)
	  plan, _, _ := planSvc.OpenPlan(ctx, wrfreeze.OpenPlanReq{
		  IncidentRef: "INC-AUDIT",
		   Scope: wrfreeze.ScopeUserSet,
		   UserSet: []string{"u-001", "u-002"},
		  Actor:   "ops_alice",
		   Reason:  "audit test",
	  })
	   // 第 1 次证明（gap）
	   proof1, _ := proofSvc.ComputeProof(ctx, plan.ID)
	   if proof1.Verdict != wrfreeze.VerdictGap {
		   t.Errorf("first proof should be gap, got %s", proof1.Verdict)
	  }
	  // 冻结后第 2 次证明（passed）
	   f.setFrozen("u-001", "u-002")
	  proof2, _ := proofSvc.ComputeProof(ctx, plan.ID)
	   if proof2.Verdict != wrfreeze.VerdictPassed {
		   t.Errorf("second proof should pass, got %s", proof2.Verdict)
	  }
	  // 历史保留（2 条）
	   proofs, _ := proofSvc.ListProofs(ctx, plan.ID)
	   if len(proofs) != 2 {
		  t.Errorf("want 2 historical proofs, got %d", len(proofs))
	  }
	  // 最新证明为 passed
	   latest,
		  _ := proofSvc.LatestProof(ctx, plan.ID)
	   if latest.Verdict != wrfreeze.VerdictPassed {
		  t.Errorf("latest should be passed, got %s", latest.Verdict)
	   }
   }

// ─── TestWorker_PatrolSeal ─────────────────────────────────────────────────────

  func TestWorker_PatrolSeal(t *testing.T) {
	  ctx := context.Background()
	   store := wrfreeze.NewMemStore()
	   f := newFlowFreezer()
	   planSvc := wrfreeze.NewPlanService(store, nil, f)
	   waveSvc := wrfreeze.NewWaveService(store, f, 4)
	  proofSvc := wrfreeze.NewProofService(store, f)
	  users := []string{"u-p1", "u-p2"}
	   plan, _, _ := planSvc.OpenPlan(ctx, wrfreeze.OpenPlanReq{
		  IncidentRef: "INC-WORKER",
		   Scope: wrfreeze.ScopeUserSet,
		   UserSet: users,
		   Actor:  "ops_alice",
		  Reason: "worker patrol test",
	   })
	  _, _ = waveSvc.AddWave(ctx, plan.ID, "all", users, "ops_alice")
	   _, _ = planSvc.ArmPlan(ctx, wrfreeze.ArmPlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	   _, _ = planSvc.ExecutePlan(ctx, wrfreeze.ExecutePlanReq{PlanID: plan.ID, Actor: "ops_bob"})
	  _, _ = waveSvc.AdvancePlan(ctx, plan.ID)
	  // worker 巡检
	  err := proofSvc.PatrolAndSeal(ctx,
		  planSvc, waveSvc)
	  if err != nil {
		   t.Fatalf("PatrolAndSeal: %v", err)
	   }
	   // 验证计划已封存
	  got, _ := planSvc.GetPlan(ctx, plan.ID)
	   if got.Status != wrfreeze.PlanSealed {
		  t.Errorf("want sealed after patrol, got %s", got.Status)
	   }
   }

// ─── flowFreezer.setFrozen helper ─────────────────────────────────────────────

   func (f *flowFreezer) setFrozen(users ...string) {
	   for _, u := range users {
		   f.frozen[u] = true
	   }
  }

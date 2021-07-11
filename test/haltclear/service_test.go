package haltclear_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
	"account-assets.kucoin.net/funding/internal/haltclear"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ---- 辅助 -----------------------------------------------------------

   func newTestService(t *testing.T,
	  diffCount int) (*haltclear.Service, *custodyhalt.Gate) {
	  t.Helper()
	   kv := persist.NewMemKV()
	  gate := custodyhalt.New(kv)
	   quorumStore := custodyhalt.NewQuorumStore(kv)
	   forcedAuditStore := custodyhalt.NewForcedAuditStore(kv)
	   guard := custodyhalt.NewQuorumGuard(quorumStore)
	  gate.BindClearCheck(guard)
	  caseRepo := haltclear.NewMemCaseRepo()
	  legRepo := haltclear.NewMemLegRepo()
	  diffQuery := &haltclear.StaticDiffQuerier{Count: diffCount}
	  detector := haltclear.NewFalseClearDetector(gate, diffQuery, caseRepo,
		  haltclear.ScopeTarget{Scope: "global"},
	   )
	   rollback := haltclear.NewReEngageRollback(gate, caseRepo, legRepo, diffQuery)
	   svc := haltclear.NewService(haltclear.ServiceConfig{
		   Gate: gate,
		  Quorum:      quorumStore,
		   ForcedAudit: forcedAuditStore,
		  Detector:    detector,
		  Rollback:    rollback,
		  CaseRepo: caseRepo,
		   LegRepo: legRepo,
	  })
	  return svc, gate
  }

// ---- 四眼清闸完整流程 -----------------------------------------------
// TestService_QuorumFlowFull 端到端：propose → approve1 → approve2 → CloseProof 有效。
  func TestService_QuorumFlowFull(t *testing.T) {
	   svc, gate := newTestService(t, 0)
	   ctx := context.Background()
	   // 先熔断。
	   if err := gate.Engage(ctx, "system", "test halt"); err != nil {
		   t.Fatal(err)
	  }
	   // 提案。
	  plan, err := svc.ProposePlan(ctx, haltclear.ProposePlanReq{
		   PlanID: "svc-plan-001",
		   Scope:    "global",
		   Proposer: "alice",
		   Reason: "hot wallet ok",
	   })
	  if err != nil {
		  t.Fatalf("propose: %v", err)
	  }
	   if plan.State != custodyhalt.PlanStateProposed {
		  t.Errorf("state=%s want proposed", plan.State)
	  }
	  // 第一签。
	  plan, err = svc.FirstApprovePlan(ctx, "svc-plan-001", "bob")
	   if err != nil {
		  t.Fatalf("approve1: %v", err)
	   }
	   if plan.State != custodyhalt.PlanStateQuorum {
		  t.Errorf("state=%s want quorum", plan.State)
	   }
	  // 第二签（零差异）。
	  proof, err := svc.SecondApprovePlan(ctx, "svc-plan-001", "carol", 0, "recon batch clear")
	  if err != nil {
		   t.Fatalf("approve2: %v", err)
	   }
	  if proof == nil {
		   t.Fatal("expected proof")
	   }
	  if err := custodyhalt.VerifyProof(*proof); err != nil {
		  t.Errorf("proof invalid: %v", err)
	   }
	   // 读取已 cleared 计划。
	   stored, err := svc.GetPlan(ctx, "svc-plan-001")
	   if err != nil {
		   t.Fatal(err)
	   }
	   if stored.State != custodyhalt.PlanStateCleared {
		   t.Errorf("state=%s want cleared", stored.State)
	  }
	   if stored.CloseProofID == "" {
		  t.Error("close_proof_id should be set")
	  }
	   // 读取证明并校验。
	  storedProof, err := svc.GetProof(ctx, stored.CloseProofID)
	   if err != nil {
		  t.Fatal(err)
	   }
	  if storedProof.OpenDiffCount != 0 {
		   t.Errorf("open_diff_count=%d want 0", storedProof.OpenDiffCount)
	  }
  }

// TestService_QuorumReject 否决流程。
  func TestService_QuorumReject(t *testing.T) {
	  svc, _ := newTestService(t, 3)
	  ctx := context.Background()
	  _, err := svc.ProposePlan(ctx, haltclear.ProposePlanReq{
		   PlanID:   "plan-reject",
		   Scope:    "global",
		  Proposer: "alice",
		  Reason: "try clear",
		   OpenDiffs: 3,
	   })
	   if err != nil {
		  t.Fatal(err)
	  }
	  plan, err := svc.RejectPlan(ctx, "plan-reject", "bob", "diffs still open")
	   if err != nil {
		  t.Fatal(err)
	  }
	  if plan.State != custodyhalt.PlanStateRejected {
		  t.Errorf("state=%s want rejected", plan.State)
	   }
   }

// ---- 强制清闸升级 -----------------------------------------------
// TestService_ForcedClear_WithAudit 强制清闸留审计记录。

  func TestService_ForcedClear_WithAudit(t *testing.T) {
	   svc, gate := newTestService(t, 3)
	   ctx := context.Background()
	   if err := gate.Engage(ctx, "system", "halt for forced test"); err != nil {
		   t.Fatal(err)
	  }
	   rec, mc, err := svc.ForcedClear(ctx, haltclear.ForcedClearReq{
		   Scope: "global",
		  Actor: "alice",
		   Reason:        "emergency hot recovery",
		   ReasonCode:    "emergency_hot_recovery",
		   OpenDiffCount: 3,
		   DiffKeys: []string{"k1", "k2", "k3"},
	  })
	  if err != nil {
		   t.Fatalf("forced clear: %v", err)
	  }
	   if rec == nil {
		  t.Fatal("expected audit record")
	  }
	   if !rec.MisClearRisk {
		   t.Error("expected misclear_risk=true")
	  }
	   // 差异 > 0 时应产出 MisClearCase。
	  if mc == nil {
		  t.Error("expected misclear case for open diffs")
	  }
	   if mc != nil && mc.Trigger != haltclear.TriggerForcedClear {
		  t.Errorf("trigger=%s want forced_clear", mc.Trigger)
	   }
	  // Gate 应已解除。
	  halted, _ := gate.Halted(ctx)
	  if halted {
		   t.Error("gate should be cleared after forced clear")
	   }
   }

// TestService_ForcedClear_InvalidReasonCode 缺理由码时拒绝。

  func TestService_ForcedClear_InvalidReasonCode(t *testing.T) {
	  svc, gate := newTestService(t, 0)
	   ctx := context.Background()
	   if err := gate.Engage(ctx, "system", "halt"); err != nil {
		  t.Fatal(err)
	   }
	   _, _, err := svc.ForcedClear(ctx, haltclear.ForcedClearReq{
		  Scope: "global",
		  Actor: "alice",
		  Reason:     "trying to bypass",
		   ReasonCode: "",
	   })
	  if err == nil {
		  t.Error("expected error for empty reason_code")
	   }
	  // Gate 应仍处于熔断。
	   halted, _ := gate.Halted(ctx)
	  if !halted {
		   t.Error("gate should still be halted after failed forced clear")
	   }
   }

// ---- 误清检测与再熔断端到端 -----------------------------------------------
// TestService_MisClearDetectAndReengage 端到端：检测误清 → 调度再熔断 → 确认执行 → 关闭。
  func TestService_MisClearDetectAndReengage(t *testing.T) {
	   svc, gate := newTestService(t, 3) // 差异数=3
	  ctx := context.Background()
	   // 注意：熔断已解除（Flag.On=false）；差异仍开（count=3）→ 误清场景。
	  // 直接手工创建一个 MisClearCase（ detector 产出）。
	   now := time.Now().UTC()
	   mc := &haltclear.MisClearCase{
		   MisclearID: "svc-mc-001",
		  Scope:   "global",
		   State:   haltclear.MisClearStateOpen,
		  Trigger: haltclear.TriggerOpenDiffScan,
		   DetectedAt: now,
		  OpenDiffCount: 3,
		   CreatedAt:     now,
		   UpdatedAt:     now,
	   }
	   // 通过 caseRepo 直接插入（绕过 detector）。
	   // 由于 Service 不暴露 caseRepo，使用 ListMisClearCases + ScheduleReEngage。
	   _ = mc // 通过 ForcedClear 产出案例。
	   if err := gate.Engage(ctx, "system", "setup halt"); err != nil {
		  t.Fatal(err)
	  }
	   rec, mc2, err := svc.ForcedClear(ctx, haltclear.ForcedClearReq{
		   Scope: "global",
		  Actor: "ops-alice",
		   Reason:        "drill test",
		  ReasonCode:    "drill_test",
		  OpenDiffCount: 3,
		   DiffKeys:      []string{"k1", "k2", "k3"},
	  })
	   if err != nil {
		   t.Fatalf("forced clear: %v", err)
	   }
	  _ = rec
	  if mc2 == nil {
		  t.Fatal("expected misclear case from forced clear")
	   }
	   // 调度再熔断（半自动）。
	   leg, err := svc.ScheduleReEngage(ctx, haltclear.ScheduleReEngageReq{
		   MisclearID: mc2.MisclearID,
		  ActorID:    "ops-alice",
		  Reason: "manual reengage confirm",
		   IsAuto: false,
	   })
	   if err != nil {
		   t.Fatalf("schedule: %v", err)
	  }
	  if leg.State != haltclear.ReEngageStatePending {
		  t.Errorf("leg state=%s want pending", leg.State)
	  }
	   // 确认执行再熔断。
	  proof, err := svc.ConfirmReEngage(ctx, leg.LegID)
	   if err != nil {
		  t.Fatalf("confirm: %v", err)
	  }
	  if !haltclear.VerifyRollbackProof(proof) {
		   t.Error("proof invalid")
	  }
	  // Gate 应已恢复熔断。
	   halted, _ := gate.Halted(ctx)
	  if !halted {
		  t.Error("gate should be halted after reengage")
	   }
	   // 查询案例状态。
	  updated, err := svc.GetMisClearCase(ctx, mc2.MisclearID)
	   if err != nil {
		  t.Fatal(err)
	   }
	  if updated.State != haltclear.MisClearStateReengaged {
		   t.Errorf("case state=%s want reengaged", updated.State)
	  }
	   // 人工关闭案例。
	   if err := svc.CloseCase(ctx, mc2.MisclearID, "ops-alice", "resolved by recon team"); err != nil {
		   t.Fatalf("close case: %v", err)
	  }
	   closed, _ := svc.GetMisClearCase(ctx, mc2.MisclearID)
	  if closed.State != haltclear.MisClearStateClosed {
		  t.Errorf("case state=%s want closed", closed.State)
	  }
   }

// TestService_ListCasesFilter 列出案例按 state 过滤。

   func TestService_ListCasesFilter(t *testing.T) {
	   svc, gate := newTestService(t, 2)
	   ctx := context.Background()
	   // 强制清闸产出两个案例（两次不同 scope）。
	  for i := 0; i < 2; i++ {
		  if err := gate.Engage(ctx, "system", "halt"); err != nil {
			   t.Fatal(err)
		  }
		  _, _, err := svc.ForcedClear(ctx, haltclear.ForcedClearReq{
			  Scope:      "global",
			   Actor:      "alice",
			  Reason:     "drill",
			  ReasonCode: "drill_test",
			  OpenDiffCount: 2,
		   })
		  if err != nil {
			  t.Fatalf("forced clear %d: %v", i, err)
		   }
	   }
	   cases, err := svc.ListMisClearCases(ctx, haltclear.CaseFilter{
		   State: haltclear.MisClearStateOpen,
		  Limit: 10,
	   })
	  if err != nil {
		   t.Fatal(err)
	   }
	   // 第二次 ForcedClear 不产出新案例（CountOpen > 0）。
	   if len(cases) != 1 {
		  t.Errorf("expected 1 open case, got %d", len(cases))
	   }
   }

// TestService_VerifyProof Service 层证明校验接口。

   func TestService_VerifyProof(t *testing.T) {
	   svc, _ := newTestService(t, 0)
	   leg := &haltclear.ReEngageLeg{
		  LegID: "leg-svc-vp",
		  MisclearID: "mc-svc-vp",
		   Scope:      "global",
		  ActorID:    "system",
		  Reason:     "test",
	   }
	   proof := haltclear.NewRollbackProof(leg, 0)
	  if !svc.VerifyProof(leg.LegID, proof) {
		  t.Error("expected valid proof")
	   }
	  tampered := *proof
	  tampered.MisclearID = "other"
	   if svc.VerifyProof(leg.LegID, &tampered) {
		  t.Error("expected invalid for tampered proof")
	  }
  }

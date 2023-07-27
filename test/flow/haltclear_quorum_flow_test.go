package flow

import (
	"context"
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
	"account-assets.kucoin.net/funding/internal/haltclear"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ---- 辅助 -------------------------------------------------------

type hcTestEnv struct {
	svc  *haltclear.Service
	gate *custodyhalt.Gate
	caseRepo *haltclear.MemCaseRepo
}

   func newHCEnv(diffCount int) *hcTestEnv {
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
		  Detector: detector,
		  Rollback: rollback,
		   CaseRepo: caseRepo,
		   LegRepo: legRepo,
	  })
	   return &hcTestEnv{svc: svc, gate: gate, caseRepo: caseRepo}
  }

// ---- -----------------------------------------------------------
// TestHaltClear_QuorumFlowFull 四眼完整路径验证：需两名不同操作人审批才能清闸。

   func TestHaltClear_QuorumFlowFull(t *testing.T) {
	  ctx := context.Background()
	  env := newHCEnv(0) // 无差异 — 允许清闸
	   if err := env.gate.Engage(ctx, "system", "test halt"); err != nil {
		   t.Fatal(err)
	   }
	   plan, err := env.svc.ProposePlan(ctx, haltclear.ProposePlanReq{
		   PlanID:   "plan-001",
		  Scope:    "global",
		   Proposer: "alice",
		  Reason: "清闸",
	   })
	   if err != nil {
		  t.Fatalf("ProposePlan: %v", err)
	  }
	   if plan.State != custodyhalt.PlanStateProposed {
		   t.Errorf("expected proposed, got %s", plan.State)
	   }
	  // bob 第一签（≠ proposer alice）。
	  _, err = env.svc.FirstApprovePlan(ctx, "plan-001", "bob")
	   if err != nil {
		   t.Fatalf("FirstApprovePlan: %v", err)
	   }
	   // carol 第二签（≠ bob）。
	   proof, err := env.svc.SecondApprovePlan(ctx, "plan-001", "carol", 0, "")
	   if err != nil {
		   t.Fatalf("SecondApprovePlan: %v", err)
	   }
	   if proof.ProofType != custodyhalt.ProofTypeNormal {
		   t.Errorf("expected normal proof, got %s", proof.ProofType)
	  }
	   if proof.OpenDiffCount != 0 {
		  t.Errorf("expected zero diffs in proof, got %d", proof.OpenDiffCount)
	  }
   }

// TestHaltClear_SelfApprove_Rejected proposer 不得自我签发第一审批（单点突破防御）。
   func TestHaltClear_SelfApprove_Rejected(t *testing.T) {
	   ctx := context.Background()
	   env := newHCEnv(0)
	  if err := env.gate.Engage(ctx, "system", "test"); err != nil {
		   t.Fatal(err)
	   }
	   _, err := env.svc.ProposePlan(ctx, haltclear.ProposePlanReq{
		   PlanID: "plan-self", Scope: "global", Proposer: "alice", Reason: "test",
	  })
	  if err != nil {
		   t.Fatal(err)
	   }
	  // alice 尝试为自己的提案签第一审批。
	   _, err = env.svc.FirstApprovePlan(ctx, "plan-self", "alice")
	   if err == nil {
		   t.Fatal("self-approval must be rejected")
	   }
	   if !errors.Is(err, custodyhalt.ErrSameApprover) {
		   t.Errorf("expected ErrSameApprover, got %v", err)
	   }
   }

// TestHaltClear_SameApproverBothRounds_Rejected 同一人不得同时担任 approver1 和 approver2。
   func TestHaltClear_SameApproverBothRounds_Rejected(t *testing.T) {
	   ctx := context.Background()
	   env := newHCEnv(0)
	  if err := env.gate.Engage(ctx, "system", "test"); err != nil {
		   t.Fatal(err)
	   }
	  _, err := env.svc.ProposePlan(ctx, haltclear.ProposePlanReq{
		   PlanID: "plan-dup", Scope: "global", Proposer: "alice", Reason: "test",
	   })
	   if err != nil {
		  t.Fatal(err)
	   }
	   if _, err = env.svc.FirstApprovePlan(ctx, "plan-dup", "bob"); err != nil {
		  t.Fatal(err)
	   }
	  // bob 尝试再次签第二轮（违反四眼规则）。
	   _, err = env.svc.SecondApprovePlan(ctx, "plan-dup", "bob", 0, "")
	  if err == nil {
		  t.Fatal("same approver for both rounds must be rejected")
	  }
	  if !errors.Is(err, custodyhalt.ErrSameApprover) {
		  t.Errorf("expected ErrSameApprover, got %v", err)
	  }
  }

// TestHaltClear_OpenDiffs_BlocksNormalClear 存在未关差异时，正常清闸应被拒绝。
  func TestHaltClear_OpenDiffs_BlocksNormalClear(t *testing.T) {
	  ctx := context.Background()
	   env := newHCEnv(3) // 3 个未关差异
	   if err := env.gate.Engage(ctx, "system", "test"); err != nil {
		  t.Fatal(err)
	   }
	  _, err := env.svc.ProposePlan(ctx, haltclear.ProposePlanReq{
		  PlanID: "plan-diff", Scope: "global", Proposer: "alice", Reason: "test", OpenDiffs: 3,
	  })
	  if err != nil {
		   t.Fatal(err)
	  }
	  if _, err = env.svc.FirstApprovePlan(ctx, "plan-diff", "bob"); err != nil {
		   t.Fatal(err)
	   }
	  // 尝试二审，实测差异数仍为 3 — 必须拒绝。
	   _, err = env.svc.SecondApprovePlan(ctx, "plan-diff", "carol", 3, "diff-summary")
	   if err == nil {
		  t.Fatal("open diffs must block normal clear")
	  }
	  if !errors.Is(err, custodyhalt.ErrNoOpenDiffs) {
		  t.Errorf("expected ErrNoOpenDiffs, got %v", err)
	  }
  }

// TestHaltClear_FalseClearDetector_CreatesMisClearCase
// 强制清闸后若仍存在未关差异，检测器应生成 MisClearCase（再熔断触发源）。
   func TestHaltClear_FalseClearDetector_CreatesMisClearCase(t *testing.T) {
	  ctx := context.Background()
	   env := newHCEnv(2) // 仍有 2 个未关差异
	   if err := env.gate.Engage(ctx, "system", "pre-test halt"); err != nil {
		  t.Fatal(err)
	   }
	  // 强制清闸（service 层，附带误清检测）。
	   _, mc, err := env.svc.ForcedClear(ctx, haltclear.ForcedClearReq{
		  Scope: "global",
		   Actor: "ops-lead",
		   Reason:        "incident cleared by war room",
		  ReasonCode:    custodyhalt.ForcedReasonIncidentCleared,
		   OpenDiffCount: 2,
		  DiffKeys: []string{"diff-key-1", "diff-key-2"},
	   })
	   if err != nil {
		   t.Fatalf("ForcedClear: %v", err)
	  }
	  if mc == nil {
		  t.Fatal("expected MisClearCase to be created when open diffs > 0 after forced clear")
	   }
	   if mc.State != haltclear.MisClearStateOpen {
		  t.Errorf("expected open misclear case, got %s", mc.State)
	   }
	  if mc.OpenDiffCount != 2 {
		  t.Errorf("expected 2 open diffs in case, got %d", mc.OpenDiffCount)
	  }
   }

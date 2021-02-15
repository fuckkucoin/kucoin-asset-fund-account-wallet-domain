package custodyhalt_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ---- 辅助 -----------------------------------------------------------

   func newQuorumStore() *custodyhalt.QuorumStore {
	  kv := persist.NewMemKV()
	   return custodyhalt.NewQuorumStore(kv)
  }

  func newGateWithQuorum(t *testing.T) (*custodyhalt.Gate, *custodyhalt.QuorumStore) {
	   t.Helper()
	   kv := persist.NewMemKV()
	   gate := custodyhalt.New(kv)
	   qs := custodyhalt.NewQuorumStore(kv)
	  guard := custodyhalt.NewQuorumGuard(qs)
	   gate.BindClearCheck(guard)
	   return gate, qs
   }

  func makeProposal(qs *custodyhalt.QuorumStore, planID, proposer string) error {
	   return qs.ProposeClearPlan(context.Background(), custodyhalt.ClearPlan{
		   PlanID: planID,
		  Scope: "global",
		   Proposer: proposer,
		   Reason:   "hot wallet ok",
	   })
  }

// ---- ProposePlan -----------------------------------------------

   func TestProposePlan_OK(t *testing.T) {
	  qs := newQuorumStore()
	   if err := makeProposal(qs, "plan-001", "alice"); err != nil {
		  t.Fatalf("propose: %v", err)
	   }
	   p, err := qs.GetPlan(context.Background(), "plan-001")
	  if err != nil {
		  t.Fatalf("get plan: %v", err)
	   }
	  if p.State != custodyhalt.PlanStateProposed {
		   t.Errorf("state=%s want proposed", p.State)
	  }
	   if p.Proposer != "alice" {
		  t.Errorf("proposer=%s want alice", p.Proposer)
	   }
   }

var proposeErrCases = []struct {
	name     string
	planID   string
	proposer string
	reason  string
	wantErr error
}{
	{"empty plan_id", "", "alice", "reason", custodyhalt.ErrPlanIDRequired},
	{"empty proposer", "p1", "", "reason", custodyhalt.ErrApproverRequired},
	{"empty reason", "p2", "alice", "", custodyhalt.ErrReason},
}

   func TestProposePlan_Errors(t *testing.T) {
	   for _, tc := range proposeErrCases {
		   tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  qs := newQuorumStore()
			   err := qs.ProposeClearPlan(context.Background(), custodyhalt.ClearPlan{
				  PlanID:   tc.planID,
				  Proposer: tc.proposer,
				  Reason:   tc.reason,
				   Scope: "global",
			  })
			  if !errors.Is(err, tc.wantErr) {
				   t.Errorf("got %v want %v", err, tc.wantErr)
			   }
		   })
	  }
  }

// ---- FirstApprove -----------------------------------------------

var firstApproveTable = []struct {
	name string
	approver string
	wantState string
	wantErr   error
}{
	{"ok distinct approver", "bob", custodyhalt.PlanStateQuorum, nil},
	{"same as proposer", "alice", "", custodyhalt.ErrSameApprover},
	{"empty approver", "", "", custodyhalt.ErrApproverRequired},
}

  func TestFirstApprove(t *testing.T) {
	   for _, tc := range firstApproveTable {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  qs := newQuorumStore()
			  if err := makeProposal(qs, "plan-fa", "alice"); err != nil {
				  t.Fatal(err)
			  }
			  err := qs.FirstApprove(context.Background(), "plan-fa", tc.approver)
			  if tc.wantErr != nil {
				   if !errors.Is(err, tc.wantErr) {
					  t.Errorf("got %v want %v", err, tc.wantErr)
				  }
				   return
			  }
			   if err != nil {
				  t.Fatalf("unexpected error: %v", err)
			   }
			   p, _ := qs.GetPlan(context.Background(), "plan-fa")
			   if p.State != tc.wantState {
				  t.Errorf("state=%s want %s", p.State, tc.wantState)
			  }
		   })
	  }
  }

// ---- SecondApprove -----------------------------------------------

var secondApproveTable = []struct {
	name      string
	approver2 string
	openDiffs int
	wantErr   error
}{
	{"ok zero diffs", "carol", 0, nil},
	{"nonzero diffs", "carol", 2, custodyhalt.ErrNoOpenDiffs},
	{"same as approver1", "bob", 0, custodyhalt.ErrSameApprover},
	{"same as proposer", "alice", 0, custodyhalt.ErrSameApprover},
	{"empty approver", "", 0, custodyhalt.ErrApproverRequired},
}

   func TestSecondApprove(t *testing.T) {
	   for _, tc := range secondApproveTable {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  qs := newQuorumStore()
			  if err := makeProposal(qs, "plan-sa", "alice"); err != nil {
				  t.Fatal(err)
			  }
			  if err := qs.FirstApprove(context.Background(), "plan-sa", "bob"); err != nil {
				  t.Fatal(err)
			  }
			   proof, err := qs.SecondApprove(context.Background(), "plan-sa", tc.approver2, tc.openDiffs, "")
			  if tc.wantErr != nil {
				  if !errors.Is(err, tc.wantErr) {
					   t.Errorf("got %v want %v", err, tc.wantErr)
				  }
				   return
			  }
			   if err != nil {
				   t.Fatalf("unexpected error: %v", err)
			  }
			  if proof == nil {
				  t.Fatal("expected non-nil proof")
			  }
			   if proof.ProofType != custodyhalt.ProofTypeNormal {
				  t.Errorf("proof_type=%s want normal", proof.ProofType)
			   }
			   if err := custodyhalt.VerifyProof(*proof); err != nil {
				  t.Errorf("proof verification failed: %v", err)
			  }
			   // 计划应处于 cleared 状态。
			   p, _ := qs.GetPlan(context.Background(), "plan-sa")
			  if p.State != custodyhalt.PlanStateCleared {
				   t.Errorf("plan state=%s want cleared", p.State)
			  }
		  })
	   }
  }

// ---- RejectPlan -----------------------------------------------

  func TestRejectPlan(t *testing.T) {
	  qs := newQuorumStore()
	  if err := makeProposal(qs, "plan-rej", "alice"); err != nil {
		  t.Fatal(err)
	   }
	   if err := qs.RejectPlan(context.Background(), "plan-rej", "bob", "recon ticket open"); err != nil {
		  t.Fatalf("reject: %v", err)
	   }
	   p, _ := qs.GetPlan(context.Background(), "plan-rej")
	   if p.State != custodyhalt.PlanStateRejected {
		   t.Errorf("state=%s want rejected", p.State)
	  }
	   if p.RejectedBy != "bob" {
		  t.Errorf("rejected_by=%s want bob", p.RejectedBy)
	   }
  }

  func TestRejectPlanAlreadyFinal(t *testing.T) {
	  qs := newQuorumStore()
	   if err := makeProposal(qs, "plan-fin", "alice"); err != nil {
		   t.Fatal(err)
	   }
	   if err := qs.FirstApprove(context.Background(), "plan-fin", "bob"); err != nil {
		  t.Fatal(err)
	  }
	   if _, err := qs.SecondApprove(context.Background(), "plan-fin", "carol", 0, ""); err != nil {
		   t.Fatal(err)
	   }
	   err := qs.RejectPlan(context.Background(), "plan-fin", "dave", "too late")
	  if !errors.Is(err, custodyhalt.ErrPlanAlreadyFinal) {
		   t.Errorf("got %v want ErrPlanAlreadyFinal", err)
	  }
  }

// ---- QuorumGuard 与 Gate 集成 -----------------------------------
// TestQuorumGuardBlocksClearWithoutPlan 未提供 plan_id 时 Clear 被挡。
  func TestQuorumGuardBlocksClearWithoutPlan(t *testing.T) {
	  gate, _ := newGateWithQuorum(t)
	  ctx := context.Background()
	  // 先打开熔断。
	  if err := gate.Engage(ctx, "system", "test engage"); err != nil {
		  t.Fatal(err)
	   }
	  // 不注入 plan_id，Clear 应被 QuorumGuard 拦截。
	   err := gate.Clear(ctx, "alice", "try clear")
	  if !errors.Is(err, custodyhalt.ErrQuorumRequired) {
		  t.Errorf("got %v want ErrQuorumRequired", err)
	   }
  }

// TestQuorumGuardPassesWithClearedPlan 提供已 cleared 计划时 Clear 成功。

  func TestQuorumGuardPassesWithClearedPlan(t *testing.T) {
	   gate, qs := newGateWithQuorum(t)
	   ctx := context.Background()
	   // 先打开熔断。
	   if err := gate.Engage(ctx, "system", "test engage"); err != nil {
		  t.Fatal(err)
	   }
	   // 走四眼流程。
	  planID := "plan-pass"
	  if err := makeProposal(qs, planID, "alice"); err != nil {
		  t.Fatal(err)
	   }
	  if err := qs.FirstApprove(ctx, planID, "bob"); err != nil {
		   t.Fatal(err)
	   }
	   if _, err := qs.SecondApprove(ctx, planID, "carol", 0, ""); err != nil {
		  t.Fatal(err)
	  }
	   // 注入 plan_id 后 Clear 应成功。
	  ctxWithPlan := custodyhalt.WithClearPlanID(ctx,
		   planID)
	  if err := gate.Clear(ctxWithPlan, "alice", "clear after quorum"); err != nil {
		  t.Fatalf("unexpected error: %v", err)
	  }
	  halted, _ := gate.Halted(ctx)
	  if halted {
		  t.Error("gate should be cleared but still halted")
	   }
  }

// TestQuorumGuardBlocksWithRejectedPlan 否决的计划不能解锁 Clear。

   func TestQuorumGuardBlocksWithRejectedPlan(t *testing.T) {
	  gate, qs := newGateWithQuorum(t)
	  ctx := context.Background()
	  if err := gate.Engage(ctx, "system", "test engage"); err != nil {
		  t.Fatal(err)
	   }
	   planID := "plan-rej"
	  if err := makeProposal(qs, planID, "alice"); err != nil {
		  t.Fatal(err)
	  }
	   if err := qs.RejectPlan(ctx, planID, "bob", "not safe"); err != nil {
		  t.Fatal(err)
	  }
	  ctxWithPlan := custodyhalt.WithClearPlanID(ctx,
		  planID)
	   err := gate.Clear(ctxWithPlan, "alice", "should fail")
	   if !errors.Is(err, custodyhalt.ErrQuorumRequired) {
		  t.Errorf("got %v want ErrQuorumRequired", err)
	   }
  }

// TestClearForcedBypassesQuorum ClearForced 直接跳过四眼检查。
   func TestClearForcedBypassesQuorum(t *testing.T) {
	   gate, _ := newGateWithQuorum(t)
	  ctx := context.Background()
	  if err := gate.Engage(ctx, "system", "test engage"); err != nil {
		  t.Fatal(err)
	  }
	   // ClearForced 无需 plan_id。
	   if err := gate.ClearForced(ctx, "alice", "emergency"); err != nil {
		   t.Fatalf("ClearForced failed: %v", err)
	   }
	   halted, _ := gate.Halted(ctx)
	   if halted {
		   t.Error("gate should be cleared after ClearForced")
	  }
  }

// ---- ForcedAudit -----------------------------------------------

var forcedReasonCodeTable = []struct {
	name       string
	reasonCode string
	postTicketID string
	wantErr      error
}{
	{"emergency_hot_recovery ok", "emergency_hot_recovery", "", nil},
	{"incident_cleared_by_war ok", "incident_cleared_by_war", "", nil},
	{"other with ticket ok", "other_see_ticket", "MC-9012", nil},
	{"other without ticket", "other_see_ticket", "", custodyhalt.ErrForcedTicketOther},
	{"empty reason code", "", "", custodyhalt.ErrForcedReasonCode},
	{"unknown reason code", "unknown_code", "", custodyhalt.ErrForcedReasonCode},
}

  func TestValidateForced(t *testing.T) {
	   for _, tc := range forcedReasonCodeTable {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  err := custodyhalt.ValidateForced(tc.reasonCode, tc.postTicketID)
			  if tc.wantErr != nil {
				   if !errors.Is(err, tc.wantErr) {
					  t.Errorf("got %v want %v", err, tc.wantErr)
				   }
				   return
			  }
			   if err != nil {
				   t.Errorf("unexpected error: %v", err)
			  }
		  })
	  }
  }

  func TestForcedAuditStore_RecordAndLoad(t *testing.T) {
	  kv := persist.NewMemKV()
	  fas := custodyhalt.NewForcedAuditStore(kv)
	   ctx := context.Background()
	  rec := custodyhalt.ForcedAuditRecord{
		   AuditID: "fa:test:001:alice",
		   Scope:   "global",
		  Actor:  "alice",
		   Reason: "emergency",
		  ReasonCode:    "emergency_hot_recovery",
		  OpenDiffCount: 3,
	   }
	   if err := fas.RecordForced(ctx, rec); err != nil {
		  t.Fatalf("record: %v", err)
	  }
	   recs := fas.LoadForced(ctx, 10)
	  if len(recs) == 0 {
		  t.Fatal("expected records, got empty")
	   }
	   if recs[0].Actor != "alice" {
		  t.Errorf("actor=%s want alice", recs[0].Actor)
	   }
	  if !recs[0].MisClearRisk {
		   t.Error("expected misclear_risk=true for open_diff_count>0")
	  }
  }

   func TestForcedAuditStore_QueryFilter(t *testing.T) {
	  kv := persist.NewMemKV()
	   fas := custodyhalt.NewForcedAuditStore(kv)
	   ctx := context.Background()
	  records := []custodyhalt.ForcedAuditRecord{
		  {AuditID: "fa:1:alice", Actor: "alice", ReasonCode: "emergency_hot_recovery", Scope: "global"},
		   {AuditID: "fa:2:bob", Actor: "bob", ReasonCode: "drill_test", Scope: "global"},
		   {AuditID: "fa:3:alice", Actor: "alice", ReasonCode: "drill_test", Scope: "chain"},
	   }
	  for _, r := range records {
		  if err := fas.RecordForced(ctx, r); err != nil {
			  t.Fatal(err)
		   }
	  }
	   cases := []struct {
		  actor string
		   scope string
		  reasonCode string
		   wantCount int
	   }{
		   {"alice", "", "", 2},
		  {"bob", "", "", 1},
		   {"", "global", "", 2},
		  {"", "", "drill_test", 2},
		  {"alice", "", "drill_test", 1},
		  {"carol", "", "", 0},
	   }
	   for _, tc := range cases {
		  tc := tc
		   t.Run(fmt.Sprintf("actor=%s scope=%s code=%s", tc.actor, tc.scope, tc.reasonCode), func(t *testing.T) {
			   result := fas.QueryForced(ctx, tc.actor, tc.scope, tc.reasonCode, 50)
			  if len(result) != tc.wantCount {
				  t.Errorf("got %d want %d", len(result), tc.wantCount)
			  }
		  })
	  }
  }

// ---- VerifyProof -----------------------------------------------

  func TestVerifyProof_TamperDetect(t *testing.T) {
	  qs := newQuorumStore()
	  ctx := context.Background()
	  if err := makeProposal(qs, "plan-vp", "alice"); err != nil {
		  t.Fatal(err)
	   }
	  if err := qs.FirstApprove(ctx, "plan-vp", "bob"); err != nil {
		  t.Fatal(err)
	   }
	   proof, err := qs.SecondApprove(ctx, "plan-vp", "carol", 0, "")
	  if err != nil {
		   t.Fatal(err)
	   }
	   // 正常校验应通过。
	  if err := custodyhalt.VerifyProof(*proof); err != nil {
		  t.Errorf("valid proof: %v", err)
	  }
	   // 篡改 approver2 后校验应失败。
	  tampered := *proof
	   tampered.Approver2 = "mallory"
	   if err := custodyhalt.VerifyProof(tampered); err == nil {
		  t.Error("expected error for tampered proof")
	  }
	  // 篡改 open_diff_count 后校验应失败。
	   tampered2 := *proof
	  tampered2.OpenDiffCount = 1
	   if err := custodyhalt.VerifyProof(tampered2); err == nil {
		  t.Error("expected error for tampered open_diff_count")
	  }
  }

// ---- ListPlans -----------------------------------------------

  func TestListPlans(t *testing.T) {
	  qs := newQuorumStore()
	  ctx := context.Background()
	  for i := 0; i < 5; i++ {
		   if err := makeProposal(qs, fmt.Sprintf("plan-%d", i), "alice"); err != nil {
			   t.Fatal(err)
		   }
	  }
	  plans, err := qs.ListPlans(ctx, 10)
	   if err != nil {
		   t.Fatalf("list: %v", err)
	  }
	   if len(plans) != 5 {
		   t.Errorf("got %d want 5", len(plans))
	  }
  }

// TestNoFreezePlanWritten 验证四眼流程不写战争室 freeze_plans。
// 通过检查 KV 中无 "freeze_plans" kind 键来确认。

   func TestNoFreezePlanWritten(t *testing.T) {
	  kv := persist.NewMemKV()
	  qs := custodyhalt.NewQuorumStore(kv)
	   ctx := context.Background()
	   if err := makeProposal(qs, "plan-nf", "alice"); err != nil {
		   t.Fatal(err)
	   }
	   if err := qs.FirstApprove(ctx, "plan-nf", "bob"); err != nil {
		   t.Fatal(err)
	  }
	  if _, err := qs.SecondApprove(ctx, "plan-nf", "carol", 0, ""); err != nil {
		  t.Fatal(err)
	  }
	   // 检查 KV 中不存在 freeze_plans kind。
	  rows, err := kv.List(ctx, "freeze_plans", 100)
	  if err != nil {
		  t.Fatal(err)
	   }
	  if len(rows) != 0 {
		  t.Errorf("expected 0 freeze_plans rows, got %d", len(rows))
	   }
  }

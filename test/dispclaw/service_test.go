package dispclaw_test

import (
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/dispclaw"
)

// ─────────────────────────── GeneratePlan ───────────────────────────

   func TestGeneratePlan(t *testing.T) {
	   t.Parallel()
	  cases := []struct {
		   name   string
		  caseID string
		  planType dispclaw.PlanType
		  caseUserID  string
		   caseChannel string
		   caseAsset   string
		   caseAmount string
		  openedBy   string
		  actor string
		  idem        string
		  railBreakID string
		   wantErr      string
		   wantLegCount int
		   wantLegTypes []dispclaw.LegType
		  wantPlanStatus dispclaw.PlanStatus
	   }{
		   {
			   name:   "claw_back normal",
			  caseID: "case-001",
			   planType: dispclaw.PlanTypeClawBack,
			  caseUserID:  "user-10001",
			   caseChannel: "fiat",
			  caseAsset:   "USDT",
			   caseAmount:  "500.00",
			   openedBy: "ops-alice",
			   actor: "ops-alice",
			   idem:  "idem-001",
			  wantErr: "",
			   wantLegCount:   2,
			   wantLegTypes:   []dispclaw.LegType{dispclaw.LegTypeFreeze, dispclaw.LegTypeClawDebit},
			   wantPlanStatus: dispclaw.PlanStatusOpen,
		  },
		  {
			   name: "refund normal",
			  caseID:      "case-002",
			   planType:    dispclaw.PlanTypeRefund,
			  caseUserID:  "user-10002",
			   caseChannel: "p2p",
			   caseAsset: "BTC",
			   caseAmount: "0.05",
			   openedBy:   "ops-bob",
			  actor:      "ops-bob",
			  idem:       "idem-002",
			   wantErr:    "",
			   wantLegCount: 2,
			   wantLegTypes: []dispclaw.LegType{dispclaw.LegTypeFreeze, dispclaw.LegTypeRefundCredit},
			  wantPlanStatus: dispclaw.PlanStatusOpen,
		   },
		  {
			  name:     "restore normal",
			  caseID:   "case-003",
			   planType: dispclaw.PlanTypeRestore,
			   caseUserID:  "user-10003",
			   caseChannel: "fiat",
			   caseAsset: "ETH",
			   caseAmount: "1.0",
			  openedBy: "ops-carol",
			  actor: "ops-carol",
			   idem:    "idem-003",
			  wantErr: "",
			   wantLegCount: 1,
			  wantLegTypes: []dispclaw.LegType{dispclaw.LegTypeRestoreUnfreeze},
			  wantPlanStatus: dispclaw.PlanStatusOpen,
		   },
		  {
			   name:       "with rail_break_id readonly attach",
			   caseID:     "case-004",
			  planType:   dispclaw.PlanTypeRefund,
			  caseUserID: "user-10004",
			  caseChannel: "fiat",
			  caseAsset: "USDT",
			   caseAmount: "100.00",
			   openedBy:   "ops-alice",
			  actor:      "ops-alice",
			   idem: "idem-004",
			  railBreakID: "rb-fiat-20220315-001",
			  wantErr: "",
			  wantLegCount:   2,
			  wantPlanStatus: dispclaw.PlanStatusOpen,
		   },
		   {
			  name: "empty case_id rejected",
			  caseID:   "",
			  planType: dispclaw.PlanTypeRefund,
			   caseUserID: "user-10001",
			  caseChannel: "fiat",
			  caseAsset:   "USDT",
			  caseAmount: "100.00",
			  openedBy: "ops-alice",
			  actor:   "ops-alice",
			   idem:    "idem-005",
			  wantErr: "dispclaw: case not found or invalid",
		   },
		   {
			  name: "invalid plan type",
			   caseID: "case-006",
			   planType:    dispclaw.PlanType("invalid_type"),
			  caseUserID:  "user-10001",
			  caseChannel: "fiat",
			  caseAsset:   "USDT",
			   caseAmount:  "100.00",
			   openedBy: "ops-alice",
			  actor:    "ops-alice",
			   idem:     "idem-006",
			   wantErr:  "dispclaw: illegal state transition",
		  },
		  {
			  name: "zero amount rejected",
			  caseID: "case-007",
			   planType:   dispclaw.PlanTypeRefund,
			  caseUserID: "user-10001",
			  caseChannel: "fiat",
			  caseAsset: "USDT",
			   caseAmount: "0",
			   openedBy: "ops-alice",
			  actor:    "ops-alice",
			   idem:     "idem-007",
			  wantErr:  "dispclaw: amount invalid",
		   },
		  {
			   name: "empty user_id rejected",
			  caseID: "case-008",
			   planType: dispclaw.PlanTypeRefund,
			   caseUserID: "",
			  caseChannel: "fiat",
			   caseAsset: "USDT",
			  caseAmount: "100.00",
			  openedBy:   "ops-alice",
			   actor:   "ops-alice",
			   idem:    "idem-008",
			   wantErr: "dispclaw: case not found or invalid",
		  },
		  {
			   name: "empty idempotency_key rejected",
			  caseID:     "case-009",
			   planType:   dispclaw.PlanTypeRefund,
			   caseUserID: "user-10001",
			   caseChannel: "fiat",
			  caseAsset:  "USDT",
			   caseAmount: "100.00",
			  openedBy:   "ops-alice",
			  actor:      "ops-alice",
			   idem:       "",
			  wantErr:    "dispclaw: idempotency conflict",
		  },
	   }
	  for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   svc := dispclaw.New()
			  req := dispclaw.GeneratePlanReq{
				   CaseID:   tc.caseID,
				  PlanType: tc.planType,
				   Actor: tc.actor,
				   RailBreakID: tc.railBreakID,
				  IdempotencyKey: tc.idem,
			  }
			  plan, legs, err := svc.GeneratePlan(req, tc.caseUserID, tc.caseChannel, tc.caseAsset, tc.caseAmount, tc.openedBy)
			  if tc.wantErr != "" {
				  if err == nil {
					   t.Fatalf("expected error %q, got nil", tc.wantErr)
				  }
				  if !strings.Contains(err.Error(), tc.wantErr) {
					   t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
				  }
				  return
			  }
			   if err != nil {
				  t.Fatalf("unexpected error: %v", err)
			   }
			   if plan.CaseID != tc.caseID {
				   t.Errorf("plan.CaseID=%q want %q", plan.CaseID, tc.caseID)
			   }
			  if plan.Status != tc.wantPlanStatus {
				  t.Errorf("plan.Status=%q want %q", plan.Status, tc.wantPlanStatus)
			  }
			   if plan.PlanType != tc.planType {
				  t.Errorf("plan.PlanType=%q want %q", plan.PlanType, tc.planType)
			  }
			   if tc.railBreakID != "" && plan.RailBreakID != tc.railBreakID {
				   t.Errorf("plan.RailBreakID=%q want %q", plan.RailBreakID, tc.railBreakID)
			  }
			  if len(legs) != tc.wantLegCount {
				  t.Fatalf("legs count=%d want %d", len(legs), tc.wantLegCount)
			  }
			  for i, lt := range tc.wantLegTypes {
				  if legs[i].LegType != lt {
					  t.Errorf("legs[%d].LegType=%q want %q", i, legs[i].LegType, lt)
				   }
				  if legs[i].Status != dispclaw.LegStatusPending {
					   t.Errorf("legs[%d].Status=%q want pending", i, legs[i].Status)
				   }
				   if legs[i].LegHash == "" {
					  t.Errorf("legs[%d].LegHash empty", i)
				  }
			  }
		  })
	  }
  }

  func TestGeneratePlan_Idempotent(t *testing.T) {
	   t.Parallel()
	   svc := dispclaw.New()
	  req := dispclaw.GeneratePlanReq{
		   CaseID:   "case-idem",
		   PlanType: dispclaw.PlanTypeRefund,
		   Actor: "ops-alice",
		  IdempotencyKey: "idem-idempotent",
	   }
	  p1, l1, err1 := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "200.00", "ops-alice")
	  if err1 != nil {
		   t.Fatal(err1)
	  }
	  p2, l2, err2 := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "200.00", "ops-alice")
	  if err2 != nil {
		   t.Fatal(err2)
	   }
	  if p1.ID != p2.ID {
		  t.Errorf("idempotent plans differ: %s vs %s", p1.ID, p2.ID)
	   }
	   if len(l1) != len(l2) {
		   t.Errorf("leg counts differ: %d vs %d", len(l1), len(l2))
	   }
   }

  func TestGeneratePlan_OneCaseOnePlan(t *testing.T) {
	  t.Parallel()
	   svc := dispclaw.New()
	  req1 := dispclaw.GeneratePlanReq{
		  CaseID:   "case-dup",
		  PlanType: dispclaw.PlanTypeRefund,
		   Actor: "ops-alice",
		  IdempotencyKey: "idem-dup-1",
	   }
	  req2 := dispclaw.GeneratePlanReq{
		  CaseID: "case-dup", // 相同 case_id
		   PlanType: dispclaw.PlanTypeClawBack,
		   Actor:          "ops-alice",
		  IdempotencyKey: "idem-dup-2", // 不同 idem
	   }
	  if _, _, err := svc.GeneratePlan(req1, "user-10001", "fiat", "USDT", "100.00", "ops-alice"); err != nil {
		   t.Fatal(err)
	  }
	   _, _, err := svc.GeneratePlan(req2, "user-10001", "fiat", "USDT", "100.00", "ops-alice")
	  if err == nil {
		   t.Fatal("expected ErrPlanExists, got nil")
	   }
	  if !strings.Contains(err.Error(), "already exists") {
		   t.Errorf("unexpected error: %v", err)
	  }
  }

// ─────────────────────────── ExecuteLeg（四眼） ───────────────────────────

  func TestExecuteLeg_FourEye(t *testing.T) {
	   t.Parallel()
	   cases := []struct {
		   name          string
		  openerActor   string
		   executorActor string
		  wantErr    string
		  wantStatus dispclaw.LegStatus
	   }{
		  {
			   name: "opener rejected (four-eye)",
			  openerActor: "ops-alice",
			   executorActor: "ops-alice",
			  wantErr: "dispclaw: four-eye check failed",
		   },
		   {
			   name:        "different actor passes",
			   openerActor: "ops-alice",
			  executorActor: "ops-bob",
			   wantErr: "",
			   wantStatus: dispclaw.LegStatusPosted,
		   },
		  {
			   name:          "case-insensitive opener check",
			   openerActor:   "ops-alice",
			   executorActor: "OPS-ALICE",
			  wantErr:       "dispclaw: four-eye check failed",
		  },
	   }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			  svc := dispclaw.New()
			   caseID := "case-fe-" + tc.name
			   req := dispclaw.GeneratePlanReq{
				  CaseID: caseID,
				  PlanType: dispclaw.PlanTypeRefund,
				  Actor:    tc.openerActor,
				  IdempotencyKey: "idem-fe-" + tc.name,
			  }
			  _, legs, err := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "100.00", tc.openerActor)
			   if err != nil {
				   t.Fatal(err)
			  }
			  execReq := dispclaw.ExecuteLegReq{
				  PlanID: legs[0].PlanID,
				   LegID: legs[0].ID,
				  Actor: tc.executorActor,
			   }
			   result, err := svc.ExecuteLeg(execReq)
			  if tc.wantErr != "" {
				  if err == nil {
					   t.Fatalf("expected error %q, got nil", tc.wantErr)
				  }
				   if !strings.Contains(err.Error(), tc.wantErr) {
					   t.Fatalf("got %q want contains %q", err.Error(), tc.wantErr)
				   }
				   return
			  }
			  if err != nil {
				  t.Fatalf("unexpected error: %v", err)
			   }
			  if result.Status != tc.wantStatus {
				  t.Errorf("status=%q want %q", result.Status, tc.wantStatus)
			   }
			   if result.PostedBy != tc.executorActor {
				   t.Errorf("posted_by=%q want %q", result.PostedBy, tc.executorActor)
			   }
			   if result.LegHash == "" {
				   t.Error("leg_hash empty after execute")
			   }
		  })
	   }
   }

  func TestExecuteLeg_NotPending(t *testing.T) {
	  t.Parallel()
	   svc := dispclaw.New()
	   req := dispclaw.GeneratePlanReq{
		  CaseID:   "case-notpending",
		   PlanType: dispclaw.PlanTypeRefund,
		  Actor:    "ops-alice",
		  IdempotencyKey: "idem-notpending",
	   }
	  _, legs, err := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "100.00", "ops-alice")
	   if err != nil {
		   t.Fatal(err)
	  }
	   // 先成功执行一次。
	   execReq := dispclaw.ExecuteLegReq{
		  PlanID: legs[0].PlanID,
		  LegID:  legs[0].ID,
		  Actor: "ops-bob",
	  }
	   if _, err := svc.ExecuteLeg(execReq); err != nil {
		  t.Fatal(err)
	  }
	  // 再次执行已 posted 腿应报错。
	  _, err = svc.ExecuteLeg(execReq)
	   if err == nil {
		   t.Fatal("expected ErrLegNotPending, got nil")
	  }
   }

  func TestExecuteLeg_PlanNotFound(t *testing.T) {
	  t.Parallel()
	  svc := dispclaw.New()
	   _, err := svc.ExecuteLeg(dispclaw.ExecuteLegReq{
		   PlanID: "nonexistent-plan",
		   LegID:  "nonexistent-leg",
		  Actor: "ops-bob",
	  })
	   if err == nil {
		  t.Fatal("expected error, got nil")
	   }
  }

// ─────────────────────────── CloseGate ───────────────────────────

   func TestCheckCloseGate(t *testing.T) {
	   t.Parallel()
	  cases := []struct {
		   name string
		   planType       dispclaw.PlanType
		  executeLegs    bool // 是否先执行所有腿
		   wantGateStatus dispclaw.GateStatus
	  }{
		   {
			  name:        "all legs pending → blocking",
			  planType:    dispclaw.PlanTypeRefund,
			  executeLegs: false,
			  wantGateStatus: dispclaw.GateStatusBlocking,
		  },
		  {
			  name: "all legs posted → passed",
			  planType: dispclaw.PlanTypeRefund,
			   executeLegs: true,
			   wantGateStatus: dispclaw.GateStatusPassed,
		   },
		  {
			  name: "restore single leg → blocking when pending",
			   planType: dispclaw.PlanTypeRestore,
			   executeLegs: false,
			   wantGateStatus: dispclaw.GateStatusBlocking,
		  },
		  {
			  name: "restore single leg → passed when posted",
			  planType:    dispclaw.PlanTypeRestore,
			   executeLegs: true,
			  wantGateStatus: dispclaw.GateStatusPassed,
		   },
	  }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			  svc := dispclaw.New()
			   caseID := "case-gate-" + tc.name
			   req := dispclaw.GeneratePlanReq{
				   CaseID: caseID,
				   PlanType:       tc.planType,
				   Actor:          "ops-alice",
				  IdempotencyKey: "idem-gate-" + tc.name,
			   }
			  plan, legs, err := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "100.00", "ops-alice")
			   if err != nil {
				   t.Fatal(err)
			   }
			  if tc.executeLegs {
				   for _, leg := range legs {
					  if _, err := svc.ExecuteLeg(dispclaw.ExecuteLegReq{
						   PlanID: plan.ID,
						  LegID:  leg.ID,
						   Actor:  "ops-bob",
					  }); err != nil {
						   t.Fatal(err)
					  }
				   }
			  }
			   gate, err := svc.CheckCloseGate(dispclaw.CloseGateCheckReq{
				   CaseID: caseID,
				  Actor:  "system-gate-patrol",
			  })
			  if err != nil {
				  t.Fatal(err)
			   }
			  if gate.GateStatus != tc.wantGateStatus {
				   t.Errorf("gate_status=%q want %q", gate.GateStatus, tc.wantGateStatus)
			  }
			  if tc.wantGateStatus == dispclaw.GateStatusPassed && gate.PassedAt == nil {
				   t.Error("passed_at should be set when gate is passed")
			   }
		   })
	   }
  }

   func TestBypassCloseGate(t *testing.T) {
	   t.Parallel()
	   svc := dispclaw.New()
	  req := dispclaw.GeneratePlanReq{
		   CaseID:   "case-bypass",
		  PlanType: dispclaw.PlanTypeRefund,
		   Actor:    "ops-alice",
		  IdempotencyKey: "idem-bypass",
	   }
	  if _, _, err := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "100.00", "ops-alice"); err != nil {
		   t.Fatal(err)
	  }
	   // 豁免操作。
	  gate, err := svc.BypassCloseGate(dispclaw.BypassGateReq{
		  CaseID:       "case-bypass",
		   Actor:        "ops-admin",
		   BypassReason: "emergency admin bypass: user refund urgent",
	  })
	  if err != nil {
		   t.Fatal(err)
	   }
	  if gate.GateStatus != dispclaw.GateStatusBypassed {
		  t.Errorf("gate_status=%q want bypassed", gate.GateStatus)
	  }
	  if gate.BypassedBy != "ops-admin" {
		   t.Errorf("bypassed_by=%q want ops-admin", gate.BypassedBy)
	   }
   }

  func TestBypassCloseGate_EmptyReason(t *testing.T) {
	   t.Parallel()
	   svc := dispclaw.New()
	  req := dispclaw.GeneratePlanReq{
		  CaseID:   "case-bypass-empty",
		   PlanType: dispclaw.PlanTypeRefund,
		   Actor:    "ops-alice",
		  IdempotencyKey: "idem-bypass-empty",
	  }
	   if _, _, err := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "100.00", "ops-alice"); err != nil {
		  t.Fatal(err)
	  }
	  _, err := svc.BypassCloseGate(dispclaw.BypassGateReq{
		  CaseID:       "case-bypass-empty",
		   Actor:        "ops-admin",
		   BypassReason: "", // 空理由应报错
	   })
	  if err == nil {
		  t.Fatal("expected ErrBypassReason, got nil")
	  }
   }

// ─────────────────────────── GateBlocking 阻断结案 ─────────────────────────────
// TestGateBlockingPreventsResolve 验证收口闸 blocking 时无法生成证明。

   func TestGateBlockingPreventsResolve(t *testing.T) {
	   t.Parallel()
	   svc := dispclaw.New()
	  req := dispclaw.GeneratePlanReq{
		  CaseID:   "case-gate-block-resolve",
		  PlanType: dispclaw.PlanTypeRefund,
		  Actor:    "ops-alice",
		   IdempotencyKey: "idem-gate-block-resolve",
	  }
	  if _, _, err := svc.GeneratePlan(req, "user-50001", "fiat", "USDT", "100.00", "ops-alice"); err != nil {
		  t.Fatal(err)
	   }
	  // 收口闸 blocking，生成证明应失败（不等腿完成就结案意图）。
	   _, err := svc.GenerateCloseProof("case-gate-block-resolve", "ops-carol", "resolve")
	  if err == nil {
		  t.Fatal("expected gate blocking error")
	  }
	   if !strings.Contains(err.Error(), "gate blocking") {
		  t.Errorf("unexpected error: %v", err)
	   }
  }

// ─────────────────────────── CloseProof ─────────────────────────────

   func TestGenerateCloseProof(t *testing.T) {
	  t.Parallel()
	   svc := dispclaw.New()
	   req := dispclaw.GeneratePlanReq{
		  CaseID: "case-proof-001",
		  PlanType: dispclaw.PlanTypeRefund,
		  Actor: "ops-alice",
		  IdempotencyKey: "idem-proof-001",
	   }
	  plan, legs, err := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "200.00", "ops-alice")
	  if err != nil {
		   t.Fatal(err)
	   }
	  // 执行所有腿。
	   for _, leg := range legs {
		  if _, err := svc.ExecuteLeg(dispclaw.ExecuteLegReq{
			  PlanID: plan.ID,
			   LegID:  leg.ID,
			   Actor:  "ops-bob",
		   }); err != nil {
			  t.Fatal(err)
		   }
	  }
	   // 检查收口闸。
	   gate, err := svc.CheckCloseGate(dispclaw.CloseGateCheckReq{CaseID: "case-proof-001", Actor: "ops-bob"})
	  if err != nil {
		   t.Fatal(err)
	   }
	  if gate.GateStatus != dispclaw.GateStatusPassed {
		   t.Fatalf("gate not passed: %q", gate.GateStatus)
	   }
	   // 生成收口证明。
	   proof, err := svc.GenerateCloseProof("case-proof-001", "ops-carol", "resolve")
	   if err != nil {
		   t.Fatal(err)
	   }
	  if proof.LegCount != len(legs) {
		   t.Errorf("proof.LegCount=%d want %d", proof.LegCount, len(legs))
	   }
	   if proof.ProofHash == "" {
		  t.Error("proof_hash empty")
	   }
	  if proof.LegHashes == "" {
		  t.Error("leg_hashes empty")
	   }
	   hashes, err := dispclaw.UnmarshalLegHashes(proof.LegHashes)
	   if err != nil {
		  t.Fatal(err)
	   }
	  if len(hashes) != len(legs) {
		  t.Errorf("leg_hashes count=%d want %d", len(hashes), len(legs))
	   }
	  // 验证每个腿哈希非空。
	  for i, h := range hashes {
		  if h == "" {
			   t.Errorf("hashes[%d] empty", i)
		   }
	   }
  }

   func TestGenerateCloseProof_GateBlocking(t *testing.T) {
	  t.Parallel()
	   svc := dispclaw.New()
	   req := dispclaw.GeneratePlanReq{
		  CaseID:   "case-proof-block",
		   PlanType: dispclaw.PlanTypeRefund,
		  Actor:    "ops-alice",
		  IdempotencyKey: "idem-proof-block",
	  }
	   if _, _, err := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "100.00", "ops-alice"); err != nil {
		   t.Fatal(err)
	  }
	  // 不执行任何腿，收口闸 blocking，生成证明应失败。
	   _, err := svc.GenerateCloseProof("case-proof-block", "ops-carol", "resolve")
	   if err == nil {
		   t.Fatal("expected ErrGateBlocking, got nil")
	   }
	   if !strings.Contains(err.Error(), "gate blocking") {
		   t.Errorf("unexpected error: %v", err)
	   }
   }

  func TestGenerateCloseProof_Idempotent(t *testing.T) {
	   t.Parallel()
	  svc := dispclaw.New()
	   req := dispclaw.GeneratePlanReq{
		  CaseID:   "case-proof-idem",
		   PlanType: dispclaw.PlanTypeRestore,
		  Actor:          "ops-alice",
		   IdempotencyKey: "idem-proof-idem",
	   }
	  plan, legs, err := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "50.00", "ops-alice")
	  if err != nil {
		  t.Fatal(err)
	   }
	   for _, leg := range legs {
		  if _, err := svc.ExecuteLeg(dispclaw.ExecuteLegReq{PlanID: plan.ID, LegID: leg.ID, Actor: "ops-bob"}); err != nil {
			   t.Fatal(err)
		   }
	   }
	  svc.CheckCloseGate(dispclaw.CloseGateCheckReq{CaseID: "case-proof-idem", Actor: "ops-bob"})
	  p1, _ := svc.GenerateCloseProof("case-proof-idem", "ops-carol", "resolve")
	  p2, _ := svc.GenerateCloseProof("case-proof-idem", "ops-carol", "resolve")
	  if p1.ID != p2.ID {
		   t.Error("proof not idempotent")
	   }
   }

// ─────────────────────────── PatrolProof（篡改检测） ───────────────────────────
// TestPatrolProof_TamperDetected 验收夹具 3：篡改腿哈希后巡检必须检测到。
   func TestPatrolProof_TamperDetected(t *testing.T) {
	  t.Parallel()
	   svc := dispclaw.New()
	   req := dispclaw.GeneratePlanReq{
		  CaseID: "case-tamper",
		   PlanType: dispclaw.PlanTypeRefund,
		  Actor:          "ops-alice",
		   IdempotencyKey: "idem-tamper",
	  }
	  plan, legs, err := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "300.00", "ops-alice")
	   if err != nil {
		   t.Fatal(err)
	   }
	  for _, leg := range legs {
		  if _, err := svc.ExecuteLeg(dispclaw.ExecuteLegReq{PlanID: plan.ID, LegID: leg.ID, Actor: "ops-bob"}); err != nil {
			   t.Fatal(err)
		   }
	  }
	  svc.CheckCloseGate(dispclaw.CloseGateCheckReq{CaseID: "case-tamper", Actor: "ops-bob"})
	   proof, err := svc.GenerateCloseProof("case-tamper", "ops-carol", "resolve")
	  if err != nil {
		  t.Fatal(err)
	  }
	   // 正常巡检应通过。
	  _, err = svc.PatrolProof("case-tamper")
	   if err != nil {
		   t.Fatalf("patrol should pass: %v", err)
	  }
	   // 篡改：直接修改内部腿金额后重新计算证明哈希（通过修改证明期望哈希来）。
	  // 此处通过 VerifyProof 直接篡改检测逻辑。
	  tamperedLegs := []dispclaw.RecoveryLeg{
		   {
			   PlanID: plan.ID,
			   Seq:     1,
			   LegType: dispclaw.LegTypeFreeze,
			   UserID: "user-10001",
			   Asset:  "USDT",
			  Amount: "999999.00", // 篡改金额
			  Status: dispclaw.LegStatusPosted,
		  },
	   }
	   err = dispclaw.VerifyProof(proof, tamperedLegs)
	   if err != dispclaw.ErrProofTamper {
		  t.Errorf("expected ErrProofTamper for tampered legs, got %v", err)
	   }
   }

  func TestPatrolProof_NoTamper(t *testing.T) {
	   t.Parallel()
	   svc := dispclaw.New()
	  req := dispclaw.GeneratePlanReq{
		   CaseID:         "case-notamper",
		   PlanType:       dispclaw.PlanTypeRestore,
		   Actor:          "ops-alice",
		  IdempotencyKey: "idem-notamper",
	  }
	  plan, legs, err := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "100.00", "ops-alice")
	   if err != nil {
		  t.Fatal(err)
	   }
	   for _, leg := range legs {
		  svc.ExecuteLeg(dispclaw.ExecuteLegReq{PlanID: plan.ID, LegID: leg.ID, Actor: "ops-bob"})
	  }
	  svc.CheckCloseGate(dispclaw.CloseGateCheckReq{CaseID: "case-notamper", Actor: "ops-bob"})
	   svc.GenerateCloseProof("case-notamper", "ops-carol", "resolve")
	   _, err = svc.PatrolProof("case-notamper")
	  if err != nil {
		   t.Errorf("patrol should pass, got %v", err)
	   }
  }

// ─────────────────────────── AbortPlan ───────────────────────────

   func TestAbortPlan(t *testing.T) {
	   t.Parallel()
	   cases := []struct {
		  name        string
		   abortReason string
		  wantErr     string
	  }{
		   {
			  name: "normal abort",
			   abortReason: "审批超时，管理员中止",
			  wantErr:     "",
		   },
		  {
			  name: "empty reason rejected",
			   abortReason: "",
			   wantErr: "dispclaw: abort reason required",
		  },
	  }
	  for _, tc := range cases {
		   tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			  svc := dispclaw.New()
			   req := dispclaw.GeneratePlanReq{
				   CaseID: "case-abort-" + tc.name,
				  PlanType: dispclaw.PlanTypeRefund,
				   Actor: "ops-alice",
				   IdempotencyKey: "idem-abort-" + tc.name,
			  }
			   plan, _, err := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "100.00", "ops-alice")
			   if err != nil {
				  t.Fatal(err)
			   }
			   aborted, err := svc.AbortPlan(dispclaw.AbortPlanReq{
				  PlanID: plan.ID,
				   Actor:       "ops-admin",
				  AbortReason: tc.abortReason,
			  })
			  if tc.wantErr != "" {
				   if err == nil {
					  t.Fatalf("expected error %q, got nil", tc.wantErr)
				  }
				  if !strings.Contains(err.Error(), tc.wantErr) {
					   t.Fatalf("got %q want contains %q", err.Error(), tc.wantErr)
				  }
				   return
			  }
			   if err != nil {
				  t.Fatalf("unexpected error: %v", err)
			   }
			   if aborted.Status != dispclaw.PlanStatusAborted {
				  t.Errorf("plan.Status=%q want aborted", aborted.Status)
			   }
			   // 腿应全部被跳过。
			   legs := svc.ListLegs(plan.ID)
			  for _, l := range legs {
				  if l.Status != dispclaw.LegStatusSkipped {
					   t.Errorf("leg %s status=%q want skipped", l.ID, l.Status)
				  }
			  }
		   })
	   }
   }

  func TestAbortPlan_AlreadyTerminal(t *testing.T) {
	   t.Parallel()
	   svc := dispclaw.New()
	   req := dispclaw.GeneratePlanReq{
		  CaseID:   "case-abort-terminal",
		  PlanType: dispclaw.PlanTypeRestore,
		  Actor:          "ops-alice",
		   IdempotencyKey: "idem-abort-terminal",
	  }
	  plan, legs, err := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "50.00", "ops-alice")
	  if err != nil {
		   t.Fatal(err)
	   }
	   for _, leg := range legs {
		  svc.ExecuteLeg(dispclaw.ExecuteLegReq{PlanID: plan.ID, LegID: leg.ID, Actor: "ops-bob"})
	   }
	  svc.CheckCloseGate(dispclaw.CloseGateCheckReq{CaseID: "case-abort-terminal", Actor: "ops-bob"})
	  svc.GenerateCloseProof("case-abort-terminal", "ops-carol", "resolve")
	   // 计划已 closed，再中止应报错。
	   _, err = svc.AbortPlan(dispclaw.AbortPlanReq{
		   PlanID: plan.ID,
		  Actor: "ops-admin",
		   AbortReason: "too late",
	   })
	   if err == nil {
		   t.Fatal("expected ErrState, got nil")
	  }
   }

// ─────────────────────────── MarkLegFailed / SkipLeg ─────────────────────────

   func TestMarkLegFailed(t *testing.T) {
	  t.Parallel()
	   svc := dispclaw.New()
	  req := dispclaw.GeneratePlanReq{
		   CaseID: "case-legfail",
		  PlanType: dispclaw.PlanTypeRefund,
		   Actor:          "ops-alice",
		   IdempotencyKey: "idem-legfail",
	   }
	  _, legs, err := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "100.00", "ops-alice")
	  if err != nil {
		   t.Fatal(err)
	  }
	   nextRetry := time.Now().UTC().Add(2 * time.Minute)
	  result, err := svc.MarkLegFailed(legs[0].ID, "account insufficient", &nextRetry)
	   if err != nil {
		  t.Fatal(err)
	   }
	  if result.Status != dispclaw.LegStatusFailed {
		  t.Errorf("status=%q want failed", result.Status)
	  }
	   if result.RetryCount != 1 {
		   t.Errorf("retry_count=%d want 1", result.RetryCount)
	  }
	  if result.LegHash == "" {
		   t.Error("leg_hash empty after fail")
	  }
   }

  func TestSkipLeg(t *testing.T) {
	   t.Parallel()
	   svc := dispclaw.New()
	   req := dispclaw.GeneratePlanReq{
		  CaseID: "case-skip",
		  PlanType: dispclaw.PlanTypeRefund,
		  Actor:          "ops-alice",
		   IdempotencyKey: "idem-skip",
	   }
	   _, legs, err := svc.GeneratePlan(req, "user-10001", "fiat", "USDT", "100.00", "ops-alice")
	   if err != nil {
		   t.Fatal(err)
	   }
	   result, err := svc.SkipLeg(legs[0].ID, "refund decision changed")
	   if err != nil {
		  t.Fatal(err)
	   }
	   if result.Status != dispclaw.LegStatusSkipped {
		  t.Errorf("status=%q want skipped", result.Status)
	   }
   }

// ─────────────────────────── Hash ─────────────────────────────

   func TestComputeLegHash(t *testing.T) {
	   t.Parallel()
	   in := dispclaw.LegHashInput{
		  PlanID: "plan-hash-test",
		  Seq:    1,
		  LegType: dispclaw.LegTypeFreeze,
		   UserID: "user-10001",
		   Asset:  "USDT",
		  Amount: "100.00",
		   Status: dispclaw.LegStatusPosted,
	  }
	  h1 := dispclaw.ComputeLegHash(in)
	   h2 := dispclaw.ComputeLegHash(in)
	  if h1 != h2 {
		   t.Error("hash not deterministic")
	  }
	  // 修改任一字段，哈希应变化。
	  in2 := in
	   in2.Amount = "200.00"
	   h3 := dispclaw.ComputeLegHash(in2)
	   if h1 == h3 {
		  t.Error("hash should differ after amount change")
	   }
	  if len(h1) != 64 {
		   t.Errorf("hash length=%d want 64", len(h1))
	   }
   }

   func TestComputeProofHash(t *testing.T) {
	   t.Parallel()
	   hashes := []string{"hash-a", "hash-b", "hash-c"}
	  ph1 := dispclaw.ComputeProofHash(hashes)
	   ph2 := dispclaw.ComputeProofHash(hashes)
	   if ph1 != ph2 {
		   t.Error("proof hash not deterministic")
	  }
	  // 不同输入应得不同哈希。
	   hashes2 := []string{"hash-a", "hash-b"}
	  ph3 := dispclaw.ComputeProofHash(hashes2)
	   if ph1 == ph3 {
		  t.Error("proof hash collision")
	   }
   }

// ─────────────────────────── Counts ─────────────────────────────

  func TestCounts(t *testing.T) {
	   t.Parallel()
	  svc := dispclaw.New()
	  counts := svc.Counts()
	   if counts == nil {
		  t.Fatal("counts nil")
	  }
	   // 初始应全为 0。
	   for k, v := range counts {
		   if v != 0 {
			  t.Errorf("counts[%q]=%d want 0", k, v)
		  }
	  }
	   // 生成一个计划。
	   svc.GeneratePlan(dispclaw.GeneratePlanReq{
		   CaseID:   "case-count",
		   PlanType: dispclaw.PlanTypeRefund,
		  Actor:          "ops-alice",
		  IdempotencyKey: "idem-count",
	  }, "user-10001", "fiat", "USDT", "100.00", "ops-alice")
	  counts2 := svc.Counts()
	  if counts2["plans_open"] != 1 {
		  t.Errorf("plans_open=%d want 1", counts2["plans_open"])
	  }
	   if counts2["legs_pending"] != 2 { // refund: freeze + refund_credit
		   t.Errorf("legs_pending=%d want 2", counts2["legs_pending"])
	  }
	   if counts2["gates_blocking"] != 1 {
		  t.Errorf("gates_blocking=%d want 1", counts2["gates_blocking"])
	  }
  }

// ─────────────────────────── 并行安全 ─────────────────────────────

   func TestService_Concurrent(t *testing.T) {
	  t.Parallel()
	  svc := dispclaw.New()
	  const N = 50
	   done := make(chan struct{}, N)
	   for i := 0; i < N; i++ {
		  i := i
		  go func() {
			  defer func() { done <- struct{}{} }()
			   caseID := strings.Repeat("a", 8) + strings.Repeat("0", i)
			  svc.GeneratePlan(dispclaw.GeneratePlanReq{
				  CaseID: "case-conc-" + caseID,
				   PlanType: dispclaw.PlanTypeRestore,
				  Actor:    "ops-alice",
				  IdempotencyKey: "idem-conc-" + caseID,
			   }, "user-10001", "fiat", "USDT", "10.00", "ops-alice")
		   }()
	   }
	  for i := 0; i < N; i++ {
		  <-done
	  }
	   counts := svc.Counts()
	  if counts["plans_open"] != N {
		  t.Errorf("plans_open=%d want %d", counts["plans_open"], N)
	  }
  }

// ─────────────────────────── 辅助函数 ─────────────────────────────

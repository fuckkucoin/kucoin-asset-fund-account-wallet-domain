package flow

import (
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/dispclaw"
	"account-assets.kucoin.net/funding/internal/disputedesk"
)

// ─────────────────────────── 辅助函数 ────────────────────────────────────────
// buildDeskWithGate 构造注入了 GateChecker 的 disputedesk.Service。
// GateChecker 从 dispclaw.Service 读取 CloseGate 状态；两个服务内存共享，不跨进程。

  func buildDeskWithGate(t *testing.T) (*disputedesk.Service, *dispclaw.Service) {
	   t.Helper()
	  clawSvc := dispclaw.New()
	  deskSvc := disputedesk.New()
	   deskSvc.SetGateChecker(disputedesk.NewGateCheckerFunc(func(caseID string) (disputedesk.GateStatus, error) {
		   gate, err := clawSvc.GetGateByCaseID(caseID)
		   if err == dispclaw.ErrGate {
			   return disputedesk.GatePassed, nil
		   }
		   if err != nil {
			   return disputedesk.GateBlocking, err
		  }
		   switch gate.GateStatus {
		  case dispclaw.GateStatusPassed, dispclaw.GateStatusBypassed:
			   return disputedesk.GatePassed, nil
		  default:
			   return disputedesk.GateBlocking, nil
		   }
	   }))
	  return deskSvc, clawSvc
   }

// openCaseAndEvidence 在 disputedesk 中开 Case 并提交证据（让 Case 进入可结案状态）。
  func openCaseAndEvidence(t *testing.T, desk *disputedesk.Service, opener, userID string, idem string) string {
	   t.Helper()
	   c, err := desk.Open(disputedesk.OpenReq{
		  Channel: disputedesk.ChanFiat,
		   UserID:  userID,
		   Asset:   "USDT",
		  Amount:   "100.00",
		  Reason:   "dispute chargeback test",
		  OpenedBy: opener,
		  IdempotencyKey: idem,
	  })
	  if err != nil {
		   t.Fatalf("open case: %v", err)
	  }
	  _, err = desk.AttachPayload(c.ID, opener, "receipt", "refund receipt", []byte("receipt-data-"+idem))
	  if err != nil {
		  t.Fatalf("attach evidence: %v", err)
	   }
	  return c.ID
   }

// ─────────────────────────── 验收夹具 1 ──────────────────────────────────────
// TestDispClaw_GateBlockingPreventsResolve 验收夹具 1：
// Case→FundPlan 后，未完成 RecoveryLeg 时 Resolve 必须失败。
  func TestDispClaw_GateBlockingPreventsResolve(t *testing.T) {
	   t.Parallel()
	   cases := []struct {
		  name     string
		  planType dispclaw.PlanType
		  execLegs bool // 是否先完成腿
		   wantFail bool // Resolve 是否应该失败
	   }{
		  {
			   name: "blocking when no legs executed",
			  planType: dispclaw.PlanTypeRefund,
			  execLegs: false,
			   wantFail: true,
		  },
		   {
			   name:     "passes when all legs executed",
			  planType: dispclaw.PlanTypeRefund,
			   execLegs: true,
			   wantFail: false,
		  },
		   {
			  name: "restore blocking when leg pending",
			  planType: dispclaw.PlanTypeRestore,
			   execLegs: false,
			   wantFail: true,
		   },
		   {
			  name:     "restore passes when leg posted",
			   planType: dispclaw.PlanTypeRestore,
			   execLegs: true,
			  wantFail: false,
		  },
		   {
			   name:     "claw_back blocking when legs pending",
			  planType: dispclaw.PlanTypeClawBack,
			   execLegs: false,
			   wantFail: true,
		  },
	   }
	  for _, tc := range cases {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   desk, clawSvc := buildDeskWithGate(t)
			  idem := "idem-fixture1-" + tc.name
			  opener := "ops-alice"
			  userID := "user-50001"
			   resolver := "ops-carol"
			  caseID := openCaseAndEvidence(t, desk, opener, userID, idem)
			  plan, legs, err := clawSvc.GeneratePlan(dispclaw.GeneratePlanReq{
				   CaseID:   caseID,
				  PlanType: tc.planType,
				   Actor:    opener,
				  IdempotencyKey: idem + "-plan",
			  }, userID, "fiat", "USDT", "100.00", opener)
			  if err != nil {
				   t.Fatalf("generate plan: %v", err)
			  }
			  if tc.execLegs {
				  for _, leg := range legs {
					  if _, err := clawSvc.ExecuteLeg(dispclaw.ExecuteLegReq{
						  PlanID: plan.ID,
						  LegID: leg.ID,
						   Actor: "ops-bob", // 换人
					  }); err != nil {
						   t.Fatalf("execute leg: %v", err)
					  }
				  }
				   clawSvc.CheckCloseGate(dispclaw.CloseGateCheckReq{CaseID: caseID, Actor: "ops-bob"})
			  }
			  _, resolveErr := desk.ResolveFourEye(caseID, resolver, "resolve", "")
			   if tc.wantFail {
				  if resolveErr == nil {
					   t.Fatal("expected gate blocking error, got nil")
				  }
				  if !strings.Contains(resolveErr.Error(), "fund gate blocking") && !strings.Contains(resolveErr.Error(), "gate blocking") {
					  t.Errorf("unexpected error: %v", resolveErr)
				  }
			  } else {
				   if resolveErr != nil {
					  t.Fatalf("unexpected resolve error: %v", resolveErr)
				   }
			  }
		   })
	   }
   }

// ─────────────────────────── 验收夹具 2 ──────────────────────────────────────
// TestDispClaw_FourEyeOpenerReject 验收夹具 2：
// 开单人不得独自执行追偿腿（四眼拒绝）；换人后成功。

   func TestDispClaw_FourEyeOpenerReject(t *testing.T) {
	   t.Parallel()
	   cases := []struct {
		   name        string
		  openerActor string
		   executorActor string
		   wantErr       string
	  }{
		  {
			  name:          "opener rejected",
			  openerActor:   "ops-alice",
			  executorActor: "ops-alice",
			   wantErr: "four-eye",
		   },
		  {
			   name: "different actor succeeds",
			   openerActor:   "ops-alice",
			  executorActor: "ops-bob",
			  wantErr:       "",
		  },
		  {
			   name:          "uppercase opener rejected",
			  openerActor:   "ops-alice",
			  executorActor: "OPS-ALICE",
			   wantErr: "four-eye",
		   },
		   {
			   name: "third actor succeeds",
			  openerActor:   "ops-alice",
			  executorActor: "ops-charlie",
			  wantErr: "",
		   },
	  }
	  for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			   _, clawSvc := buildDeskWithGate(t)
			  idem := "idem-fixture2-" + tc.name
			   plan, legs, err := clawSvc.GeneratePlan(dispclaw.GeneratePlanReq{
				  CaseID: "case-fe-" + tc.name,
				   PlanType:       dispclaw.PlanTypeRefund,
				  Actor:          tc.openerActor,
				  IdempotencyKey: idem,
			  }, "user-50002", "fiat", "USDT", "200.00", tc.openerActor)
			  if err != nil {
				   t.Fatal(err)
			   }
			   result, err := clawSvc.ExecuteLeg(dispclaw.ExecuteLegReq{
				   PlanID: plan.ID,
				  LegID:  legs[0].ID,
				   Actor:  tc.executorActor,
			  })
			  if tc.wantErr != "" {
				   if err == nil {
					  t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				  }
				  if !strings.Contains(err.Error(), tc.wantErr) {
					  t.Errorf("got %q want containing %q", err.Error(), tc.wantErr)
				  }
				  return
			   }
			   if err != nil {
				   t.Fatalf("unexpected error: %v", err)
			   }
			   if result.Status != dispclaw.LegStatusPosted {
				  t.Errorf("status=%q want posted", result.Status)
			  }
		   })
	   }
  }

// ─────────────────────────── 验收夹具 3 ──────────────────────────────────────
// TestDispClaw_RefundProofHashSet 验收夹具 3：
// refund 腿 posted 后 CloseProof 含腿哈希集合；篡改导致巡检失败。

   func TestDispClaw_RefundProofHashSet(t *testing.T) {
	   t.Parallel()
	   _, clawSvc := buildDeskWithGate(t)
	  caseID := "case-proof-f3"
	   plan, legs, err := clawSvc.GeneratePlan(dispclaw.GeneratePlanReq{
		   CaseID:         caseID,
		  PlanType:       dispclaw.PlanTypeRefund,
		   Actor:          "ops-alice",
		  IdempotencyKey: "idem-proof-f3",
	   }, "user-50003", "fiat", "USDT", "300.00", "ops-alice")
	   if err != nil {
		  t.Fatal(err)
	   }
	  // 执行所有腿（换人）。
	  for _, leg := range legs {
		  if _, err := clawSvc.ExecuteLeg(dispclaw.ExecuteLegReq{
			   PlanID: plan.ID,
			   LegID: leg.ID,
			  Actor: "ops-bob",
		   }); err != nil {
			   t.Fatalf("execute leg %s: %v", leg.ID, err)
		   }
	  }
	  // 收口闸应 passed。
	   gate, err := clawSvc.CheckCloseGate(dispclaw.CloseGateCheckReq{CaseID: caseID, Actor: "ops-bob"})
	   if err != nil {
		  t.Fatal(err)
	  }
	   if gate.GateStatus != dispclaw.GateStatusPassed {
		  t.Fatalf("gate status=%q want passed", gate.GateStatus)
	   }
	  // 生成收口证明。
	  proof, err := clawSvc.GenerateCloseProof(caseID, "ops-carol", "resolve")
	   if err != nil {
		  t.Fatal(err)
	   }
	   if proof.ProofHash == "" {
		  t.Error("proof_hash empty")
	   }
	  hashes, err := dispclaw.UnmarshalLegHashes(proof.LegHashes)
	   if err != nil {
		  t.Fatal(err)
	   }
	   if len(hashes) != len(legs) {
		  t.Errorf("leg_hashes count=%d want %d", len(hashes), len(legs))
	   }
	   for i, h := range hashes {
		  if h == "" {
			   t.Errorf("hashes[%d] empty", i)
		  }
	  }
	   // 正常巡检：不应检测到篡改。
	  _, err = clawSvc.PatrolProof(caseID)
	   if err != nil {
		  t.Errorf("patrol should pass: %v", err)
	   }
	   // 篡改：用被修改的腿数据验证检测逻辑。
	   tamperedLegs := make([]dispclaw.RecoveryLeg, len(legs))
	   copy(tamperedLegs, legs)
	  if len(tamperedLegs) > 0 {
		   tamperedLegs[0].Amount = "999999.00" // 篡改金额
	  }
	   if err := dispclaw.VerifyProof(proof, tamperedLegs); err != dispclaw.ErrProofTamper {
		   t.Errorf("expected ErrProofTamper for tampered legs, got %v", err)
	   }
  }

// TestDispClaw_RefundProofHashes_MultiLeg 验证多腿证明哈希完整性。

   func TestDispClaw_RefundProofHashes_MultiLeg(t *testing.T) {
	  t.Parallel()
	   _, clawSvc := buildDeskWithGate(t)
	   planTypes := []dispclaw.PlanType{
		  dispclaw.PlanTypeClawBack, // 2 腿
		   dispclaw.PlanTypeRefund,   // 2 腿
		   dispclaw.PlanTypeRestore, // 1 腿
	   }
	  expectedLegCounts := map[dispclaw.PlanType]int{
		   dispclaw.PlanTypeClawBack: 2,
		  dispclaw.PlanTypeRefund:   2,
		   dispclaw.PlanTypeRestore: 1,
	   }
	  for _, pt := range planTypes {
		  pt := pt
		   t.Run(string(pt), func(t *testing.T) {
			   t.Parallel()
			  caseID := "case-multileg-" + string(pt)
			   plan, legs, err := clawSvc.GeneratePlan(dispclaw.GeneratePlanReq{
				  CaseID:   caseID,
				   PlanType: pt,
				   Actor: "ops-alice",
				   IdempotencyKey: "idem-multileg-" + string(pt),
			   }, "user-50004", "p2p", "BTC", "0.1", "ops-alice")
			   if err != nil {
				  t.Fatal(err)
			   }
			  if len(legs) != expectedLegCounts[pt] {
				  t.Fatalf("leg count=%d want %d for %s", len(legs), expectedLegCounts[pt], pt)
			  }
			  for _, leg := range legs {
				   clawSvc.ExecuteLeg(dispclaw.ExecuteLegReq{PlanID: plan.ID, LegID: leg.ID, Actor: "ops-bob"})
			   }
			   clawSvc.CheckCloseGate(dispclaw.CloseGateCheckReq{CaseID: caseID, Actor: "ops-bob"})
			   proof, err := clawSvc.GenerateCloseProof(caseID, "ops-carol", "resolve")
			   if err != nil {
				   t.Fatal(err)
			  }
			  if proof.LegCount != expectedLegCounts[pt] {
				  t.Errorf("proof.LegCount=%d want %d", proof.LegCount, expectedLegCounts[pt])
			   }
			  hashes, _ := dispclaw.UnmarshalLegHashes(proof.LegHashes)
			   if len(hashes) != expectedLegCounts[pt] {
				   t.Errorf("hashes count=%d want %d", len(hashes), expectedLegCounts[pt])
			  }
		  })
	  }
   }

// ─────────────────────────── 验收夹具 4 ──────────────────────────────────────
// TestDispClaw_PlanSourceIsCaseID 验收夹具 4：
// 本包计划源必须是 dispute Case ID，不得扫描运营错贷记队列。
   func TestDispClaw_PlanSourceIsCaseID(t *testing.T) {
	  t.Parallel()
	  cases := []struct {
		   name    string
		   caseID  string
		   wantErr string
	  }{
		   {
			   name: "valid case_id accepted",
			  caseID: "case-valid-12345",
			   wantErr: "",
		  },
		   {
			   name: "empty case_id rejected",
			   caseID: "",
			  wantErr: "case not found",
		   },
		   {
			   name:   "whitespace case_id rejected",
			  caseID: "   ",
			   wantErr: "case not found",
		   },
	  }
	  for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			  _, clawSvc := buildDeskWithGate(t)
			  _, _, err := clawSvc.GeneratePlan(dispclaw.GeneratePlanReq{
				   CaseID: tc.caseID,
				  PlanType: dispclaw.PlanTypeRefund,
				   Actor:    "ops-alice",
				  IdempotencyKey: "idem-source-" + tc.name,
			  }, "user-50005", "fiat", "USDT", "100.00", "ops-alice")
			   if tc.wantErr != "" {
				   if err == nil {
					  t.Fatalf("expected error %q, got nil", tc.wantErr)
				   }
				  if !strings.Contains(err.Error(), tc.wantErr) {
					   t.Errorf("got %q want containing %q", err.Error(), tc.wantErr)
				   }
				   return
			  }
			  if err != nil {
				  t.Fatalf("unexpected error: %v", err)
			  }
		  })
	  }
   }

// ─────────────────────────── 验收夹具 5 ──────────────────────────────────────
// TestDispClaw_RailBreakIDReadonly 验收夹具 5：
// 可只读挂 rail_break_id；不写 fiatbreak 检测器。
   func TestDispClaw_RailBreakIDReadonly(t *testing.T) {
	  t.Parallel()
	   cases := []struct {
		  name string
		  railBreakID  string
		   wantAttached bool
	   }{
		  {
			  name: "attach rail_break_id",
			  railBreakID: "rb-fiat-20220315-001",
			  wantAttached: true,
		  },
		  {
			  name: "no rail_break_id (omitted)",
			   railBreakID: "",
			   wantAttached: false,
		   },
		  {
			   name: "attach rb from different PSP",
			   railBreakID: "rb-stripe-20220315-xyz",
			   wantAttached: true,
		   },
	  }
	  for _, tc := range cases {
		   tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   _, clawSvc := buildDeskWithGate(t)
			  caseID := "case-railbreak-" + tc.name
			   plan, _, err := clawSvc.GeneratePlan(dispclaw.GeneratePlanReq{
				  CaseID:   caseID,
				   PlanType: dispclaw.PlanTypeRefund,
				  Actor:          "ops-alice",
				   RailBreakID:    tc.railBreakID,
				  IdempotencyKey: "idem-rb-" + tc.name,
			   }, "user-50006", "fiat", "USDT", "150.00", "ops-alice")
			   if err != nil {
				  t.Fatal(err)
			   }
			  if tc.wantAttached && plan.RailBreakID != tc.railBreakID {
				  t.Errorf("plan.RailBreakID=%q want %q", plan.RailBreakID, tc.railBreakID)
			   }
			   if !tc.wantAttached && plan.RailBreakID != "" {
				   t.Errorf("plan.RailBreakID=%q want empty", plan.RailBreakID)
			  }
			  // 确认通过 GetPlanByCase 也能读到（只读引用持久化正确）。
			   loaded, err := clawSvc.GetPlanByCase(caseID)
			   if err != nil {
				  t.Fatal(err)
			  }
			  if tc.wantAttached && loaded.RailBreakID != tc.railBreakID {
				  t.Errorf("loaded.RailBreakID=%q want %q", loaded.RailBreakID, tc.railBreakID)
			   }
		  })
	  }
   }

// ─────────────────────────── 完整端到端流程 ─────────────────────────────────
// TestDispClaw_E2E_ClawBack 完整追偿端到端流程。

   func TestDispClaw_E2E_ClawBack(t *testing.T) {
	  t.Parallel()
	   desk,
		  clawSvc := buildDeskWithGate(t)
	   opener := "ops-alice"
	   executor := "ops-bob"
	  resolver := "ops-carol"
	  userID := "user-60001"
	   caseID := openCaseAndEvidence(t, desk, opener, userID, "idem-e2e-claw")
	  // 1. 生成追偿计划。
	  plan, legs, err := clawSvc.GeneratePlan(dispclaw.GeneratePlanReq{
		  CaseID:         caseID,
		   PlanType:       dispclaw.PlanTypeClawBack,
		   Actor:          opener,
		  IdempotencyKey: "idem-e2e-claw-plan",
	   }, userID, "fiat", "USDT", "500.00", opener)
	  if err != nil {
		  t.Fatalf("generate plan: %v", err)
	   }
	   if plan.Status != dispclaw.PlanStatusOpen {
		  t.Errorf("plan status=%q want open", plan.Status)
	  }
	  if len(legs) != 2 {
		   t.Fatalf("leg count=%d want 2", len(legs))
	  }
	  // 2. 开单人执行腿：四眼拒绝。
	   if _, err := clawSvc.ExecuteLeg(dispclaw.ExecuteLegReq{PlanID: plan.ID, LegID: legs[0].ID, Actor: opener}); err == nil {
		  t.Fatal("expected four-eye rejection for opener")
	   }
	  // 3. 换人执行第一条腿。
	   leg1, err := clawSvc.ExecuteLeg(dispclaw.ExecuteLegReq{PlanID: plan.ID, LegID: legs[0].ID, Actor: executor})
	  if err != nil {
		  t.Fatalf("execute leg1: %v", err)
	   }
	   if leg1.Status != dispclaw.LegStatusPosted {
		  t.Errorf("leg1 status=%q want posted", leg1.Status)
	   }
	   // 4. Resolve 仍应失败（第二条腿未完成）。
	   if _, err := desk.ResolveFourEye(caseID, resolver, "resolve", ""); err == nil {
		   t.Fatal("expected gate blocking after leg1 only")
	  }
	  // 5. 执行第二条腿。
	   leg2, err := clawSvc.ExecuteLeg(dispclaw.ExecuteLegReq{PlanID: plan.ID, LegID: legs[1].ID, Actor: executor})
	   if err != nil {
		  t.Fatalf("execute leg2: %v", err)
	   }
	   if leg2.Status != dispclaw.LegStatusPosted {
		  t.Errorf("leg2 status=%q want posted", leg2.Status)
	  }
	  // 6. 刷新收口闸。
	   gate, err := clawSvc.CheckCloseGate(dispclaw.CloseGateCheckReq{CaseID: caseID, Actor: executor})
	   if err != nil {
		  t.Fatal(err)
	   }
	  if gate.GateStatus != dispclaw.GateStatusPassed {
		   t.Errorf("gate=%q want passed", gate.GateStatus)
	  }
	   // 7. 生成收口证明。
	   proof, err := clawSvc.GenerateCloseProof(caseID, resolver, "resolve")
	  if err != nil {
		  t.Fatalf("generate proof: %v", err)
	   }
	   if proof.LegCount != 2 {
		  t.Errorf("proof.LegCount=%d want 2", proof.LegCount)
	   }
	  // 8. 现在 Resolve 应该成功。
	  if _, err := desk.ResolveFourEye(caseID, resolver, "resolve", "claw back completed"); err != nil {
		  t.Fatalf("resolve: %v", err)
	  }
	   // 9. 巡检证明应通过。
	  if _, err := clawSvc.PatrolProof(caseID); err != nil {
		  t.Errorf("patrol proof: %v", err)
	  }
  }

// TestDispClaw_E2E_Refund 完整退还端到端流程。

  func TestDispClaw_E2E_Refund(t *testing.T) {
	  t.Parallel()
	  desk, clawSvc := buildDeskWithGate(t)
	   opener := "ops-alice"
	  executor := "ops-eve"
	  resolver := "ops-frank"
	   userID := "user-60002"
	   caseID := openCaseAndEvidence(t, desk, opener, userID, "idem-e2e-refund")
	   plan, legs, err := clawSvc.GeneratePlan(dispclaw.GeneratePlanReq{
		   CaseID: caseID,
		  PlanType:       dispclaw.PlanTypeRefund,
		   Actor:          opener,
		  IdempotencyKey: "idem-e2e-refund-plan",
	   }, userID, "fiat", "USDT", "200.00", opener)
	  if err != nil {
		   t.Fatal(err)
	  }
	   for _, leg := range legs {
		  if _, err := clawSvc.ExecuteLeg(dispclaw.ExecuteLegReq{PlanID: plan.ID, LegID: leg.ID, Actor: executor}); err != nil {
			  t.Fatal(err)
		   }
	  }
	   clawSvc.CheckCloseGate(dispclaw.CloseGateCheckReq{CaseID: caseID,
		   Actor: executor})
	   proof, err := clawSvc.GenerateCloseProof(caseID, resolver, "resolve")
	  if err != nil {
		   t.Fatal(err)
	   }
	   hashes, _ := dispclaw.UnmarshalLegHashes(proof.LegHashes)
	   // refund = freeze + refund_credit
	   if len(hashes) != 2 {
		  t.Errorf("proof hashes count=%d want 2", len(hashes))
	   }
	  if _, err := desk.ResolveFourEye(caseID, resolver, "resolve", ""); err != nil {
		  t.Fatalf("resolve: %v", err)
	  }
  }

// ─────────────────────────── 计划中止 ─────────────────────────────────────────
// TestDispClaw_AbortPlanFlow 完整中止流程。
   func TestDispClaw_AbortPlanFlow(t *testing.T) {
	   t.Parallel()
	  _,
		   clawSvc := buildDeskWithGate(t)
	   caseID := "case-abort-flow"
	  plan, legs, err := clawSvc.GeneratePlan(dispclaw.GeneratePlanReq{
		  CaseID:   caseID,
		  PlanType: dispclaw.PlanTypeRefund,
		  Actor:    "ops-alice",
		  IdempotencyKey: "idem-abort-flow",
	  }, "user-70001", "fiat", "USDT", "100.00", "ops-alice")
	   if err != nil {
		   t.Fatal(err)
	  }
	   if len(legs) != 2 {
		  t.Fatalf("leg count=%d want 2", len(legs))
	   }
	  // 中止计划。
	  aborted, err := clawSvc.AbortPlan(dispclaw.AbortPlanReq{
		   PlanID: plan.ID,
		  Actor:       "ops-admin",
		   AbortReason: "审批超时，紧急中止",
	  })
	  if err != nil {
		   t.Fatal(err)
	  }
	   if aborted.Status != dispclaw.PlanStatusAborted {
		  t.Errorf("plan status=%q want aborted", aborted.Status)
	  }
	   // 所有腿应被跳过。
	   for _, leg := range clawSvc.ListLegs(plan.ID) {
		  if leg.Status != dispclaw.LegStatusSkipped {
			   t.Errorf("leg %s status=%q want skipped", leg.ID, leg.Status)
		  }
	   }
	   // 中止后不能再执行腿。
	  _, err = clawSvc.ExecuteLeg(dispclaw.ExecuteLegReq{
		  PlanID: plan.ID,
		   LegID:  legs[0].ID,
		   Actor: "ops-bob",
	  })
	   if err == nil {
		   t.Fatal("expected error for aborted plan, got nil")
	   }
   }

// ─────────────────────────── 指标统计 ─────────────────────────────────────────
// TestDispClaw_Counts 验证统计数字正确性。
   func TestDispClaw_Counts(t *testing.T) {
	  t.Parallel()
	   _, clawSvc := buildDeskWithGate(t)
	  initial := clawSvc.Counts()
	  if initial["plans_open"] != 0 {
		  t.Errorf("initial plans_open=%d want 0", initial["plans_open"])
	   }
	  // 创建 3 个计划。
	  for i := 0; i < 3; i++ {
		  suffix := "count-" + intToLetter(i)
		  clawSvc.GeneratePlan(dispclaw.GeneratePlanReq{
			  CaseID:         "case-" + suffix,
			   PlanType:       dispclaw.PlanTypeRefund,
			   Actor:          "ops-alice",
			  IdempotencyKey: "idem-" + suffix,
		  }, "user-80001", "fiat", "USDT", "50.00", "ops-alice")
	  }
	   counts := clawSvc.Counts()
	   if counts["plans_open"] != 3 {
		   t.Errorf("plans_open=%d want 3", counts["plans_open"])
	  }
	  if counts["legs_pending"] != 6 { // 每个 refund 计划有 2 条 pending 腿
		   t.Errorf("legs_pending=%d want 6", counts["legs_pending"])
	   }
	   if counts["gates_blocking"] != 3 {
		   t.Errorf("gates_blocking=%d want 3", counts["gates_blocking"])
	  }
  }

// ─────────────────────────── 工具函数 ──────────────────────────────
// intToLetter 将 0..25 映射为 A..Z（用于生成 suffix）。

   func intToLetter(i int) string {
	   return string([]byte{byte('A' + i)})
  }

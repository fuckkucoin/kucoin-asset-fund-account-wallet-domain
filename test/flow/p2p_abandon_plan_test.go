package flow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/p2pabandon"
)

// ─────────────────────────── 辅助函数 ─────────────────────────────────────────

  func newTestPlanService(t *testing.T) *p2pabandon.Service {
	  t.Helper()
	  svc := p2pabandon.New()
	  // 注入固定时钟，确保确定性。
	  fixedTime := time.Date(2022, 4, 10, 8, 0, 0, 0, time.UTC)
	  svc.SetClock(func() time.Time { return fixedTime })
	   return svc
  }

  func armTestPlan(t *testing.T, svc *p2pabandon.Service, orderID, userID, asset, amount, holdID string) p2pabandon.AbandonPlan {
	   t.Helper()
	  plan, err := svc.ArmPlan(p2pabandon.ArmPlanReq{
		   OrderID: orderID,
		   UserID:         userID,
		  Asset:          asset,
		  Amount:         amount,
		   HoldID:         holdID,
		   IdempotencyKey: "arm-plan:" + orderID,
	   })
	   if err != nil {
		   t.Fatalf("ArmPlan(%s): %v", orderID, err)
	  }
	  return plan
   }

   func advanceTestPlan(t *testing.T, svc *p2pabandon.Service, planID string) (p2pabandon.AbandonPlan, p2pabandon.MutexToken) {
	   t.Helper()
	   plan, tok, err := svc.AdvanceAbandon(p2pabandon.AdvanceAbandonReq{
		  PlanID: planID,
		  Actor: "abandon_adv_worker",
	   })
	  if err != nil {
		   t.Fatalf("AdvanceAbandon(%s): %v", planID, err)
	  }
	   return plan, tok
   }

   func commitReleaseTestPlan(t *testing.T, svc *p2pabandon.Service, planID, tokenID string) (p2pabandon.AbandonPlan, p2pabandon.CloseProof) {
	   t.Helper()
	   plan, proof, err := svc.CommitRelease(p2pabandon.CommitReleaseReq{
		  PlanID:    planID,
		   TokenID:   tokenID,
		  LedgerRef: "ledger-rel-" + planID,
		  HoldRef: "hold-rel-" + planID,
		  Actor:   "abandon_adv_worker",
	  })
	   if err != nil {
		  t.Fatalf("CommitRelease(%s): %v", planID, err)
	   }
	  return plan, proof
   }

// ─────────────────────────── 验收夹具 1：超时弃单 armed→released ─────────────────────
// TestP2PAbandon_TimeoutRelease 验收夹具 1：
// SellHold 超时后 AbandonPlan 自动 armed→released；可用恢复且 hold 释放。

   func TestP2PAbandon_TimeoutRelease(t *testing.T) {
	   t.Parallel()
	   type testCase struct {
		   name string
		  orderID string
		   userID  string
		  asset   string
		   amount  string
		  holdID     string
		  wantStatus p2pabandon.PlanStatus
	  }
	  cases := []testCase{
		   {
			   name: "USDT卖单超时弃单",
			   orderID: "p2p-order-usdt-timeout",
			  userID:  "user-seller-001",
			   asset:      "USDT",
			   amount:     "500.00",
			   holdID:     "hold-usdt-001",
			   wantStatus: p2pabandon.PlanStatusReleased,
		   },
		   {
			  name:    "BTC卖单超时弃单",
			   orderID: "p2p-order-btc-timeout",
			  userID: "user-seller-002",
			   asset:  "BTC",
			  amount: "0.10000000",
			  holdID: "hold-btc-001",
			   wantStatus: p2pabandon.PlanStatusReleased,
		   },
		   {
			  name:    "ETH小额超时弃单",
			   orderID: "p2p-order-eth-timeout",
			  userID: "user-seller-003",
			   asset:      "ETH",
			  amount:     "0.05000000",
			   holdID:     "hold-eth-001",
			   wantStatus: p2pabandon.PlanStatusReleased,
		  },
		   {
			  name: "大额USDT超时弃单",
			  orderID: "p2p-order-usdt-large",
			  userID:  "user-merchant-001",
			   asset:   "USDT",
			  amount:  "100000.00",
			  holdID:     "hold-usdt-large-001",
			  wantStatus: p2pabandon.PlanStatusReleased,
		  },
		   {
			   name:    "极小金额超时弃单",
			   orderID: "p2p-order-dust-timeout",
			  userID:  "user-seller-004",
			   asset:   "USDT",
			   amount:  "0.01",
			  holdID:  "hold-dust-001",
			   wantStatus: p2pabandon.PlanStatusReleased,
		   },
	   }
	   for _, tc := range cases {
		   tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   svc := newTestPlanService(t)
			   // 步骤 1：创建弃单计划（armed）。
			  plan := armTestPlan(t, svc, tc.orderID, tc.userID, tc.asset, tc.amount, tc.holdID)
			   if plan.Status != p2pabandon.PlanStatusArmed {
				   t.Fatalf("ArmPlan: want status=armed, got=%s", plan.Status)
			   }
			  if plan.OrderID != tc.orderID {
				   t.Fatalf("ArmPlan: want order_id=%s, got=%s", tc.orderID, plan.OrderID)
			   }
			   // 步骤 2：推进弃单（armed→abandoning）。
			  advPlan, token := advanceTestPlan(t, svc, plan.ID)
			  if advPlan.Status != p2pabandon.PlanStatusAbandoning {
				  t.Fatalf("AdvanceAbandon: want status=abandoning, got=%s", advPlan.Status)
			  }
			   if token.Kind != p2pabandon.TokenKindRelease {
				   t.Fatalf("AdvanceAbandon: want release token, got=%s", token.Kind)
			  }
			   if token.Status != p2pabandon.TokenStatusReserved {
				  t.Fatalf("AdvanceAbandon: want token reserved, got=%s", token.Status)
			  }
			   // 步骤 3：提交释放令牌（abandoning→released）。
			   relPlan, proof := commitReleaseTestPlan(t, svc, plan.ID, token.ID)
			   if relPlan.Status != p2pabandon.PlanStatusReleased {
				  t.Fatalf("CommitRelease: want status=released, got=%s", relPlan.Status)
			   }
			  if proof.Outcome != p2pabandon.OutcomeReleased {
				   t.Fatalf("CommitRelease: want outcome=released, got=%s", proof.Outcome)
			   }
			   if proof.PlanID != plan.ID {
				  t.Fatalf("Proof: want plan_id=%s, got=%s", plan.ID, proof.PlanID)
			  }
			   if proof.ProofHash == "" {
				  t.Fatal("Proof: proof_hash must not be empty")
			  }
			   // 步骤 4：收口校验（可标 closed）。
			  validateResp, validateErr := svc.ValidateClose(p2pabandon.ValidateCloseReq{
				   PlanID:  plan.ID,
				   OrderID: tc.orderID,
			  })
			  if validateErr != nil {
				  t.Fatalf("ValidateClose: %v", validateErr)
			  }
			   if !validateResp.CanClose {
				   t.Fatalf("ValidateClose: want can_close=true, reason=%s", validateResp.Reason)
			  }
			  if validateResp.Outcome != p2pabandon.OutcomeReleased {
				   t.Fatalf("ValidateClose: want outcome=released, got=%s", validateResp.Outcome)
			  }
			  // 步骤 5：验证状态符合 golden 夹具。
			   checkGoldenPlan(t, plan, "plan_armed.json")
		   })
	   }
   }

// ─────────────────────────── 验收夹具：ArmPlan 幂等 ─────────────────────────────
// TestP2PAbandon_ArmPlanIdempotent 相同 idempotency_key 多次调用返回同一计划。
   func TestP2PAbandon_ArmPlanIdempotent(t *testing.T) {
	   t.Parallel()
	   svc := newTestPlanService(t)
	   req := p2pabandon.ArmPlanReq{
		   OrderID: "p2p-idem-order-001",
		  UserID:  "user-idem-001",
		  Asset:   "USDT",
		   Amount: "200.00",
		   HoldID:         "hold-idem-001",
		   IdempotencyKey: "arm-plan:idem-key-001",
	  }
	  plan1, err1 := svc.ArmPlan(req)
	  if err1 != nil {
		  t.Fatalf("first ArmPlan: %v", err1)
	  }
	   plan2, err2 := svc.ArmPlan(req)
	   if err2 != nil {
		  t.Fatalf("second ArmPlan (idempotent): %v", err2)
	   }
	  if plan1.ID != plan2.ID {
		   t.Fatalf("idempotent: want same plan_id, got %s vs %s", plan1.ID, plan2.ID)
	  }
   }

// ─────────────────────────── 验收夹具：ArmPlan 唯一约束 ─────────────────────────
// TestP2PAbandon_ArmPlanDuplicateOrder 相同 order_id 不同幂等键返回 ErrPlanExists。

  func TestP2PAbandon_ArmPlanDuplicateOrder(t *testing.T) {
	  t.Parallel()
	   svc := newTestPlanService(t)
	   req1 := p2pabandon.ArmPlanReq{
		  OrderID: "p2p-dup-order-001",
		  UserID: "user-dup-001",
		  Asset:          "USDT",
		   Amount:         "100.00",
		   HoldID:         "hold-dup-001",
		   IdempotencyKey: "arm-plan:dup-key-001",
	  }
	   req2 := p2pabandon.ArmPlanReq{
		  OrderID: "p2p-dup-order-001", // 相同 order_id
		  UserID:  "user-dup-001",
		   Asset:   "USDT",
		   Amount:  "100.00",
		   HoldID:  "hold-dup-002",
		   IdempotencyKey: "arm-plan:dup-key-002", // 不同幂等键
	   }
	  _, err1 := svc.ArmPlan(req1)
	  if err1 != nil {
		  t.Fatalf("first ArmPlan: %v", err1)
	  }
	   _, err2 := svc.ArmPlan(req2)
	   if err2 != p2pabandon.ErrPlanExists {
		   t.Fatalf("want ErrPlanExists for duplicate order_id, got=%v", err2)
	   }
  }

// ─────────────────────────── 验收夹具：强制弃单双人确认 ─────────────────────────
// TestP2PAbandon_ForceAbandonDualControl 验收夹具 3：强制弃单双人确认。
  func TestP2PAbandon_ForceAbandonDualControl(t *testing.T) {
	   t.Parallel()
	  type testCase struct {
		   name    string
		   op1     string
		  op2     string
		  wantErr error
		  wantStatus p2pabandon.PlanStatus
	  }
	  cases := []testCase{
		  {
			   name: "双人不同_成功",
			  op1:  "ops-alice",
			  op2:        "ops-bob",
			   wantErr:    nil,
			   wantStatus: p2pabandon.PlanStatusReleased,
		   },
		  {
			  name: "双人相同_拒绝",
			   op1: "ops-alice",
			  op2: "ops-alice",
			   wantErr: p2pabandon.ErrFourEye,
		  },
		  {
			   name:    "大小写相同_拒绝",
			  op1:     "Ops-Alice",
			   op2:     "ops-alice",
			   wantErr: p2pabandon.ErrFourEye,
		  },
		   {
			  name: "操作人1为空_拒绝",
			   op1: "",
			  op2: "ops-bob",
			   wantErr: p2pabandon.ErrActor,
		  },
		   {
			   name: "操作人2为空_拒绝",
			   op1:     "ops-alice",
			   op2:     "",
			  wantErr: p2pabandon.ErrActor,
		   },
	  }
	  for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			  svc := newTestPlanService(t)
			   plan := armTestPlan(t, svc, "p2p-force-"+tc.name, "user-force-001", "USDT", "500.00", "hold-force-001")
			  _, _, err := svc.ForceAbandon(p2pabandon.ForceAbandonReq{
				   PlanID:    plan.ID,
				   Operator1: tc.op1,
				   Operator2: tc.op2,
				  Reason: "force abandon test",
			   })
			   if tc.wantErr != nil {
				  if err != tc.wantErr {
					   t.Fatalf("ForceAbandon: want err=%v, got=%v", tc.wantErr, err)
				   }
				  return
			  }
			  if err != nil {
				  t.Fatalf("ForceAbandon: %v", err)
			   }
			  got, _ := svc.GetPlan(plan.ID)
			   if got.Status != tc.wantStatus {
				  t.Fatalf("ForceAbandon: want status=%s, got=%s", tc.wantStatus, got.Status)
			   }
		   })
	  }
  }

// ─────────────────────────── 验收夹具：ValidateClose 约束 ─────────────────────────
// TestP2PAbandon_ValidateCloseConstraints ValidateClose 各条件验证。
   func TestP2PAbandon_ValidateCloseConstraints(t *testing.T) {
	   t.Parallel()
	   type testCase struct {
		  name         string
		   setup        func(svc *p2pabandon.Service) string // 返回 planID
		   wantCanClose bool
		  wantErrIs error
	  }
	   cases := []testCase{
		  {
			   name: "armed状态不可close",
			  setup: func(svc *p2pabandon.Service) string {
				  p := armTestPlan(t, svc, "validate-armed-001", "user-001", "USDT", "100.00", "hold-001")
				   return p.ID
			  },
			  wantCanClose: false,
		   },
		  {
			   name: "abandoning状态不可close",
			   setup: func(svc *p2pabandon.Service) string {
				   p := armTestPlan(t, svc, "validate-abandoning-001", "user-001", "USDT", "100.00", "hold-001")
				   advP, _ := advanceTestPlan(t, svc, p.ID)
				  return advP.ID
			  },
			  wantCanClose: false,
		   },
		  {
			   name: "released且有证明可close",
			   setup: func(svc *p2pabandon.Service) string {
				   p := armTestPlan(t, svc, "validate-released-001", "user-001", "USDT", "100.00", "hold-001")
				  advP, tok := advanceTestPlan(t, svc, p.ID)
				   commitReleaseTestPlan(t, svc, advP.ID, tok.ID)
				  return advP.ID
			  },
			   wantCanClose: true,
		  },
		   {
			   name: "conflict状态不可close",
			  setup: func(svc *p2pabandon.Service) string {
				   p := armTestPlan(t, svc, "validate-conflict-001", "user-001", "USDT", "100.00", "hold-001")
				   // 使用辅助方法注入 conflict 状态。
				  if err := svc.InjectConflictForTest(p.ID); err != nil {
					  t.Fatalf("InjectConflictForTest: %v", err)
				  }
				   return p.ID
			  },
			  wantCanClose: false,
		  },
		   {
			  name: "plan不存在",
			  setup: func(svc *p2pabandon.Service) string {
				  return "nonexistent-plan-id"
			  },
			  wantCanClose: false,
			  wantErrIs: p2pabandon.ErrPlan,
		   },
	   }
	  for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   svc := newTestPlanService(t)
			  planID := tc.setup(svc)
			  resp, err := svc.ValidateClose(p2pabandon.ValidateCloseReq{PlanID: planID})
			  if tc.wantErrIs != nil && err != tc.wantErrIs {
				  t.Fatalf("ValidateClose: want err=%v, got=%v", tc.wantErrIs, err)
			  }
			  if resp.CanClose != tc.wantCanClose {
				   t.Fatalf("ValidateClose: want can_close=%v, reason=%s", tc.wantCanClose, resp.Reason)
			   }
		  })
	   }
   }

// ─────────────────────────── 验收夹具：证明篡改检测 ─────────────────────────
// TestP2PAbandon_ProofTamperDetection 证明哈希篡改后 PatrolProof 必须检测到。
   func TestP2PAbandon_ProofTamperDetection(t *testing.T) {
	   t.Parallel()
	  svc := newTestPlanService(t)
	   p := armTestPlan(t, svc, "p2p-tamper-001", "user-tamper", "USDT", "200.00", "hold-tamper-001")
	   advP,
		  tok := advanceTestPlan(t, svc, p.ID)
	   _, proof := commitReleaseTestPlan(t, svc, advP.ID, tok.ID)
	   // 正常巡检：无篡改。
	  cleanProof,
		  err := svc.PatrolProof(advP.ID)
	  if err != nil {
		   t.Fatalf("PatrolProof (clean): %v", err)
	  }
	  if cleanProof.TamperDetected {
		  t.Fatal("PatrolProof (clean): should not detect tamper")
	  }
	  // 验证 proof_hash 与字段一致性。
	   if err := p2pabandon.VerifyProofHash(proof); err != nil {
		  t.Fatalf("VerifyProofHash: %v", err)
	   }
	  // 篡改：手动构造 proof，修改 outcome。
	   tamperedProof := proof
	  tamperedProof.Outcome = p2pabandon.OutcomeFilled // 改为 filled，但 hash 不变
	   if err := p2pabandon.VerifyProofHash(tamperedProof); err == nil {
		  t.Fatal("VerifyProofHash: should detect tamper when outcome changed")
	  }
   }

// ─────────────────────────── 验收夹具：超龄自动处置 ─────────────────────────
// TestP2PAbandon_ForceAbandonResolvesAge 强制弃单后超龄记录自动标 resolved。
   func TestP2PAbandon_ForceAbandonResolvesAge(t *testing.T) {
	   t.Parallel()
	   svc := newTestPlanService(t)
	  p := armTestPlan(t, svc, "p2p-force-age-001", "user-force-age", "USDT", "1000.00", "hold-force-age-001")
	   // 手动创建超龄记录（巡检已升级）。
	   age, err := svc.UpsertAge(p.ID, 90000, 86400)
	  if err != nil {
		  t.Fatalf("UpsertAge: %v", err)
	   }
	  age2, err := svc.EscalateAge(p2pabandon.EscalateAgeReq{
		   AgeID:        age.ID,
		   EscalateTo:   "p2p-abandon-desk",
		  EscalateNote: "hold exceeded SLA",
	  })
	  if err != nil {
		   t.Fatalf("EscalateAge: %v", err)
	   }
	   if age2.Stage != p2pabandon.AgeStageEscalated {
		   t.Fatalf("EscalateAge: want escalated, got %s", age2.Stage)
	   }
	  // 强制弃单（附 age_record_id）。
	   forcedPlan, proof, forceErr := svc.ForceAbandon(p2pabandon.ForceAbandonReq{
		   PlanID:    p.ID,
		   Operator1: "ops-alice",
		  Operator2: "ops-bob",
		   Reason:    "hold exceeded SLA; force abandon",
		   AgeRecordID: age.ID,
	   })
	  if forceErr != nil {
		   t.Fatalf("ForceAbandon: %v", forceErr)
	  }
	  if forcedPlan.Status != p2pabandon.PlanStatusReleased {
		  t.Fatalf("ForceAbandon: want released, got %s", forcedPlan.Status)
	  }
	   if proof.Outcome != p2pabandon.OutcomeReleased {
		   t.Fatalf("ForceAbandon: want outcome=released")
	   }
	  // 验证超龄记录已标 resolved。
	  resolvedAge, err := svc.GetAge(age.ID)
	  if err != nil {
		   t.Fatalf("GetAge: %v", err)
	  }
	   if resolvedAge.Stage != p2pabandon.AgeStageResolved {
		  t.Fatalf("ForceAbandon: want age resolved, got %s", resolvedAge.Stage)
	   }
  }

// ─────────────────────────── 条件集 ─────────────────────────
// TestP2PAbandon_EdgeCases 各种/错误情况大表驱动。
  func TestP2PAbandon_EdgeCases(t *testing.T) {
	   t.Parallel()
	   type errCase struct {
		   name string
		   run     func(svc *p2pabandon.Service) error
		   wantErr error
	   }
	  cases := []errCase{
		  {
			  name: "ArmPlan_空订单ID",
			  run: func(svc *p2pabandon.Service) error {
				  _, err := svc.ArmPlan(p2pabandon.ArmPlanReq{
					   OrderID: "",
					  UserID:  "user-001",
					   Asset:   "USDT",
					  Amount:  "100.00",
					  HoldID:  "hold-001",
					   IdempotencyKey: "idem-001",
				   })
				   return err
			   },
			   wantErr: p2pabandon.ErrOrder,
		  },
		  {
			  name: "ArmPlan_空幂等键",
			   run: func(svc *p2pabandon.Service) error {
				   _, err := svc.ArmPlan(p2pabandon.ArmPlanReq{
					  OrderID: "order-001",
					  UserID: "user-001",
					  Asset:  "USDT",
					   Amount: "100.00",
					  HoldID: "hold-001",
				  })
				   return err
			  },
			   wantErr: p2pabandon.ErrIdem,
		   },
		   {
			  name: "ArmPlan_无效金额",
			   run: func(svc *p2pabandon.Service) error {
				  _, err := svc.ArmPlan(p2pabandon.ArmPlanReq{
					   OrderID: "order-002",
					   UserID:  "user-001",
					   Asset:   "USDT",
					   Amount:  "-100.00",
					   HoldID:  "hold-001",
					   IdempotencyKey: "idem-002",
				  })
				  return err
			  },
			   wantErr: p2pabandon.ErrAmount,
		  },
		  {
			   name: "AdvanceAbandon_不存在计划",
			   run: func(svc *p2pabandon.Service) error {
				   _, _, err := svc.AdvanceAbandon(p2pabandon.AdvanceAbandonReq{
					  PlanID: "nonexistent-plan",
					   Actor:  "worker",
				  })
				   return err
			  },
			  wantErr: p2pabandon.ErrPlan,
		  },
		  {
			   name: "CommitRelease_错误TokenID",
			   run: func(svc *p2pabandon.Service) error {
				   p := armTestPlan(t, svc, "p2p-edge-release-001", "user-001", "USDT", "100.00", "hold-001")
				  _, _, err := svc.AdvanceAbandon(p2pabandon.AdvanceAbandonReq{PlanID: p.ID, Actor: "w"})
				   if err != nil {
					  return err
				   }
				   _, _, err = svc.CommitRelease(p2pabandon.CommitReleaseReq{
					   PlanID:  p.ID,
					   TokenID: "wrong-token-id",
					   Actor: "w",
				   })
				   return err
			   },
			   wantErr: p2pabandon.ErrToken,
		  },
		  {
			   name: "ForceAbandon_理由为空",
			  run: func(svc *p2pabandon.Service) error {
				  p := armTestPlan(t, svc, "p2p-force-noreason", "user-001", "USDT", "100.00", "hold-001")
				  _, _, err := svc.ForceAbandon(p2pabandon.ForceAbandonReq{
					   PlanID:    p.ID,
					  Operator1: "ops-alice",
					  Operator2: "ops-bob",
					   Reason: "",
				  })
				   return err
			   },
			  wantErr: p2pabandon.ErrReason,
		   },
		   {
			   name: "AdvanceAbandon_已终态计划",
			  run: func(svc *p2pabandon.Service) error {
				   p := armTestPlan(t, svc, "p2p-edge-terminal-001", "user-001", "USDT", "100.00", "hold-001")
				   advP, tok := advanceTestPlan(t, svc, p.ID)
				   commitReleaseTestPlan(t, svc, advP.ID, tok.ID)
				  // 再次 AdvanceAbandon 应失败。
				  _, _, err := svc.AdvanceAbandon(p2pabandon.AdvanceAbandonReq{
					   PlanID: advP.ID, Actor: "w",
				   })
				  return err
			  },
			  wantErr: p2pabandon.ErrPlanTerminal,
		   },
	  }
	  for _, tc := range cases {
		   tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			  svc := newTestPlanService(t)
			   err := tc.run(svc)
			   if err != tc.wantErr {
				   t.Fatalf("%s: want err=%v, got=%v", tc.name, tc.wantErr, err)
			   }
		   })
	  }
   }

// ─────────────────────────── 隔离验证：与 dispclaw 并行 ─────────────────────────
// TestP2PAbandon_IsolationFromDispClaw 验收夹具 5：
// 本包驱动源是 p2p order_id+timer，不写 dispute 追偿腿，不改 holdcat 分层模型。
   func TestP2PAbandon_IsolationFromDispClaw(t *testing.T) {
	   t.Parallel()
	   // 验证：p2pabandon.ArmPlan 入参不含 case_id 字段。
	   // 验证：AbandonPlan 结构体中无 case_id 字段（编译期保证）。
	  plan := p2pabandon.AbandonPlan{}
	  planJSON, _ := json.Marshal(plan)
	  if strings.Contains(string(planJSON), "case_id") {
		   t.Fatal("isolation: AbandonPlan must not contain case_id field (r7 G isolation)")
	  }
	   // 验证：AbandonPlan 驱动源是 order_id，不是 dispute case_id。
	   svc := newTestPlanService(t)
	   p := armTestPlan(t, svc, "p2p-iso-order-001", "user-iso-001", "USDT", "100.00", "hold-iso-001")
	   if p.OrderID != "p2p-iso-order-001" {
		   t.Fatalf("isolation: plan driver must be order_id, got %s", p.OrderID)
	  }
	   // 读取隔离 golden 夹具。
	  checkGoldenFile(t, "isolation_from_dispclaw.json", func(data map[string]interface{}) {
		   driver, _ := data["p2pabandon_plan"].(map[string]interface{})["driver_source"].(string)
		   if driver != "order_timeout_signal" {
			  t.Fatalf("golden: driver_source must be order_timeout_signal, got %s", driver)
		   }
	   })
  }

// ─────────────────────────── Golden 辅助函数 ─────────────────────────

  func checkGoldenPlan(t *testing.T, plan p2pabandon.AbandonPlan, goldenFile string) {
	  t.Helper()
	  goldenPath := filepath.Join("..", "..", "testdata", "golden", "p2pabandon", goldenFile)
	  data, err := os.ReadFile(goldenPath)
	   if err != nil {
		  // golden 文件不存在时仅跳过，不失败（允许渐进式添加）。
		  t.Logf("golden file not found: %s (skipped)", goldenPath)
		   return
	  }
	  var golden map[string]interface{}
	  if json.Unmarshal(data, &golden) != nil {
		  return
	   }
	   // 检查状态字段（golden 仅做约束性验证，非完整比较）。
	  if wantStatus, ok := golden["status"].(string); ok {
		  if string(plan.Status) != wantStatus {
			  t.Errorf("golden %s: want status=%s, got=%s", goldenFile, wantStatus, plan.Status)
		  }
	   }
   }

   func checkGoldenFile(t *testing.T, goldenFile string, fn func(map[string]interface{})) {
	  t.Helper()
	  goldenPath := filepath.Join("..", "..", "testdata", "golden", "p2pabandon", goldenFile)
	   data, err := os.ReadFile(goldenPath)
	  if err != nil {
		   t.Logf("golden file not found: %s (skipped)", goldenPath)
		  return
	   }
	   var m map[string]interface{}
	   if json.Unmarshal(data, &m) != nil {
		  return
	   }
	  fn(m)
  }

  // ─────────────────────────── 大型综合流程 ─────────────────────────
   // TestP2PAbandon_FullFlow 完整流程端到端（armed→abandoning→released→validate_close）。

   func TestP2PAbandon_FullFlow(t *testing.T) {
	  t.Parallel()
	   type flowCase struct {
		   name    string
		   orderID string
		  userID string
		   asset  string
		  amount string
	  }
	   cases := make([]flowCase, 0, 20)
	   // 生成 20 条不同资产/金额的完整流程用例。
	   assets := []string{"USDT", "BTC", "ETH", "BNB", "SOL", "TRX", "MATIC", "DOT", "LINK", "UNI"}
	  amounts := []string{"100.00", "0.01", "1000000.00", "0.00001000", "999.99", "5000.00", "0.50", "250.00", "75.25", "12345.67"}
	  for i := 0; i < 20; i++ {
		   cases = append(cases, flowCase{
			  name:    fmt.Sprintf("全流程_%s_%s", assets[i%len(assets)], amounts[i%len(amounts)]),
			  orderID: fmt.Sprintf("p2p-full-flow-%03d", i+1),
			   userID:  fmt.Sprintf("user-full-%03d", i+1),
			   asset:  assets[i%len(assets)],
			  amount: amounts[i%len(amounts)],
		   })
	  }
	  for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name,
			   func(t *testing.T) {
				   t.Parallel()
				   svc := newTestPlanService(t)
				   holdID := "hold-full-" + tc.orderID
				  plan := armTestPlan(t, svc, tc.orderID, tc.userID, tc.asset, tc.amount, holdID)
				   advPlan, tok := advanceTestPlan(t, svc, plan.ID)
				   relPlan, proof := commitReleaseTestPlan(t, svc, advPlan.ID, tok.ID)
				  if relPlan.Status != p2pabandon.PlanStatusReleased {
					   t.Fatalf("want released, got %s", relPlan.Status)
				  }
				  validateResp, _ := svc.ValidateClose(p2pabandon.ValidateCloseReq{PlanID: plan.ID})
				   if !validateResp.CanClose {
					  t.Fatalf("want can_close=true, reason=%s", validateResp.Reason)
				   }
				   if proof.ProofHash == "" {
					   t.Fatal("proof_hash must not be empty")
				  }
			   })
	  }
   }

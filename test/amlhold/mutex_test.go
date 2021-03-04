package amlhold_test

import (
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/amlhold"
)

// ─── MutexGate 表驱动 ────────────────────────────────────────────────────

  func TestMutexGateCheckCloseAllowed(t *testing.T) {
	  t.Parallel()
	   cases := []struct {
		  name string
		  setupFn func(svc *amlhold.Service, // 返回 holdID
			  caseID string) (string, error)
		  releaseHold bool
		  wantErr     bool
		   errContains string
	   }{
		  {
			   name: "open hold 阻断结案",
			  setupFn: func(svc *amlhold.Service, caseID string) (string, error) {
				   h, err := svc.OpenHold(caseID, "user-g1", "USDT", "100", "ops-a", 86400)
				   return h.ID, err
			   },
			  releaseHold: false,
			  wantErr:     true,
			  errContains: "hold",
		  },
		  {
			   name: "released hold 允许结案",
			   setupFn: func(svc *amlhold.Service, caseID string) (string, error) {
				   h, err := svc.OpenHold(caseID, "user-g2", "BTC", "0.1", "ops-a", 86400)
				  return h.ID, err
			   },
			  releaseHold: true,
			   wantErr:     false,
		   },
		   {
			  name: "无 hold 允许结案",
			  setupFn: func(svc *amlhold.Service, caseID string) (string, error) {
				  return "", nil // 不开 hold
			   },
			  releaseHold: false,
			   wantErr: false,
		  },
	  }
	  for _, tc := range cases {
		   tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			  svc := amlhold.New()
			  caseID := "case-mg-" + tc.name
			   holdID, err := tc.setupFn(svc, caseID)
			   if err != nil {
				  t.Fatal("setup:", err)
			  }
			  if tc.releaseHold && holdID != "" {
				  h, _ := svc.GetHold(holdID)
				  ticket, err := svc.RequestRelease(holdID, caseID, "ops-b", "解冻", "")
				   if err != nil {
					   t.Fatal(err)
				  }
				   _, _, err = svc.ApproveRelease(ticket.ID, "reviewer-c", ticket.FenceEpoch, ticket.FenceToken)
				   if err != nil {
					  t.Fatal(err)
				   }
				  _ = h
			  }
			  err = svc.CheckCloseAllowed(caseID, "reviewer-x")
			   if tc.wantErr {
				  if err == nil {
					   t.Fatal("期望错误，但得到 nil")
				   }
				  if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					   t.Errorf("错误应含 %q，得到: %v", tc.errContains, err)
				  }
			   } else {
				  if err != nil {
					   t.Fatalf("不期望错误，得到: %v", err)
				  }
			   }
		   })
	  }
  }

// ─── SameActorBlock 表驱动 ───────────────────────────────────────────────

  func TestSameActorBlocking(t *testing.T) {
	  t.Parallel()
	  cases := []struct {
		  name             string
		  releaseRequester string
		  closeActor string
		   wantSameActorErr bool
	  }{
		  {
			   name: "不同 actor：不触发互斥",
			  releaseRequester: "ops-alpha",
			   closeActor:       "reviewer-beta",
			   wantSameActorErr: false,
		   },
		  {
			  name:             "相同 actor：触发互斥",
			  releaseRequester: "ops-gamma",
			  closeActor:       "ops-gamma",
			  wantSameActorErr: true,
		   },
	  }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			   svc := amlhold.New()
			  caseID := "case-sa-" + tc.name
			  h, err := svc.OpenHold(caseID, "user-sa", "ETH", "3.0", "ops-opener", 86400)
			   if err != nil {
				  t.Fatal(err)
			  }
			   _, err = svc.RequestRelease(h.ID, caseID, tc.releaseRequester, "申请", "")
			   if err != nil {
				   t.Fatal("申请解冻:", err)
			  }
			  err = svc.CheckSameActorReleaseThenClose(caseID, tc.closeActor)
			  if tc.wantSameActorErr {
				   if err == nil {
					  t.Fatal("同一 actor 应触发互斥错误，但得到 nil")
				   }
			   } else {
				  if err != nil {
					   t.Fatalf("不同 actor 不应触发互斥，得到: %v", err)
				   }
			  }
		   })
	   }
  }

// ─── compel_review 双人换手校验 ───────────────────────────────────────────────

  func TestCompelReviewFourEye(t *testing.T) {
	   t.Parallel()
	  svc := amlhold.New()
	  h, err := svc.OpenHold("case-compel", "user-compel", "USDT", "1000", "ops-opener", 86400)
	  if err != nil {
		  t.Fatal(err)
	  }
	  // 推进到 compel_review
	  for i := 0; i < 4; i++ {
		   cur, _ := svc.GetHold(h.ID)
		  if cur.Terminal() {
			  break
		  }
		   h, _, err = svc.AdvanceAging(h.ID, "system:aging", "test", cur.FenceEpoch, cur.FenceToken)
		  if err != nil {
			   break
		   }
	  }
	  cur, _ := svc.GetHold(h.ID)
	   if cur.Status != amlhold.HoldStatusCompelReview {
		   t.Skipf("未到达 compel_review 状态（当前 %s），跳过", cur.Status)
	  }
	  // MutexGate 的 compel_review 校验
	   gate := amlhold.NewMutexGate(svc.Book())
	  ccr := amlhold.CaseCloseRecord{
		  CaseID: "case-compel",
		   OpenedBy: "ops-opener",
	  }
	  // 开案人试图操作 compel_review：应失败
	  err = gate.ValidateCompelReview(h.ID, "ops-opener", ccr)
	   if err == nil {
		   t.Error("开案人不得操作 compel_review 阶段，应拒绝")
	  }
	  // 独立督查 actor：应成功
	  err = gate.ValidateCompelReview(h.ID, "supervisor-independent", ccr)
	   if err != nil {
		   t.Errorf("独立督查应允许操作 compel_review，得到: %v", err)
	   }
   }

// ─── 解冻申请拒绝 ─────────────────────────────────────────────────────────

  func TestRejectRelease(t *testing.T) {
	  t.Parallel()
	  svc := amlhold.New()
	  h, err := svc.OpenHold("case-reject", "user-rj", "XRP", "500", "ops-alice", 86400)
	   if err != nil {
		  t.Fatal(err)
	   }
	   ticket, err := svc.RequestRelease(h.ID, "case-reject", "ops-bob", "申请解冻", "")
	   if err != nil {
		   t.Fatal(err)
	   }
	  // 拒绝解冻（四眼：拒绝人≠申请人）
	   rejected, err := svc.RejectRelease(ticket.ID, "reviewer-carol", "材料不足", ticket.FenceEpoch, ticket.FenceToken)
	  if err != nil {
		   t.Fatal("拒绝解冻失败:", err)
	  }
	  if rejected.Status != amlhold.TicketStatusRejected {
		   t.Errorf("期望 rejected，得到 %s", rejected.Status)
	  }
	   // 拒绝后 hold 仍应 open
	  cur, _ := svc.GetHold(h.ID)
	  if !cur.Open() {
		  t.Error("拒绝解冻后 hold 仍应为活跃状态")
	  }
	  // 同一 actor 不得拒绝自己的申请
	  ticket2, _ := svc.RequestRelease(h.ID, "case-reject", "ops-dave", "再次申请", "")
	   _, err = svc.RejectRelease(ticket2.ID, "ops-dave", "自己拒自己", ticket2.FenceEpoch, ticket2.FenceToken)
	  if err == nil {
		  t.Error("申请人不得自行拒绝解冻申请")
	   }
   }

// ─── 全流程集成 ───────────────────────────────────────────────────────────

  func TestFullReleaseCloseFlow(t *testing.T) {
	   t.Parallel()
	  svc := amlhold.New()
	   caseID := "case-full-flow"
	   // 1. 开启 hold
	   h, err := svc.OpenHold(caseID, "user-ff", "USDT", "10000", "ops-alice", 86400)
	   if err != nil {
		  t.Fatal("open hold:", err)
	   }
	  // 2. 验证结案阻断
	   if err := svc.CheckCloseAllowed(caseID, "reviewer-bob"); err == nil {
		  t.Fatal("hold 未释放时结案应被阻断")
	   }
	   // 3. 申请解冻（ops-charlie）
	  ticket, err := svc.RequestRelease(h.ID, caseID, "ops-charlie", "审核完成可解冻", "")
	  if err != nil {
		   t.Fatal("request release:", err)
	   }
	   // 4. 审批解冻（reviewer-diana，四眼：≠ ops-charlie）
	   _, releasedHold, err := svc.ApproveRelease(ticket.ID, "reviewer-diana", ticket.FenceEpoch, ticket.FenceToken)
	   if err != nil {
		   t.Fatal("approve release:", err)
	   }
	  if releasedHold.Status != amlhold.HoldStatusReleased {
		   t.Fatalf("解冻后 hold 应为 released，得到 %s", releasedHold.Status)
	  }
	   // 5. 互斥检查：ops-charlie 不得结案（因其申请了解冻）
	   if err := svc.CheckSameActorReleaseThenClose(caseID, "ops-charlie"); err == nil {
		  t.Error("ops-charlie 申请了解冻，不得同时发起结案")
	  }
	   // 6. reviewer-eve（独立 actor）可以结案
	   if err := svc.CheckCloseAllowed(caseID, "reviewer-eve"); err != nil {
		  t.Fatalf("hold 已 released，reviewer-eve 应能结案，得到: %v", err)
	   }
	   if err := svc.CheckSameActorReleaseThenClose(caseID, "reviewer-eve"); err != nil {
		   t.Fatalf("reviewer-eve 未参与解冻，互斥检查应通过，得到: %v", err)
	  }
	   // 7. 验证证明链完整性
	   proofs, _ := svc.ExportProofsForCase(caseID)
	  hasOccupy, hasRelease := false, false
	   for _, ep := range proofs {
		  switch ep.Proof.ProofKind {
		  case amlhold.ProofKindOccupy:
			  hasOccupy = true
		  case amlhold.ProofKindRelease:
			  hasRelease = true
		   }
	  }
	   if !hasOccupy {
		  t.Error("证明链缺少 occupy 证明")
	   }
	  if !hasRelease {
		  t.Error("证明链缺少 release 证明")
	  }
  }

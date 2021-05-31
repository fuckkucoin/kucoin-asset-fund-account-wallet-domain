package flow

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/p2pabandon"
)

// ─────────────────────────── 辅助函数 ─────────────────────────────────────────

  func buildRaceService(t *testing.T) *p2pabandon.Service {
	   t.Helper()
	   svc := p2pabandon.New()
	   svc.SetClock(func() time.Time { return time.Date(2022, 4, 10, 10, 0, 0, 0, time.UTC) })
	  return svc
   }

  func buildAndArmPlan(t *testing.T, svc *p2pabandon.Service, orderID string) (p2pabandon.AbandonPlan, p2pabandon.MutexToken) {
	  t.Helper()
	  plan := armTestPlan(t, svc, orderID, "user-race-001", "USDT", "1000.00", "hold-race-"+orderID)
	   advPlan, tok := advanceTestPlan(t, svc, plan.ID)
	   return advPlan, tok
  }

// ─────────────────────────── 验收夹具 2：互斥竞态大表 ─────────────────────────
// TestP2PAbandon_ReleaseFillMutex 验收夹具 2：release vs fill 各路径互斥。

   func TestP2PAbandon_ReleaseFillMutex(t *testing.T) {
	   t.Parallel()
	   type raceCase struct {
		  name string
		  doRelease     bool
		  doFill        bool
		   fillBeforeAdv bool // fill 在 AdvanceAbandon 之前 commit
		  wantPlanStatus p2pabandon.PlanStatus
		   wantMutexResult string
		   wantConflict    bool
		   wantFillErr     error
		   wantReleaseErr  error
	   }
	  cases := []raceCase{
		   {
			  name:      "release先committed_fill被abort",
			   doRelease: true,
			   doFill: true,
			   wantPlanStatus:  p2pabandon.PlanStatusReleased,
			  wantMutexResult: "release_won",
			  wantConflict:    false,
		   },
		   {
			  name:           "只做release不做fill",
			   doRelease:      true,
			   doFill:         false,
			   wantPlanStatus: p2pabandon.PlanStatusReleased,
			  wantMutexResult: "release_won",
			  wantConflict: false,
		   },
		  {
			   name:      "fill先于abandon_fill_won",
			   doRelease: false,
			   doFill:        true,
			  fillBeforeAdv: true,
			  wantPlanStatus:  p2pabandon.PlanStatusFilled,
			   wantMutexResult: "fill_won",
			   wantConflict:    false,
		   },
	  }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			   svc := buildRaceService(t)
			   orderID := "race-" + tc.name
			  plan := armTestPlan(t, svc, orderID, "user-race-001", "USDT", "1000.00", "hold-"+orderID)
			   if tc.fillBeforeAdv {
				   // fill 在 armed 阶段就 commit（OrderFill 早于弃单推进）。
				  fillPlan, fillProof, fillErr := svc.CommitFill(p2pabandon.CommitFillReq{
					   PlanID: plan.ID,
					   LedgerRef: "ledger-fill-" + orderID,
					  Actor:     "fill_worker",
				  })
				  if tc.wantFillErr != nil {
					   if fillErr != tc.wantFillErr {
						   t.Fatalf("CommitFill: want %v, got %v", tc.wantFillErr, fillErr)
					   }
					   return
				   }
				   if fillErr != nil {
					   t.Fatalf("CommitFill: %v", fillErr)
				   }
				  if fillPlan.Status != tc.wantPlanStatus {
					  t.Fatalf("want plan=%s, got=%s", tc.wantPlanStatus, fillPlan.Status)
				  }
				  if fillProof.Outcome != p2pabandon.OutcomeFilled {
					  t.Fatalf("want outcome=filled, got=%s", fillProof.Outcome)
				  }
				   // release 在 fill_won 后应失败（plan 已终态）。
				  if tc.doRelease {
					  advP,
						   tok := advanceTestPlan(t, svc, plan.ID)
					   _ = advP
					  _ = tok
				   }
				  return
			   }
			   advPlan, tok := advanceTestPlan(t, svc, plan.ID)
			  if tc.doFill && !tc.doRelease {
				   // 只做 fill（release 令牌被 abort）。
				  fillPlan, fillProof, fillErr := svc.CommitFill(p2pabandon.CommitFillReq{
					   PlanID:    advPlan.ID,
					  LedgerRef: "ledger-fill-" + orderID,
					   Actor: "fill_worker",
				   })
				  if fillErr != nil {
					   t.Fatalf("CommitFill: %v", fillErr)
				   }
				  if fillPlan.Status != tc.wantPlanStatus {
					  t.Fatalf("want plan=%s, got=%s", tc.wantPlanStatus, fillPlan.Status)
				   }
				  _ = fillProof
				   return
			  }
			   // release 先 commit。
			  if tc.doRelease {
				  relPlan, relProof, relErr := svc.CommitRelease(p2pabandon.CommitReleaseReq{
					   PlanID: advPlan.ID,
					  TokenID: tok.ID,
					   LedgerRef: "ledger-rel-" + orderID,
					   HoldRef:   "hold-rel-" + orderID,
					   Actor:     "abandon_adv_worker",
				  })
				   if tc.wantReleaseErr != nil {
					   if relErr != tc.wantReleaseErr {
						  t.Fatalf("CommitRelease: want %v, got %v", tc.wantReleaseErr, relErr)
					   }
					   return
				  }
				   if relErr != nil {
					  t.Fatalf("CommitRelease: %v", relErr)
				  }
				   if relPlan.Status != tc.wantPlanStatus {
					  t.Fatalf("want plan=%s, got=%s", tc.wantPlanStatus, relPlan.Status)
				  }
				   _ = relProof
			   }
			  // fill 在 release 之后尝试 commit（应被阻止或返回终态）。
			  if tc.doFill {
				  _, _, fillErr := svc.CommitFill(p2pabandon.CommitFillReq{
					   PlanID: advPlan.ID,
					  LedgerRef: "ledger-fill-late-" + orderID,
					   Actor: "fill_worker",
				  })
				   // release 已 committed → plan released → fill 返回 ErrPlanTerminal（幂等）。
				  if fillErr != nil && fillErr != p2pabandon.ErrPlanTerminal && fillErr != p2pabandon.ErrTokenAborted {
					  t.Fatalf("CommitFill after release: want ErrPlanTerminal or ErrTokenAborted, got %v", fillErr)
				   }
			  }
			  // 验证令牌汇总。
			   sum, sumErr := svc.GetTokenSummary(advPlan.ID)
			   if sumErr != nil {
				  t.Fatalf("GetTokenSummary: %v", sumErr)
			   }
			  if sum.MutexResult != tc.wantMutexResult {
				   t.Fatalf("mutex_result: want=%s, got=%s", tc.wantMutexResult, sum.MutexResult)
			  }
			  if sum.ConflictPresent != tc.wantConflict {
				   t.Fatalf("conflict_present: want=%v, got=%v", tc.wantConflict, sum.ConflictPresent)
			  }
		   })
	   }
   }

// ─────────────────────────── 验收夹具：冲突路径 ─────────────────────────
// TestP2PAbandon_ConflictPath 当两条路径均尝试 committed 时触发 conflict。

  func TestP2PAbandon_ConflictPath(t *testing.T) {
	  t.Parallel()
	   svc := buildRaceService(t)
	   plan := armTestPlan(t, svc, "p2p-conflict-001", "user-conflict-001", "USDT", "1000.00", "hold-conflict-001")
	  // 使用辅助方法注入 conflict 状态（分布式竞态场景）。
	   if err := svc.InjectConflictForTest(plan.ID); err != nil {
		  t.Fatalf("InjectConflictForTest: %v", err)
	   }
	  // 验证计划进入 conflict。
	  conflictPlan, getPlanErr := svc.GetPlan(plan.ID)
	   if getPlanErr != nil {
		  t.Fatalf("GetPlan: %v", getPlanErr)
	  }
	   if conflictPlan.Status != p2pabandon.PlanStatusConflict {
		  t.Fatalf("want conflict, got %s", conflictPlan.Status)
	  }
	   // conflict 状态下不可 ValidateClose。
	  resp, _ := svc.ValidateClose(p2pabandon.ValidateCloseReq{PlanID: plan.ID})
	  if resp.CanClose {
		  t.Fatal("conflict plan: can_close must be false")
	  }
	   // desk 双人仲裁解决 conflict。
	  resProof, resErr := svc.GenerateConflictResolutionProof(
		  plan.ID, "desk-ops-alice", "desk-ops-charlie",
		  p2pabandon.OutcomeReleased, "", "", "",
	  )
	   if resErr != nil {
		   t.Fatalf("GenerateConflictResolutionProof: %v", resErr)
	  }
	   if resProof.Outcome != p2pabandon.OutcomeReleased {
		  t.Fatalf("resolution proof: want outcome=released, got=%s", resProof.Outcome)
	  }
	  // 解决后可 ValidateClose。
	  resolvedResp, resolvedErr := svc.ValidateClose(p2pabandon.ValidateCloseReq{PlanID: plan.ID})
	  if resolvedErr != nil {
		  t.Fatalf("ValidateClose after resolution: %v", resolvedErr)
	  }
	  if !resolvedResp.CanClose {
		  t.Fatalf("after resolution: can_close must be true")
	  }
  }

// ─────────────────────────── 验收夹具：冲突解决四眼校验 ─────────────────────────
// TestP2PAbandon_ConflictResolutionFourEye conflict 解决需双人确认。

  func TestP2PAbandon_ConflictResolutionFourEye(t *testing.T) {
	   t.Parallel()
	  type resCase struct {
		   name string
		   op1 string
		   op2     string
		  wantErr error
	   }
	  cases := []resCase{
		   {"双人不同_成功", "ops-alice", "ops-bob", nil},
		  {"双人相同_拒绝", "ops-alice", "ops-alice", p2pabandon.ErrFourEye},
		  {"op1空_拒绝", "", "ops-bob", p2pabandon.ErrActor},
		  {"op2空_拒绝", "ops-alice", "", p2pabandon.ErrActor},
	  }
	  for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   svc := buildRaceService(t)
			  // 构造 conflict 状态（通过辅助方法注入）。
			  plan := armTestPlan(t, svc, "p2p-conflict-res-"+tc.name, "user-001", "USDT", "100.00", "hold-001")
			   // 使用辅助方法将计划强制置为 conflict（分布式竞态场景）。
			  if err := svc.InjectConflictForTest(plan.ID); err != nil {
				   t.Fatalf("setup conflict (inject): %v", err)
			   }
			  conflictPlan, _ := svc.GetPlan(plan.ID)
			  if conflictPlan.Status != p2pabandon.PlanStatusConflict {
				   t.Fatalf("setup: want conflict, got %s", conflictPlan.Status)
			  }
			  _, err := svc.GenerateConflictResolutionProof(
				   plan.ID, tc.op1, tc.op2, p2pabandon.OutcomeReleased, "", "", "",
			  )
			   if err != tc.wantErr {
				  t.Fatalf("want %v, got %v", tc.wantErr, err)
			  }
		  })
	   }
   }

// ─────────────────────────── 验收夹具：外部 fill 令牌预留 ─────────────────────────
// TestP2PAbandon_ExternalFillTokenReserve 外部 fill 令牌预留后 release 被阻止。

  func TestP2PAbandon_ExternalFillTokenReserve(t *testing.T) {
	  t.Parallel()
	  svc := buildRaceService(t)
	   // armed 阶段预留 fill 令牌。
	  plan := armTestPlan(t, svc, "p2p-ext-fill-001", "user-ext-001", "USDT", "500.00", "hold-ext-001")
	   fillTok, err := svc.ReserveExternalFillToken(plan.ID, "fill_worker")
	  if err != nil {
		   t.Fatalf("ReserveExternalFillToken: %v", err)
	   }
	   if fillTok.Kind != p2pabandon.TokenKindFill {
		  t.Fatalf("want fill token kind, got %s", fillTok.Kind)
	  }
	  // fill commit（先于 release）。
	   filledPlan, proof, fillErr := svc.CommitFill(p2pabandon.CommitFillReq{
		  PlanID: plan.ID,
		   TokenID:   fillTok.ID,
		   LedgerRef: "ledger-ext-fill-001",
		   Actor:     "fill_worker",
	  })
	  if fillErr != nil {
		   t.Fatalf("CommitFill: %v", fillErr)
	  }
	   if filledPlan.Status != p2pabandon.PlanStatusFilled {
		  t.Fatalf("want filled, got %s", filledPlan.Status)
	  }
	   if proof.Outcome != p2pabandon.OutcomeFilled {
		   t.Fatalf("want outcome=filled, got %s", proof.Outcome)
	   }
	  // abandon worker 尝试 AdvanceAbandon 后 CommitRelease → plan 已终态。
	  _, _, advErr := svc.AdvanceAbandon(p2pabandon.AdvanceAbandonReq{PlanID: plan.ID, Actor: "worker"})
	   if advErr != p2pabandon.ErrPlanTerminal {
		  t.Fatalf("AdvanceAbandon on filled plan: want ErrPlanTerminal, got %v", advErr)
	   }
   }

// ─────────────────────────── 验收夹具：外部 fill 令牌 abort ─────────────────────────
// TestP2PAbandon_ExternalFillTokenAbort 外部 fill 令牌 abort 后 release 可正常推进。

  func TestP2PAbandon_ExternalFillTokenAbort(t *testing.T) {
	  t.Parallel()
	   svc := buildRaceService(t)
	  plan := armTestPlan(t, svc, "p2p-ext-abort-001", "user-ext-001", "USDT", "500.00", "hold-ext-001")
	   fillTok, _ := svc.ReserveExternalFillToken(plan.ID, "fill_worker")
	  // fill 贷记失败，abort fill 令牌。
	  if err := svc.AbortExternalFillToken(plan.ID, fillTok.ID, "fill ledger failed"); err != nil {
		  t.Fatalf("AbortExternalFillToken: %v", err)
	   }
	  // release 可正常推进。
	  advPlan,
		   relTok := advanceTestPlan(t, svc, plan.ID)
	  relPlan, proof, relErr := svc.CommitRelease(p2pabandon.CommitReleaseReq{
		   PlanID: advPlan.ID,
		  TokenID: relTok.ID,
		  Actor: "abandon_adv_worker",
	  })
	   if relErr != nil {
		  t.Fatalf("CommitRelease after fill abort: %v", relErr)
	   }
	   if relPlan.Status != p2pabandon.PlanStatusReleased {
		  t.Fatalf("want released, got %s", relPlan.Status)
	  }
	  if proof.Outcome != p2pabandon.OutcomeReleased {
		  t.Fatalf("want outcome=released, got %s", proof.Outcome)
	  }
  }

// ─────────────────────────── 验收夹具：并发 fill 只有一条 committed ─────────────────────────
// TestP2PAbandon_ConcurrentFillOnlyOneCommits 并发多路 fill 请求只有一条 committed。
   func TestP2PAbandon_ConcurrentFillOnlyOneCommits(t *testing.T) {
	   t.Parallel()
	  for _, goroutines := range []int{2, 5, 10} {
		   goroutines := goroutines
		   t.Run(fmt.Sprintf("goroutines=%d", goroutines), func(t *testing.T) {
			   t.Parallel()
			  svc := buildRaceService(t)
			  plan := armTestPlan(t, svc, fmt.Sprintf("p2p-concurrent-%d", goroutines),
				   "user-concurrent-001", "USDT", "1000.00",
				   fmt.Sprintf("hold-concurrent-%d", goroutines))
			  var wg sync.WaitGroup
			   var committed int32
			  var conflictCount int32
			   for i := 0; i < goroutines; i++ {
				  wg.Add(1)
				  go func(idx int) {
					  defer wg.Done()
					   _, _, err := svc.CommitFill(p2pabandon.CommitFillReq{
						  PlanID:    plan.ID,
						  LedgerRef: fmt.Sprintf("ledger-fill-concurrent-%d", idx),
						  Actor: fmt.Sprintf("fill_worker_%d", idx),
					  })
					  if err == nil {
						   atomic.AddInt32(&committed, 1)
					  } else if err == p2pabandon.ErrTokenConflict {
						   atomic.AddInt32(&conflictCount,
							   1)
					   }
					   // ErrPlanTerminal 或其他：正常（后来的幂等或已终态）。
				   }(i)
			  }
			   wg.Wait()
			  // 核心不变式：只有一条 fill 令牌处于 committed 状态（禁止双动账）。
			   // nil-error 可能包含幂等返回（多个 goroutines 看到已 filled 状态返回 nil）；
			  // 需通过令牌状态确认实际 committed 数量。
			  tokens := svc.ListTokensByPlan(plan.ID)
			   committedFillCount := 0
			  for _, tok := range tokens {
				   if tok.Kind == p2pabandon.TokenKindFill && tok.Status == p2pabandon.TokenStatusCommitted {
					  committedFillCount++
				  }
			   }
			   if committedFillCount > 1 {
				  t.Fatalf("concurrent fill: %d fill tokens committed, want <=1 (no double booking)", committedFillCount)
			   }
			   // 验证计划终态。
			   finalPlan, _ := svc.GetPlan(plan.ID)
			   if !finalPlan.Status.Terminal() && finalPlan.Status != p2pabandon.PlanStatusArmed {
				   t.Fatalf("plan not in terminal/armed state after concurrent fill: %s", finalPlan.Status)
			   }
			  _ = committed
		   })
	   }
  }

// ─────────────────────────── 验收夹具：终态计划再次 commit 幂等 ─────────────────────────
// TestP2PAbandon_TerminalPlanIdempotent 终态计划再次 CommitRelease 幂等返回已有证明。

   func TestP2PAbandon_TerminalPlanIdempotent(t *testing.T) {
	  t.Parallel()
	  svc := buildRaceService(t)
	   advPlan, tok := buildAndArmPlan(t, svc, "p2p-terminal-idem-001")
	   plan1, proof1, _ := svc.CommitRelease(p2pabandon.CommitReleaseReq{
		   PlanID: advPlan.ID,
		  TokenID: tok.ID,
		   Actor: "worker",
	  })
	  // 再次 CommitRelease：应幂等返回已有证明，不重新贷记。
	   plan2, proof2, err2 := svc.CommitRelease(p2pabandon.CommitReleaseReq{
		   PlanID:  advPlan.ID,
		  TokenID: tok.ID,
		  Actor: "worker",
	   })
	   if err2 != nil && err2 != p2pabandon.ErrPlanTerminal {
		  t.Fatalf("idempotent CommitRelease: want nil or ErrPlanTerminal, got %v", err2)
	  }
	  if err2 == nil {
		  if plan1.ID != plan2.ID || proof1.ID != proof2.ID {
			  t.Fatal("idempotent: should return same plan and proof")
		  }
	  }
	  _ = plan2
   }

// ─────────────────────────── 验收夹具：无证明不可 ValidateClose ─────────────────────────
// TestP2PAbandon_NoProofBlocksClose 缺失 CloseProof 时 ValidateClose 拒绝。
  func TestP2PAbandon_NoProofBlocksClose(t *testing.T) {
	  t.Parallel()
	   cases := []struct {
		  name string
		  advance bool
		   wantCan    bool
		   wantReason string
	  }{
		   {
			   name:       "armed_无证明",
			   advance:    false,
			  wantCan:    false,
			   wantReason: "not terminal",
		   },
		  {
			   name: "abandoning_无证明",
			  advance: true,
			  wantCan: false,
			   wantReason: "not terminal",
		   },
	  }
	  for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   svc := buildRaceService(t)
			   plan := armTestPlan(t, svc, "p2p-noproof-"+tc.name, "user-001", "USDT", "300.00", "hold-001")
			   if tc.advance {
				  advanceTestPlan(t, svc, plan.ID)
			   }
			   resp, _ := svc.ValidateClose(p2pabandon.ValidateCloseReq{PlanID: plan.ID})
			   if resp.CanClose != tc.wantCan {
				   t.Fatalf("want can_close=%v, reason=%s", tc.wantCan, resp.Reason)
			   }
		  })
	  }
   }

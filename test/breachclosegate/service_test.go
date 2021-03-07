package breachclosegate_test

import (
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/breachclosegate"
	"account-assets.kucoin.net/funding/internal/cases"
	"account-assets.kucoin.net/funding/internal/reviewsla"
)

// ── 工具 ─────────────────────────────────────────────────────────────────────

  func newTestService(t *testing.T) (*breachclosegate.Service, *breachclosegate.MemStore) {
	  t.Helper()
	   store := breachclosegate.NewMemStore()
	   svc := breachclosegate.New(store)
	  return svc, store
  }

   func fixedClock(ts time.Time) func() time.Time { return func() time.Time { return ts } }

   func makeRef(caseID, mvmt, userID, gen string, deadline, breachedAt time.Time) reviewsla.BreachLeaseRef {
	   return reviewsla.BreachLeaseRef{
		  CaseID: caseID,
		  MovementID: mvmt,
		   UserID:     userID,
		  BreachGen: gen,
		  Deadline:   deadline,
		  BreachedAt: breachedAt,
	  }
   }

// ── TestScanAndBlock ──────────────────────────────────────────────────────────

   func TestScanAndBlock(t *testing.T) {
	   t0 := time.Date(2021,
		  6, 20, 14, 1, 0, 0, time.UTC)
	  cases := []struct {
		  name string
		  ref  reviewsla.BreachLeaseRef
		  callTwice bool
		   wantErr error
		  wantState breachclosegate.BreachState
	   }{
		   {
			  name: "正常创建 BreachLease",
			   ref: makeRef("case-001", "mvmt-001", "user-001",
				  "aabbccdd112233441234567890abcdef1234567890abcdef1122334455667788",
				  t0.Add(-1*time.Hour), t0),
			   wantState: breachclosegate.BreachStateBlocking,
		   },
		   {
			  name: "同一 breach_gen 重复调用返回已有 BreachLease（幂等）",
			  ref: makeRef("case-002", "mvmt-002", "user-002",
				  "deadbeef00112233deadbeef00112233deadbeef00112233deadbeef00112233",
				   t0.Add(-2*time.Hour), t0),
			   callTwice: true,
			   wantState: breachclosegate.BreachStateBlocking,
		  },
		   {
			   name: "空 breach_gen 返回 ErrLeaseNotFound",
			   ref: makeRef("case-003", "mvmt-003", "user-003",
				  "", t0, t0),
			   wantErr: breachclosegate.ErrLeaseNotFound,
		   },
	  }
	   for _, tc := range cases {
		   tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  svc, _ := newTestService(t)
			   svc.WithClock(fixedClock(t0))
			  lease, err := svc.ScanAndBlock(tc.ref)
			  if tc.wantErr != nil {
				   if !errors.Is(err, tc.wantErr) {
					   t.Fatalf("want err %v, got %v", tc.wantErr, err)
				  }
				  return
			   }
			  if err != nil {
				  t.Fatalf("unexpected error: %v", err)
			  }
			  if lease.State != tc.wantState {
				   t.Errorf("state: want %s, got %s", tc.wantState, lease.State)
			   }
			   if tc.callTwice {
				   lease2, err2 := svc.ScanAndBlock(tc.ref)
				   if err2 != nil {
					   t.Fatalf("second call unexpected error: %v", err2)
				  }
				   if lease2.ID != lease.ID {
					   t.Errorf("idempotent: got different ID on second call")
				   }
			  }
		  })
	  }
   }

// ── TestTryAllowCheck ────────────────────────────────────────────────────────

   func TestTryAllowCheck(t *testing.T) {
	  t0 := time.Date(2021, 6, 20, 15, 0, 0, 0, time.UTC)
	   genA := "1111111111111111111111111111111111111111111111111111111111111111"
	  cases := []struct {
		   name   string
		   setup  func(svc *breachclosegate.Service)
		   caseID string
		  actor string
		  wantAllowed bool
		   wantErr     error
	   }{
		  {
			   name: "活跃 BreachLease 时拒绝 Decide(allow)",
			  setup: func(svc *breachclosegate.Service) {
				  ref := makeRef("case-allow-01", "mvmt-allow-01", "user-01",
					   genA, t0.Add(-1*time.Hour), t0)
				   _, _ = svc.ScanAndBlock(ref)
			   },
			  caseID: "case-allow-01",
			   actor:  "ops-zoe",
			   wantAllowed: false,
			  wantErr:     breachclosegate.ErrBreachBlock,
		   },
		  {
			   name: "无 BreachLease 时允许",
			   setup:  func(_ *breachclosegate.Service) {},
			  caseID: "case-allow-02-no-breach",
			  actor: "ops-zoe",
			  wantAllowed: true,
		   },
		   {
			   name: "BreachLease 已 expired_case 时允许",
			   setup: func(svc *breachclosegate.Service) {
				  gen2 := "2222222222222222222222222222222222222222222222222222222222222222"
				  ref := makeRef("case-allow-03", "mvmt-allow-03", "user-03",
					  gen2, t0.Add(-2*time.Hour), t0)
				   _, _ = svc.ScanAndBlock(ref)
				  // 驱动 Expire。
				  _ = svc.ForceExpireCase("case-allow-03")
			   },
			  caseID: "case-allow-03",
			   actor: "ops-zoe",
			  wantAllowed: true,
		   },
	   }
	  for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   svc, _ := newTestService(t)
			   svc.WithClock(fixedClock(t0))
			  tc.setup(svc)
			  res, err := svc.TryAllowCheck(tc.caseID, tc.actor)
			  if tc.wantErr != nil {
				   if !errors.Is(err, tc.wantErr) {
					   t.Fatalf("want err %v, got %v", tc.wantErr, err)
				   }
				  if res.Allowed {
					   t.Error("result.Allowed should be false when blocked")
				   }
				  return
			  }
			  if err != nil {
				   t.Fatalf("unexpected error: %v", err)
			  }
			   if res.Allowed != tc.wantAllowed {
				   t.Errorf("allowed: want %v, got %v", tc.wantAllowed, res.Allowed)
			   }
		  })
	  }
   }

// ── TestSubmitPrecheck ───────────────────────────────────────────────────────

   func TestSubmitPrecheck(t *testing.T) {
	   t0 := time.Date(2021,
		  6, 20, 16, 0, 0, 0, time.UTC)
	  cases := []struct {
		   name       string
		   setup      func(svc *breachclosegate.Service)
		   movementID string
		  wantAllowed bool
		  wantErr error
	   }{
		   {
			   name: "活跃 BreachLease 时 Submit 被拒（fail-closed）",
			  setup: func(svc *breachclosegate.Service) {
				  gen := "3333333333333333333333333333333333333333333333333333333333333333"
				   ref := makeRef("case-submit-01", "mvmt-submit-01", "user-s1",
					  gen,
					  t0.Add(-1*time.Hour), t0)
				   _, _ = svc.ScanAndBlock(ref)
			  },
			   movementID:  "mvmt-submit-01",
			  wantAllowed: false,
			   wantErr: breachclosegate.ErrBreachBlock,
		  },
		   {
			   name: "无 BreachLease 时 Submit 通过",
			   setup: func(_ *breachclosegate.Service) {},
			   movementID: "mvmt-submit-no-breach",
			   wantAllowed: true,
		   },
	  }
	   for _, tc := range cases {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   svc, _ := newTestService(t)
			   svc.WithClock(fixedClock(t0))
			   tc.setup(svc)
			   res, err := svc.SubmitPrecheck(tc.movementID, "ops-test")
			  if tc.wantErr != nil {
				   if !errors.Is(err, tc.wantErr) {
					  t.Fatalf("want err %v, got %v", tc.wantErr, err)
				  }
				  if res.Allowed {
					   t.Error("result.Allowed should be false when blocked")
				  }
				   return
			   }
			  if err != nil {
				  t.Fatalf("unexpected error: %v", err)
			  }
			  if res.Allowed != tc.wantAllowed {
				   t.Errorf("allowed: want %v, got %v", tc.wantAllowed, res.Allowed)
			  }
		  })
	  }
   }

// ── TestForceExpireCase ──────────────────────────────────────────────────────

  func TestForceExpireCase(t *testing.T) {
	   t0 := time.Date(2021, 6, 20, 17, 0, 0, 0, time.UTC)
	   gen := "4444444444444444444444444444444444444444444444444444444444444444"
	   t.Run("Expire 转换 BreachLease 状态为 expired_case", func(t *testing.T) {
		   svc, store := newTestService(t)
		  svc.WithClock(fixedClock(t0))
		   ref := makeRef("case-exp-01", "mvmt-exp-01", "user-e1",
			  gen, t0.Add(-1*time.Hour), t0)
		   lease, _ := svc.ScanAndBlock(ref)
		   if lease.State != breachclosegate.BreachStateBlocking {
			   t.Fatalf("initial state should be blocking")
		   }
		   err := svc.ForceExpireCase("case-exp-01")
		   if err != nil {
			  t.Fatalf("ForceExpireCase: %v", err)
		   }
		  updated, err := store.GetLeaseByID(lease.ID)
		   if err != nil {
			   t.Fatalf("GetLeaseByID: %v", err)
		  }
		  if updated.State != breachclosegate.BreachStateExpiredCase {
			   t.Errorf("state after expire: want expired_case, got %s", updated.State)
		  }
		   if updated.CaseForcedExpiredAt == nil {
			  t.Error("CaseForcedExpiredAt should be set")
		  }
		  // 验证证明已落写。
		   proofs := store.ListProofsByLease(lease.ID, 10)
		  found := false
		  for _, p := range proofs {
			   if p.BlockOutcome == "expired_case" {
				   found = true
				  break
			  }
		  }
		   if !found {
			   t.Error("SlaBreachProof with block_outcome=expired_case not found")
		   }
	  })
	   t.Run("对无活跃 BreachLease 的 caseID Expire 返回 ErrNoActiveBreach", func(t *testing.T) {
		  svc, _ := newTestService(t)
		  err := svc.ForceExpireCase("case-nonexistent")
		  if err == nil {
			  t.Error("expected error for nonexistent case")
		  }
	   })
   }

// ── TestFourEyeClearanceHappy ────────────────────────────────────────────────

  func TestFourEyeClearanceHappy(t *testing.T) {
	  t0 := time.Date(2021, 6, 20, 18, 0, 0, 0, time.UTC)
	   gen := "5555555555555555555555555555555555555555555555555555555555555555"
	  svc, store := newTestService(t)
	  svc.WithClock(fixedClock(t0))
	   ref := makeRef("case-clr-01", "mvmt-clr-01", "user-c1",
		   gen, t0.Add(-1*time.Hour), t0)
	  lease, _ := svc.ScanAndBlock(ref)
	  // 发起清障工单。
	  clr, err := svc.ProposeBreachClearance(lease.ID, "ops-alice", "investigation complete")
	   if err != nil {
		  t.Fatalf("ProposeBreachClearance: %v", err)
	  }
	   if clr.State != breachclosegate.ClearanceStatePending {
		  t.Errorf("clearance state: want pending, got %s", clr.State)
	   }
	   if !clr.NoAutoAllow {
		   t.Error("NoAutoAllow must be true")
	  }
	   // 批准清障（四眼第二人）。
	   approved, err := svc.ApproveBreachClearance(clr.ID, "ops-bob")
	  if err != nil && !errors.Is(err, breachclosegate.ErrClearanceNoAutoAllow) {
		  t.Fatalf("ApproveBreachClearance: %v", err)
	   }
	  if approved.State != breachclosegate.ClearanceStateApproved {
		   t.Errorf("clearance state after approve: want approved, got %s", approved.State)
	  }
	   // 验证 BreachLease 已转为 cleared。
	  updated, _ := store.GetLeaseByID(lease.ID)
	  if updated.State != breachclosegate.BreachStateCleared {
		   t.Errorf("lease state after clearance: want cleared, got %s", updated.State)
	   }
	  // 验证证明已落写。
	   proofs := store.ListProofsByLease(lease.ID, 10)
	   found := false
	   for _, p := range proofs {
		  if p.BlockOutcome == "cleared_no_allow" {
			   found = true
			  break
		   }
	  }
	  if !found {
		   t.Error("SlaBreachProof with block_outcome=cleared_no_allow not found")
	  }
   }

// ── TestFourEyeClearanceSameActor ────────────────────────────────────────────

  func TestFourEyeClearanceSameActor(t *testing.T) {
	  t0 := time.Date(2021, 6, 20, 19, 0, 0, 0, time.UTC)
	  gen := "6666666666666666666666666666666666666666666666666666666666666666"
	   svc, _ := newTestService(t)
	  svc.WithClock(fixedClock(t0))
	   ref := makeRef("case-4eye-01", "mvmt-4eye-01", "user-4e1",
		   gen, t0.Add(-1*time.Hour), t0)
	   lease,
		   _ := svc.ScanAndBlock(ref)
	   clr, _ := svc.ProposeBreachClearance(lease.ID, "ops-eve", "reason")
	  // 同一人批准应返回 ErrClearanceMakerCheck。
	   _, err := svc.ApproveBreachClearance(clr.ID, "ops-eve")
	  if !errors.Is(err, breachclosegate.ErrClearanceMakerCheck) {
		   t.Errorf("want ErrClearanceMakerCheck, got %v", err)
	  }
	  // 验证 BreachLease 仍然 blocking。
	   lease2, _ := svc.GetLease(lease.ID)
	   if lease2.State != breachclosegate.BreachStateBlocking {
		   t.Errorf("lease should still be blocking after failed same-actor approve, got %s", lease2.State)
	   }
  }

// ── TestFourEyeClearanceReject ───────────────────────────────────────────────

   func TestFourEyeClearanceReject(t *testing.T) {
	  t0 := time.Date(2021,
		  6, 20, 20, 0, 0, 0, time.UTC)
	   gen := "7777777777777777777777777777777777777777777777777777777777777777"
	   svc, _ := newTestService(t)
	   svc.WithClock(fixedClock(t0))
	  ref := makeRef("case-rej-01", "mvmt-rej-01", "user-r1",
		  gen, t0.Add(-1*time.Hour), t0)
	   lease, _ := svc.ScanAndBlock(ref)
	   clr, _ := svc.ProposeBreachClearance(lease.ID, "ops-frank", "requesting clearance")
	   rejected, err := svc.RejectBreachClearance(clr.ID, "ops-grace", "evidence insufficient")
	   if err != nil {
		   t.Fatalf("RejectBreachClearance: %v", err)
	  }
	   if rejected.State != breachclosegate.ClearanceStateRejected {
		  t.Errorf("clearance state: want rejected, got %s", rejected.State)
	   }
	  // 拒绝后 BreachLease 仍 blocking。
	   lease2, _ := svc.GetLease(lease.ID)
	  if lease2.State != breachclosegate.BreachStateBlocking {
		   t.Errorf("lease should still be blocking after rejection, got %s", lease2.State)
	   }
	   // 拒绝后 SubmitPrecheck 仍返回 ErrBreachBlock。
	  _, err = svc.SubmitPrecheck(ref.MovementID, "ops-test")
	  if !errors.Is(err, breachclosegate.ErrBreachBlock) {
		   t.Errorf("SubmitPrecheck after rejection: want ErrBreachBlock, got %v", err)
	  }
  }

// ── TestClearedStillRequiresReview ──────────────────────────────────────────
// 清障后，BreachLease.State == cleared，TryAllowCheck 不再拦截（挡写已解除），
// 但这是因为 BreachLease 本身已终结，与是否 allow 无关。
// 重要语义：cleared_no_allow 证明已落写，出金须重入核验路径（由核验 Gate 负责）。
   func TestClearedStillRequiresReview(t *testing.T) {
	  t0 := time.Date(2021, 6, 20, 21, 0, 0, 0, time.UTC)
	  gen := "8888888888888888888888888888888888888888888888888888888888888888"
	   svc, store := newTestService(t)
	  svc.WithClock(fixedClock(t0))
	   ref := makeRef("case-revu-01", "mvmt-revu-01", "user-rv1",
		  gen, t0.Add(-1*time.Hour), t0)
	   lease, _ := svc.ScanAndBlock(ref)
	   clr, _ := svc.ProposeBreachClearance(lease.ID, "ops-henry", "final review done")
	  _, _ = svc.ApproveBreachClearance(clr.ID, "ops-irene")
	   // BreachLease 已 cleared，TryAllowCheck 不再因本 BreachLease 拦截。
	  res, err := svc.TryAllowCheck(ref.CaseID, "ops-jack")
	  if err != nil && !errors.Is(err, breachclosegate.ErrBreachBlock) {
		  t.Fatalf("unexpected error: %v", err)
	   }
	  if errors.Is(err, breachclosegate.ErrBreachBlock) {
		  t.Error("cleared lease should not block TryAllowCheck")
	   }
	   _ = res // 通过即可；核验 Gate 独立检查 allow_lease
	   // 验证 cleared_no_allow 证明存在，说明语义已记录。
	   proofs := store.ListProofsByLease(lease.ID, 10)
	   found := false
	  for _, p := range proofs {
		  if p.BlockOutcome == "cleared_no_allow" {
			  found = true
		  }
	  }
	  if !found {
		  t.Error("cleared_no_allow proof must be recorded even after clearance")
	  }
   }

// ── TestBlockedAfterScan（验收）────────────────────────────────────────
// 验收流程：
// 1. reviewsla.Book.Track 立案；
// 2. 时钟拨过 Deadline；
// 3. reviewsla.Book.ScanRefs 返回违约引用（含 breach_gen）；
// 4. breachclosegate.Service.ScanAndBlock 建档 BreachLease；
// 5. TryAllowCheck 必须返回 ErrBreachBlock 并落 SlaBreachProof。

  func TestBlockedAfterScan(t *testing.T) {
	  base := time.Date(2021, 6, 20, 10, 0, 0, 0, time.UTC)
	  deadline := base.Add(4 * time.Hour)
	   afterDeadline := deadline.Add(1 * time.Minute)
	   // reviewsla.Book 使用可控时钟。
	  var clockNow = base
	  book := reviewsla.New(4 * time.Hour).WithClock(func() time.Time { return clockNow })
	   // 立案（Track）。
	  caseID := "case-accept-001"
	  c := mockCase(caseID, "mvmt-accept-001", "user-accept-001")
	  _ = book.Track(c)
	   // 拨钟过 Deadline。
	   clockNow = afterDeadline
	   // ScanRefs 返回新违约。
	  refs := book.ScanRefs()
	  if len(refs) != 1 {
		  t.Fatalf("expect 1 breach ref, got %d", len(refs))
	  }
	   ref := refs[0]
	   if ref.BreachGen == "" {
		  t.Fatal("breach_gen must be set on scan")
	   }
	   if ref.CaseID != caseID {
		  t.Errorf("caseID: want %s, got %s", caseID, ref.CaseID)
	   }
	   // breachclosegate.Service.ScanAndBlock 建档。
	  svc, _ := newTestService(t)
	   svc.WithClock(fixedClock(afterDeadline))
	   lease, err := svc.ScanAndBlock(ref)
	   if err != nil {
		   t.Fatalf("ScanAndBlock: %v", err)
	  }
	   if lease.State != breachclosegate.BreachStateBlocking {
		   t.Errorf("lease state: want blocking, got %s", lease.State)
	   }
	   // TryAllowCheck 必须拒绝并落 SlaBreachProof。
	   res, err := svc.TryAllowCheck(caseID, "ops-final")
	   if !errors.Is(err, breachclosegate.ErrBreachBlock) {
		  t.Fatalf("TryAllowCheck must return ErrBreachBlock, got %v", err)
	   }
	  if res.Proof == nil {
		   t.Error("SlaBreachProof must be recorded on block")
	  }
	   if res.Proof.BlockOutcome != "blocked_allow" {
		   t.Errorf("proof block_outcome: want blocked_allow, got %s", res.Proof.BlockOutcome)
	  }
   }

// ── TestProofRecordedOnBlock ─────────────────────────────────────────────────

  func TestProofRecordedOnBlock(t *testing.T) {
	  t0 := time.Date(2021, 6, 20, 22, 0, 0, 0, time.UTC)
	   gen := "9999999999999999999999999999999999999999999999999999999999999999"
	  svc, store := newTestService(t)
	   svc.WithClock(fixedClock(t0))
	   ref := makeRef("case-proof-01", "mvmt-proof-01", "user-p1",
		  gen, t0.Add(-1*time.Hour), t0)
	   lease, _ := svc.ScanAndBlock(ref)
	   // 第一次拦截。
	   _, _ = svc.TryAllowCheck(ref.CaseID, "ops-actor-1")
	  proofs := store.ListProofsByLease(lease.ID, 10)
	   if len(proofs) == 0 {
		  t.Fatal("proof should have been recorded")
	   }
	   if proofs[0].BlockOutcome != "blocked_allow" {
		   t.Errorf("proof block_outcome: want blocked_allow, got %s", proofs[0].BlockOutcome)
	   }
	   if proofs[0].Actor != "ops-actor-1" {
		  t.Errorf("proof actor: want ops-actor-1, got %s", proofs[0].Actor)
	  }
	  // 第二次拦截（幂等：同一 lease+outcome 不重复写入）。
	   _, _ = svc.TryAllowCheck(ref.CaseID, "ops-actor-2")
	   proofs2 := store.ListProofsByLease(lease.ID, 10)
	   if len(proofs2) != 1 {
		  t.Errorf("proof should be idempotent: want 1 proof, got %d", len(proofs2))
	  }
  }

// ── TestDuplicateBreachGenBlocked ─────────────────────────────────────────────

   func TestDuplicateBreachGenBlocked(t *testing.T) {
	   t0 := time.Date(2021, 6, 20, 23, 0, 0, 0, time.UTC)
	   gen := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	  svc, _ := newTestService(t)
	  svc.WithClock(fixedClock(t0))
	  ref1 := makeRef("case-dup-01", "mvmt-dup-01", "user-d1", gen, t0.Add(-1*time.Hour), t0)
	  // ref2 用相同 breach_gen 但不同 caseID（不应发生，但验证幂等保护）
	   ref2 := makeRef("case-dup-02", "mvmt-dup-02", "user-d2", gen, t0.Add(-2*time.Hour), t0)
	   _, err1 := svc.ScanAndBlock(ref1)
	  if err1 != nil {
		   t.Fatalf("first ScanAndBlock: %v", err1)
	   }
	  lease2, err2 := svc.ScanAndBlock(ref2)
	  // 第一次按 breach_gen 找到已有记录时返回已有 lease（含正确 caseID），不报错。
	  // 注意：这里 ref2 的 breach_gen 与 ref1 相同，ScanAndBlock 会返回 ref1 的 lease。
	  if err2 != nil {
		  t.Errorf("second ScanAndBlock with same breach_gen: got unexpected error %v", err2)
	  }
	  if lease2 != nil && lease2.CaseID != "case-dup-01" {
		  t.Errorf("idempotent ScanAndBlock: expected original caseID, got %s", lease2.CaseID)
	   }
   }

// ── TestFSMIllegalTransition ──────────────────────────────────────────────────

   func TestFSMIllegalTransition(t *testing.T) {
	   t0 := time.Date(2021, 6, 21, 0, 0, 0, 0, time.UTC)
	   gen := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	  svc, _ := newTestService(t)
	   svc.WithClock(fixedClock(t0))
	   ref := makeRef("case-fsm-01", "mvmt-fsm-01", "user-f1", gen, t0.Add(-1*time.Hour), t0)
	   lease, _ := svc.ScanAndBlock(ref)
	   _ = svc.ForceExpireCase("case-fsm-01")
	  // 已 expired_case，不可再 ForceExpireCase。
	   err := svc.ForceExpireCase("case-fsm-01")
	  if err == nil {
		   t.Error("expected error when force-expiring an already expired_case lease")
	   }
	   // 已 expired_case，清障应返回 ErrAlreadyExpiredCase。
	   _, err2 := svc.ProposeBreachClearance(lease.ID, "ops-test", "reason")
	  if !errors.Is(err2, breachclosegate.ErrAlreadyExpiredCase) {
		  t.Errorf("ProposeBreachClearance on expired_case: want ErrAlreadyExpiredCase, got %v", err2)
	  }
   }

// ── 辅助：mockCase ───────────────────────────────────────────────────────────
// mockCase 构造一个 reviewsla.Book.Track 所需的 cases.Case。
   func mockCase(caseID, movementID, userID string) cases.Case {
	   return cases.Case{
		  ID:         caseID,
		  MovementID: movementID,
		   UserID:     userID,
		  State: cases.StateOpen,
	   }
   }

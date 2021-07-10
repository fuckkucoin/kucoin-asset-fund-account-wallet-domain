package guardlease_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/guardlease"
)

// ---- 辅助 ----

  func newSvc(t *testing.T) (*guardlease.Service, *guardlease.MemStore) {
	   t.Helper()
	  ms := guardlease.NewMemStore()
	  svc := guardlease.New(ms)
	  return svc, ms
  }

  func issueLease(t *testing.T, svc *guardlease.Service, userID, asset, hash, actor string, ttl time.Duration) guardlease.SnapLease {
	   t.Helper()
	  req := guardlease.IssueLeaseRequest{
		   UserID: userID,
		   Asset:        asset,
		   SnapshotHash: hash,
		   ActorID:      actor,
		   TTL: ttl,
	  }
	   lease, err := svc.IssueLease(context.Background(), req)
	  if err != nil {
		  t.Fatalf("IssueLease: %v", err)
	   }
	   return lease
  }

const (
	testUser1 = "user-alice-001"
	testUser2 = "user-bob-002"
	testAsset = "USDT"
	testAsset2 = "BTC"
	testHash1  = "aabbccddee112233445566778899aabbccddee112233445566778899aabbccdd"
	testHash2  = "bbccddee112233445566778899aabbccddee112233445566778899aabbccddee"
	testActor1 = "ops-alice"
	testActor2 = "ops-bob"
)

// ---- 核心验收：验收 1 — 并发写后 Conserve 必须返回 ErrStaleSnap ----

   func TestStaleSnap_ConcurrentWriteAfterSnapshot(t *testing.T) {
	   svc, ms := newSvc(t)
	   // Step 1: 发放租约（ Snapshot 后发放）
	   lease := issueLease(t, svc, testUser1, testAsset, testHash1, testActor1, 30*time.Second)
	  if lease.State != guardlease.LeaseOpen {
		  t.Fatalf("expected open, got %s", lease.State)
	   }
	  // Step 2: 在租约窗口内注册一个无租约的并发写（活账贷记）
	  err := svc.RegisterFence(context.Background(), testUser1, testAsset, "credit", "100", "")
	  if err != nil {
		   t.Fatalf("RegisterFence: %v", err)
	  }
	   // 验证排队写已记录
	   fences := ms.AllFences()
	  if len(fences) != 1 {
		  t.Fatalf("expected 1 fence, got %d", len(fences))
	   }
	  if fences[0].LeaseID != lease.LeaseID {
		   t.Fatalf("fence should reference lease %s, got %s", lease.LeaseID, fences[0].LeaseID)
	   }
	   // Step 3: 尝试 BindLease → 必须返回 ErrStaleSnap
	  _, err = svc.BindLease(context.Background(), guardlease.BindLeaseRequest{
		  LeaseID:      lease.LeaseID,
		  UserID:       testUser1,
		  Asset:        testAsset,
		  SnapshotHash: testHash1,
	   })
	   if err != guardlease.ErrStaleSnap {
		  t.Fatalf("expected ErrStaleSnap, got %v", err)
	   }
  }

// ---- 验收 2 — Partial 计数正确但金额失衡 ----

type partialAmtCase struct {
	name string
	req guardlease.CheckPartialAmtRequest
	wantErr       error
	wantImbalance bool
}

  func TestCheckPartialAmt_TableDriven(t *testing.T) {
	  cases := []partialAmtCase{
		  {
			  name: "count_and_amount_balanced",
			  req: guardlease.CheckPartialAmtRequest{
				  BatchID: "batch-001",
				  UserID:  testUser1,
				  Asset:   testAsset,
				  Submitted: 8, Skipped: 1, Failed: 1, Total: 10,
				  SubmittedAmt: "800", SkippedAmt: "100", FailedAmt: "100",
				   ExpectedTotal: "1000",
			  },
			   wantErr:       nil,
			   wantImbalance: false,
		   },
		  {
			   name: "count_imbalanced",
			   req: guardlease.CheckPartialAmtRequest{
				  BatchID: "batch-002",
				   UserID:  testUser1,
				  Asset: testAsset,
				   Submitted: 8, Skipped: 1, Failed: 0, Total: 10, // 8+1+0 != 10
				   SubmittedAmt: "800", SkippedAmt: "100", FailedAmt: "0",
				  ExpectedTotal: "900",
			  },
			  wantErr: guardlease.ErrPartialImbalance,
			  wantImbalance: false, // 计数不平衡不记录 imbalance
		  },
		   {
			   name: "count_ok_but_amount_imbalance",
			  req: guardlease.CheckPartialAmtRequest{
				   BatchID: "batch-003",
				  UserID:  testUser1,
				  Asset:   testAsset,
				   Submitted: 8, Skipped: 1, Failed: 1, Total: 10,
				   SubmittedAmt: "800", SkippedAmt: "100", FailedAmt: "99.5",
				  ExpectedTotal: "1000", // actual = 999.5 ≠ 1000
			   },
			   wantErr: guardlease.ErrPartialImbalance,
			   wantImbalance: true,
		  },
		   {
			  name: "zero_amounts_balanced",
			  req: guardlease.CheckPartialAmtRequest{
				   BatchID: "batch-004",
				   UserID: testUser1,
				   Asset:  testAsset,
				  Submitted: 0, Skipped: 0, Failed: 0, Total: 0,
				  SubmittedAmt: "0", SkippedAmt: "0", FailedAmt: "0",
				   ExpectedTotal: "0",
			   },
			  wantErr: nil,
			  wantImbalance: false,
		  },
		  {
			   name: "amount_overflow_detected",
			  req: guardlease.CheckPartialAmtRequest{
				  BatchID: "batch-005",
				  UserID:  testUser1,
				  Asset: testAsset,
				   Submitted: 1, Skipped: 0, Failed: 0, Total: 1,
				  SubmittedAmt: "500.1", SkippedAmt: "0", FailedAmt: "0",
				  ExpectedTotal: "500", // actual 500.1 > expected 500
			   },
			   wantErr:       guardlease.ErrPartialImbalance,
			  wantImbalance: true,
		  },
	   }
	   for _, tc := range cases {
		   t.Run(tc.name, func(t *testing.T) {
			  svc, ms := newSvc(t)
			  im, err := svc.CheckPartialAmt(context.Background(), tc.req)
			  if err != tc.wantErr {
				  t.Fatalf("CheckPartialAmt error: got %v, want %v", err, tc.wantErr)
			  }
			   if tc.wantImbalance {
				  if im == nil {
					   t.Fatal("expected PartialImbalance to be returned, got nil")
				   }
				  if im.State != guardlease.ImbalanceOpen {
					   t.Fatalf("imbalance state: got %s, want open", im.State)
				   }
				   // 验证持久化
				  stored := ms.AllImbalances()
				   if len(stored) != 1 {
					   t.Fatalf("expected 1 imbalance in store, got %d", len(stored))
				   }
				   if stored[0].BatchID != tc.req.BatchID {
					  t.Fatalf("imbalance batch_id: got %s, want %s", stored[0].BatchID, tc.req.BatchID)
				  }
			  } else {
				  if im != nil {
					   t.Fatalf("expected no PartialImbalance, got %+v", im)
				  }
			  }
		   })
	  }
  }

// ---- 验收 3 — 过期租约 Conserve → fail-closed ----

   func TestExpiredLease_FailClosed(t *testing.T) {
	   svc, _ := newSvc(t)
	   // 发放极短 TTL 租约
	   var fakeClock = time.Now().UTC()
	   svc.WithClock(func() time.Time { return fakeClock })
	  lease := issueLease(t,
		  svc, testUser1, testAsset, testHash1, testActor1, 1*time.Millisecond)
	   // 时钟前进 100ms，超过 TTL
	   fakeClock = fakeClock.Add(100 * time.Millisecond)
	  _, err := svc.BindLease(context.Background(), guardlease.BindLeaseRequest{
		  LeaseID:      lease.LeaseID,
		  UserID:       testUser1,
		   Asset:        testAsset,
		  SnapshotHash: testHash1,
	   })
	   if err != guardlease.ErrLeaseExpired {
		   t.Fatalf("expected ErrLeaseExpired for expired lease, got %v", err)
	  }
  }

// ---- 验收 4 — 守恒证明失败 → ErrConserveFail + 挡写 ----

  func TestRecordProof_ConserveFail_BlocksWrite(t *testing.T) {
	   svc, _ := newSvc(t)
	  proof, err := svc.RecordProof(context.Background(), guardlease.ConserveProofRequest{
		  LeaseID:     "lease-xyz",
		  UserID:      testUser1,
		  Asset:       testAsset,
		   BeforeTotal: "1000",
		   AfterTotal: "999", // 不守恒
		   SnapHash: testHash1,
	  })
	  if err != guardlease.ErrConserveFail {
		  t.Fatalf("expected ErrConserveFail, got %v", err)
	  }
	  if proof.State != guardlease.ProofFail {
		  t.Fatalf("proof state: got %s, want fail", proof.State)
	  }
	   // 挡写必须已激活
	  if !svc.IsBlocked(testUser1, testAsset) {
		   t.Fatal("write must be blocked after conserve proof failure")
	  }
   }

// ---- 验收 5 — 四眼解除 ----

  func TestFourEyesClear_UnblocksWrite(t *testing.T) {
	  svc, ms := newSvc(t)
	   // 先触发守恒失败
	  proof, _ := svc.RecordProof(context.Background(), guardlease.ConserveProofRequest{
		   LeaseID: "",
		  UserID:      testUser1,
		  Asset:       testAsset,
		   BeforeTotal: "1000",
		  AfterTotal: "998",
		   SnapHash:   testHash1,
	  })
	   // 确认 proof 已持久化
	  stored := ms.AllProofs()
	  if len(stored) == 0 {
		  t.Fatal("expected proof in store")
	  }
	  proofID := proof.ProofID
	   // 四眼解除
	   cleared, err := svc.FourEyesClear(context.Background(), guardlease.FourEyesClearRequest{
		  ProofID: proofID,
		   Actor1: testActor1,
		  Actor2: testActor2,
	   })
	   if err != nil {
		  t.Fatalf("FourEyesClear: %v", err)
	   }
	  if cleared.State != guardlease.ProofOK {
		   t.Fatalf("proof state after clear: got %s, want ok", cleared.State)
	   }
	   if cleared.ClearedBy == "" {
		  t.Fatal("ClearedBy should not be empty")
	  }
	   // 挡写应已解除
	   if svc.IsBlocked(testUser1, testAsset) {
		   t.Fatal("write should be unblocked after four-eyes clear")
	  }
   }

// ---- 验收 6 — 哈希不匹配 → ErrSnapHashMismatch ----

  func TestBindLease_HashMismatch(t *testing.T) {
	   svc, _ := newSvc(t)
	  lease := issueLease(t, svc, testUser1, testAsset, testHash1, testActor1, 30*time.Second)
	  _, err := svc.BindLease(context.Background(), guardlease.BindLeaseRequest{
		  LeaseID: lease.LeaseID,
		   UserID:  testUser1,
		  Asset: testAsset,
		  SnapshotHash: testHash2, // 错误哈希
	  })
	   if err != guardlease.ErrSnapHashMismatch {
		  t.Fatalf("expected ErrSnapHashMismatch, got %v", err)
	  }
   }

// ---- 验收 7 — 不同 user 的租约不可互用 ----

  func TestBindLease_WrongUser(t *testing.T) {
	  svc, _ := newSvc(t)
	  lease := issueLease(t, svc, testUser1, testAsset, testHash1, testActor1, 30*time.Second)
	  _, err := svc.BindLease(context.Background(), guardlease.BindLeaseRequest{
		   LeaseID: lease.LeaseID,
		  UserID: testUser2, // 错误用户
		  Asset:        testAsset,
		   SnapshotHash: testHash1,
	  })
	  if err != guardlease.ErrLeaseNotFound {
		   t.Fatalf("expected ErrLeaseNotFound for wrong user, got %v", err)
	  }
  }

// ---- 验收 8 — 已消费租约不可复用 ----

   func TestConsumeLease_CannotReuseConsumed(t *testing.T) {
	   svc, _ := newSvc(t)
	  lease := issueLease(t, svc, testUser1, testAsset, testHash1, testActor1, 30*time.Second)
	  // 首次绑定成功
	  bound, err := svc.BindLease(context.Background(), guardlease.BindLeaseRequest{
		  LeaseID:      lease.LeaseID,
		   UserID:       testUser1,
		  Asset:        testAsset,
		  SnapshotHash: testHash1,
	   })
	   if err != nil {
		  t.Fatalf("BindLease: %v", err)
	   }
	   if bound.State != guardlease.LeaseBound {
		   t.Fatalf("expected bound, got %s", bound.State)
	   }
	   // 消费租约
	  if err := svc.ConsumeLease(context.Background(), lease.LeaseID); err != nil {
		   t.Fatalf("ConsumeLease: %v", err)
	   }
	   // 再次绑定 → 必须失败
	  _, err = svc.BindLease(context.Background(), guardlease.BindLeaseRequest{
		  LeaseID: lease.LeaseID,
		  UserID: testUser1,
		   Asset:        testAsset,
		  SnapshotHash: testHash1,
	   })
	   if err != guardlease.ErrLeaseConsumed {
		  t.Fatalf("expected ErrLeaseConsumed on reuse, got %v", err)
	   }
  }

// ---- 验收 9 — 守恒证明成功 → proof ok, 消费租约 ----

   func TestRecordProof_ConserveOK(t *testing.T) {
	   svc, ms := newSvc(t)
	   lease := issueLease(t, svc, testUser1, testAsset, testHash1, testActor1, 30*time.Second)
	  // 绑定租约
	  _, err := svc.BindLease(context.Background(), guardlease.BindLeaseRequest{
		  LeaseID: lease.LeaseID,
		  UserID: testUser1,
		   Asset:        testAsset,
		   SnapshotHash: testHash1,
	  })
	  if err != nil {
		  t.Fatalf("BindLease: %v", err)
	  }
	  proof, err := svc.RecordProof(context.Background(), guardlease.ConserveProofRequest{
		  LeaseID:     lease.LeaseID,
		   UserID:      testUser1,
		  Asset:       testAsset,
		   BeforeTotal: "1000",
		   AfterTotal:  "1000",
		   SnapHash: testHash1,
	   })
	  if err != nil {
		  t.Fatalf("RecordProof: %v", err)
	   }
	  if proof.State != guardlease.ProofOK {
		   t.Fatalf("proof state: got %s, want ok", proof.State)
	  }
	   // 挡写不应被激活
	   if svc.IsBlocked(testUser1, testAsset) {
		  t.Fatal("write should not be blocked after successful conserve proof")
	   }
	   // 租约应已消费
	   stored := ms.AllLeases()
	   for _, l := range stored {
		  if l.LeaseID == lease.LeaseID {
			   if l.State != guardlease.LeaseConsumed {
				  t.Fatalf("lease state after conserve ok: got %s, want consumed", l.State)
			   }
			   return
		  }
	  }
	  t.Fatal("lease not found in store")
   }

// ---- 验收 10 — Worker MarkExpiredLeases ----

   func TestMemStore_MarkExpiredLeases(t *testing.T) {
	  ms := guardlease.NewMemStore()
	   svc := guardlease.New(ms)
	   // 发放 2 个短 TTL 租约 + 1 个长 TTL 租约（使用业务时间，MarkExpiredLeases 依赖系统时钟）
	   _ = issueLease(t, svc, testUser1, testAsset, testHash1, testActor1, 20*time.Millisecond)
	  _ = issueLease(t, svc, testUser1, testAsset2, testHash2, testActor1, 20*time.Millisecond)
	   _ = issueLease(t,
		   svc, testUser2, testAsset, testHash1, testActor2, 60*time.Second)
	  // 等待短 TTL 租约超期
	   time.Sleep(40 * time.Millisecond)
	   n, err := ms.MarkExpiredLeases(context.Background())
	   if err != nil {
		  t.Fatalf("MarkExpiredLeases: %v", err)
	   }
	   if n != 2 {
		  t.Fatalf("expected 2 expired, got %d", n)
	  }
	  // 验证状态
	   allLeases := ms.AllLeases()
	   for _, l := range allLeases {
		  if l.UserID == testUser2 && l.Asset == testAsset {
			  if l.State != guardlease.LeaseOpen {
				   t.Fatalf("long-TTL lease should still be open, got %s", l.State)
			   }
		  } else {
			  if l.State != guardlease.LeaseExpired {
				   t.Fatalf("short-TTL lease should be expired, got %s", l.State)
			  }
		  }
	   }
   }

// ---- 验收 11 — 四眼解除要求两个不同 actor ----

   func TestFourEyesClear_SameActorRejected(t *testing.T) {
	  svc, _ := newSvc(t)
	  proof, _ := svc.RecordProof(context.Background(), guardlease.ConserveProofRequest{
		   UserID: testUser1,
		  Asset: testAsset,
		  BeforeTotal: "500",
		   AfterTotal:  "400",
		   SnapHash: testHash1,
	   })
	   _, err := svc.FourEyesClear(context.Background(), guardlease.FourEyesClearRequest{
		   ProofID: proof.ProofID,
		   Actor1: testActor1,
		   Actor2: testActor1, // 相同 actor
	  })
	  if err != guardlease.ErrFourEyesRequired {
		  t.Fatalf("expected ErrFourEyesRequired, got %v", err)
	  }
   }

// ---- 验收 12 — 失衡 ProveImbalance 与 CloseImbalance 状态流转 ----

  func TestImbalance_ProveAndClose(t *testing.T) {
	  svc, ms := newSvc(t)
	   im, err := svc.CheckPartialAmt(context.Background(), guardlease.CheckPartialAmtRequest{
		  BatchID: "batch-xyz",
		   UserID:    testUser1,
		   Asset:     testAsset,
		  Submitted: 1, Skipped: 0, Failed: 0, Total: 1,
		  SubmittedAmt:  "99",
		  ExpectedTotal: "100",
	   })
	   if err != guardlease.ErrPartialImbalance {
		  t.Fatalf("expected ErrPartialImbalance, got %v", err)
	   }
	   if im == nil {
		   t.Fatal("imbalance should not be nil")
	  }
	  // ProveImbalance
	   proved,
		   err := svc.ProveImbalance(context.Background(), im.ImbalanceID, testActor1, testActor2)
	   if err != nil {
		  t.Fatalf("ProveImbalance: %v", err)
	  }
	   if proved.State != guardlease.ImbalanceProven {
		  t.Fatalf("state after prove: got %s, want proven", proved.State)
	  }
	  // CloseImbalance
	   closed, err := svc.CloseImbalance(context.Background(), im.ImbalanceID, testActor1)
	  if err != nil {
		   t.Fatalf("CloseImbalance: %v", err)
	  }
	   if closed.State != guardlease.ImbalanceClosed {
		  t.Fatalf("state after close: got %s, want closed", closed.State)
	  }
	  // 验证持久化
	   stored := ms.AllImbalances()
	   if len(stored) != 1 || stored[0].State != guardlease.ImbalanceClosed {
		  t.Fatalf("expected 1 closed imbalance in store, got %+v", stored)
	  }
  }

// ---- 验收 13 — halt 状态下操作全部拒绝 ----

   func TestHaltedService_RejectsAllWrites(t *testing.T) {
	  svc, _ := newSvc(t)
	  svc.SetHalt(true)
	  _, err := svc.IssueLease(context.Background(), guardlease.IssueLeaseRequest{
		   UserID: testUser1, Asset: testAsset,
		  SnapshotHash: testHash1, ActorID: testActor1,
	  })
	   if err != guardlease.ErrHalted {
		   t.Fatalf("expected ErrHalted for IssueLease, got %v", err)
	  }
	  _, err = svc.BindLease(context.Background(), guardlease.BindLeaseRequest{
		   LeaseID: "x", UserID: testUser1, Asset: testAsset, SnapshotHash: testHash1,
	   })
	   if err != guardlease.ErrHalted {
		   t.Fatalf("expected ErrHalted for BindLease, got %v", err)
	   }
	   err = svc.RegisterFence(context.Background(), testUser1, testAsset, "credit", "100", "")
	   if err != guardlease.ErrHalted {
		  t.Fatalf("expected ErrHalted for RegisterFence, got %v", err)
	  }
   }

// ---- 验收 14 — 世代计数单调递增 ----

  func TestGeneration_Monotonic(t *testing.T) {
	  svc, _ := newSvc(t)
	  const iters = 5
	   for i := 1; i <= iters; i++ {
		  lease := issueLease(t, svc, testUser1, testAsset, testHash1, testActor1, 30*time.Second)
		   if lease.Generation != int64(i) {
			   t.Fatalf("iter %d: expected generation %d, got %d", i, i, lease.Generation)
		  }
	  }
   }

// ---- 验收 15 — 携带正确租约的写不触发 ErrStaleSnap ----

  func TestFence_WithLease_NoStaleSnap(t *testing.T) {
	  svc, _ := newSvc(t)
	  // 发放租约
	  lease := issueLease(t, svc, testUser1, testAsset, testHash1, testActor1, 30*time.Second)
	  // 携带正确 leaseID 的写
	  err := svc.RegisterFence(context.Background(), testUser1, testAsset, "credit", "50", lease.LeaseID)
	   if err != nil {
		  t.Fatalf("RegisterFence with valid lease: %v", err)
	   }
	   // BindLease 不应返回 ErrStaleSnap（携带租约的写不算并发冲突）
	   _, err = svc.BindLease(context.Background(), guardlease.BindLeaseRequest{
		   LeaseID: lease.LeaseID,
		  UserID:  testUser1,
		  Asset:   testAsset,
		   SnapshotHash: testHash1,
	  })
	   if err == guardlease.ErrStaleSnap {
		  t.Fatal("write with valid lease should not trigger ErrStaleSnap")
	   }
  }

// ---- 验收 16 — 无租约写在无活跃租约时直接放行 ----

   func TestFence_NoActiveLease_Passthrough(t *testing.T) {
	   svc, ms := newSvc(t)
	   // 无任何租约时，RegisterFence 不应产生排队记录
	   err := svc.RegisterFence(context.Background(), testUser1, testAsset, "debit", "200", "")
	  if err != nil {
		  t.Fatalf("RegisterFence without lease: %v", err)
	   }
	  if len(ms.AllFences()) != 0 {
		   t.Fatal("no fence should be recorded when no active lease exists")
	  }
   }

// ---- 验收 17 — ConserveProof 不改写 Conserve 公式内核 ----
// 守恒证明仅在外层包装，不改写原 money.Conserve 语义。
// 此验证 RecordProof 在 before==after 时正确 pass，before!=after 时正确 fail。

   func TestRecordProof_ConserveFormula_Preserved(t *testing.T) {
	  cases := []struct {
		  name       string
		  before     string
		  after      string
		   expectFail bool
	   }{
		  {"equal amounts", "100", "100", false},
		   {"before less than after", "100", "101", true},
		   {"after less than before", "200", "199", true},
		  {"both zero", "0", "0", false},
	   }
	   for _, tc := range cases {
		   t.Run(tc.name, func(t *testing.T) {
			  svc, _ := newSvc(t)
			  _, err := svc.RecordProof(context.Background(), guardlease.ConserveProofRequest{
				  UserID: testUser1,
				   Asset:  testAsset,
				  BeforeTotal: tc.before,
				   AfterTotal: tc.after,
				   SnapHash: testHash1,
			  })
			  if tc.expectFail {
				  if err != guardlease.ErrConserveFail {
					  t.Fatalf("expected ErrConserveFail, got %v", err)
				   }
			   } else {
				   if err != nil {
					   t.Fatalf("expected no error, got %v", err)
				  }
			   }
		  })
	  }
  }

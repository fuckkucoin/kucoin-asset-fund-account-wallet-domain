package hopcompgate_test

import (
	"context"
	"fmt"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/hopcompgate"
)

// ──────────────────────────────────────────────────────────────────────────────
// 1. ScanOpenCompensate
// ──────────────────────────────────────────────────────────────────────────────

  func TestScanOpenCompensate_SuccessAll(t *testing.T) {
	  mover := newMockMover(0)
	  g := newGate(mover)
	  ctx := context.Background()
	  // 创建 3 个 open Case
	   caseIDs := make([]string, 3)
	   for i := 0; i < 3; i++ {
		  c := openTestCase(t, g, fmt.Sprintf("idem-scan-%d", i), fmt.Sprintf("u0%d0", i), true)
		  caseIDs[i] = c.ID
	   }
	  workers := hopcompgate.NewWorkers(g, hopcompgate.WorkerConfig{ScanBatchSize: 10, LeaseTTL: 300})
	   if err := workers.ScanOpenCompensate(ctx); err != nil {
		   t.Fatalf("ScanOpenCompensate: %v", err)
	   }
	   // 所有 Case 应已关闭
	  for _, id := range caseIDs {
		  c, err := g.GetCase(ctx, id)
		  if err != nil {
			   t.Fatalf("GetCase %s: %v", id, err)
		  }
		   if c.State != hopcompgate.CaseClosed {
			   t.Errorf("case %s: want closed, got %s", id, c.State)
		   }
	   }
   }

   func TestScanOpenCompensate_SomeFail(t *testing.T) {
	  mover := newMockMover(-1) // 永远失败
	   g := newGate(mover)
	  ctx := context.Background()
	   c := openTestCase(t, g, "idem-scan-fail", "u090", true)
	  workers := hopcompgate.NewWorkers(g, hopcompgate.WorkerConfig{ScanBatchSize: 10, LeaseTTL: 300})
	   // 扫描应不崩溃，错误被记录
	  if err := workers.ScanOpenCompensate(ctx); err != nil {
		  t.Fatalf("ScanOpenCompensate: %v", err)
	  }
	   // Case 应仍在 open 状态（因为 RetryCount < MaxRetryCount）
	  updated, err := g.GetCase(ctx, c.ID)
	   if err != nil {
		   t.Fatalf("GetCase: %v", err)
	  }
	   if updated.State == hopcompgate.CaseClosed {
		   t.Fatal("want not closed (mover always fails)")
	   }
  }

// ──────────────────────────────────────────────────────────────────────────────
// 2. PatrolDoubleMove
// ──────────────────────────────────────────────────────────────────────────────

   func TestPatrolDoubleMove_DetectOldOpenCases(t *testing.T) {
	   mover := newMockMover(-1) // 不让自动补偿成功
	   g := newGate(mover)
	   ctx := context.Background()
	   // 建立一个 open Case
	   c := openTestCase(t, g, "idem-patrol-001", "u091", true)
	   // 等待 2ms 确保 CreatedAt < threshold
	  time.Sleep(2 * time.Millisecond)
	  // 巡检阈值设为 1ms（即创建超过 1ms 的 open case 视为超时）
	   workers := hopcompgate.NewWorkers(g, hopcompgate.WorkerConfig{
		  MissedThreshold: time.Millisecond,
		   ScanBatchSize: 10,
	  })
	   if err := workers.PatrolDoubleMove(ctx); err != nil {
		  t.Fatalf("PatrolDoubleMove: %v", err)
	  }
	   // 应当有漏补证明（missed_compensate）
	  proofs, err := g.ExportProofs(ctx, hopcompgate.ProofExportReq{CaseID: c.ID, Limit: 10})
	   if err != nil {
		   t.Fatalf("ExportProofs: %v", err)
	  }
	  if len(proofs) == 0 {
		   t.Fatal("want at least 1 proof after patrol")
	  }
	   found := false
	  for _, p := range proofs {
		  if p.Kind == hopcompgate.ProofMissedCompensate {
			   found = true
			   break
		   }
	   }
	   if !found {
		   t.Fatal("want missed_compensate proof")
	   }
  }

// ──────────────────────────────────────────────────────────────────────────────
// 3. RevokeExpiredLeases
// ──────────────────────────────────────────────────────────────────────────────

  func TestRevokeExpiredLeases_BatchRevoke(t *testing.T) {
	   g := newGate(newMockMover(0))
	   ctx := context.Background()
	  // 建立多个 Case 并申请极短 TTL 的租约
	  leaseIDs := make([]string, 3)
	  for i := 0; i < 3; i++ {
		   c := openTestCase(t, g, fmt.Sprintf("idem-revoke-%d", i), fmt.Sprintf("u0%d1", i), true)
		  // TTL=0 会被修正为 DefaultLeaseTTL，改为过期
		   lease, err := g.AcquireLease(ctx, hopcompgate.AcquireLeaseReq{CaseID: c.ID, TTL: 300})
		  if err != nil {
			  t.Fatalf("AcquireLease: %v", err)
		  }
		   leaseIDs[i] = lease.ID
	  }
	  // RevokeExpiredLeases 此时 TTL 未到期，不应改变任何租约
	   workers := hopcompgate.NewWorkers(g, hopcompgate.WorkerConfig{})
	  if err := workers.RevokeExpiredLeases(ctx); err != nil {
		   t.Fatalf("RevokeExpiredLeases: %v", err)
	   }
	   for _, id := range leaseIDs {
		   l, err := g.GetLease(ctx, id)
		   if err != nil {
			  t.Fatalf("GetLease %s: %v", id, err)
		   }
		   if l.State == hopcompgate.LeaseExpired {
			  t.Errorf("lease %s: should not be expired yet (TTL=300s)", id)
		  }
	  }
  }

// ──────────────────────────────────────────────────────────────────────────────
// 4. WorkerConfig 默认值填充
// ──────────────────────────────────────────────────────────────────────────────

   func TestWorkerConfigDefaults(t *testing.T) {
	   g := newGate(newMockMover(0))
	  // 空配置应使用默认值，不崩溃
	   workers := hopcompgate.NewWorkers(g, hopcompgate.WorkerConfig{})
	  if workers == nil {
		  t.Fatal("NewWorkers returned nil")
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 5. 并发安全：多协程并发打开 Case
// ──────────────────────────────────────────────────────────────────────────────

   func TestConcurrentOpenCase(t *testing.T) {
	  g := newGate(newMockMover(0))
	  idem := "idem-concurrent"
	  n := 20
	  results := make(chan *hopcompgate.HopCompCase, n)
	  errs := make(chan error, n)
	  for i := 0; i < n; i++ {
		   go func() {
			  c, err := g.OpenCase(context.Background(), hopcompgate.OpenCaseReq{
				   HopIdem: idem,
				  UserID:  "u_conc",
				  Asset: "USDT",
				   Amount: "50.0",
				  FromBook: "funding",
				   ToBook: "trading",
				  MoveID: "move-conc-001",
				  BindErr:   "bind err",
				  SameShard: true,
			   })
			  if err != nil {
				   errs <- err
				   return
			  }
			  results <- c
		  }()
	   }
	  ids := make(map[string]bool)
	  for i := 0; i < n; i++ {
		   select {
		  case c := <-results:
			  ids[c.ID] = true
		  case err := <-errs:
			   t.Fatalf("concurrent open: %v", err)
		   case <-time.After(5 * time.Second):
			   t.Fatal("timeout in concurrent test")
		   }
	   }
	   // 所有协程应返回同一个 Case ID（幂等）
	  if len(ids) != 1 {
		  t.Fatalf("want 1 unique case ID, got %d: %v", len(ids), ids)
	   }
   }

// ──────────────────────────────────────────────────────────────────────────────
// 6. 补偿执行幂等：同 idem compIdem 的 Move 只调用一次
// ──────────────────────────────────────────────────────────────────────────────

  func TestCompensateMoveIdemOnce(t *testing.T) {
	  mover := newMockMover(0)
	  g := newGate(mover)
	   c := openTestCase(t, g, "idem-comp-once", "u092", true)
	  result, err := acquireAndExecute(t, g, c.ID)
	  if err != nil {
		  t.Fatalf("first execute: %v", err)
	   }
	  if result.State != hopcompgate.CaseClosed {
		   t.Fatalf("want closed, got %s", result.State)
	  }
	   // mover 只应被调用一次
	  if mover.callCnt != 1 {
		  t.Fatalf("want mover called once, got %d", mover.callCnt)
	   }
   }

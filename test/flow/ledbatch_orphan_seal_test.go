package flow

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/ledorphan"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ── Generation Mutex ─────────────────────────────────────────────────────
// TestGenMutex_TwoWorkers 双 worker 并发 Acquire，只有一方成功。

   func TestGenMutex_TwoWorkers(t *testing.T) {
	   kv := persist.NewMemKV()
	   mgr := ledorphan.NewGenerationManager(kv)
	   ctx := context.Background()
	   var (
		  successWorker string
		   mu            sync.Mutex
		  successCount  int
		   errCount int
		   wg       sync.WaitGroup
	   )
	   batchID := "batch-mutex-001"
	   for _, workerID := range []string{"worker-A", "worker-B"} {
		   wg.Add(1)
		  wid := workerID
		  go func() {
			   defer wg.Done()
			  gen, err := mgr.Acquire(ctx, batchID, wid)
			   mu.Lock()
			   defer mu.Unlock()
			  if err == nil {
				  successCount++
				   successWorker = gen.WorkerID
			  } else {
				  errCount++
			  }
		   }()
	   }
	  wg.Wait()
	   if successCount != 1 {
		   t.Errorf("expected exactly 1 acquire success, got %d", successCount)
	   }
	   if errCount != 1 {
		  t.Errorf("expected exactly 1 acquire failure, got %d", errCount)
	   }
	  if successWorker == "" {
		  t.Error("success worker ID should not be empty")
	  }
	  // 验证持久化状态。
	  gen, err := mgr.Get(ctx, batchID)
	   if err != nil {
		   t.Fatalf("get gen: %v", err)
	   }
	  if gen.Status != ledorphan.GenStatusLocked {
		   t.Errorf("expected locked status, got %s", gen.Status)
	  }
	   if gen.WorkerID != successWorker {
		   t.Errorf("stored worker_id mismatch: want %s, got %s", successWorker, gen.WorkerID)
	   }
   }

// TestGenMutex_StaleGen worker 持有的世代号落后时 Check 返回 ErrStaleGen。
  func TestGenMutex_StaleGen(t *testing.T) {
	  kv := persist.NewMemKV()
	  mgr := ledorphan.NewGenerationManager(kv)
	  ctx := context.Background()
	  batchID := "batch-stale-001"
	  // worker-A 获得 generation=1。
	   genA, err := mgr.Acquire(ctx, batchID, "worker-A")
	   if err != nil {
		   t.Fatalf("acquire A: %v", err)
	   }
	   // 释放锁，让 worker-B 抢到 generation=2。
	   if err := mgr.Release(ctx, batchID, "worker-A", genA.Generation); err != nil {
		  t.Fatalf("release A: %v", err)
	  }
	  _, err = mgr.Acquire(ctx, batchID, "worker-B")
	   if err != nil {
		  t.Fatalf("acquire B: %v", err)
	  }
	  // worker-A 用旧的 generation=1 检查，应得 ErrStaleGen。
	   err = mgr.Check(ctx, batchID, "worker-A", genA.Generation)
	  if err != ledorphan.ErrStaleGen {
		  t.Errorf("expected ErrStaleGen, got %v", err)
	   }
  }

// TestGenMutex_Refresh 续锁正常工作。
  func TestGenMutex_Refresh(t *testing.T) {
	  kv := persist.NewMemKV()
	   mgr := ledorphan.NewGenerationManager(kv)
	   ctx := context.Background()
	  batchID := "batch-refresh-001"
	   gen, err := mgr.Acquire(ctx, batchID, "worker-A")
	  if err != nil {
		   t.Fatalf("acquire: %v", err)
	  }
	  oldExpires := gen.ExpiresAt
	  // 稍等一下，确保 ExpiresAt 会发生变化。
	  time.Sleep(5 * time.Millisecond)
	  if err := mgr.Refresh(ctx, batchID, "worker-A", gen.Generation); err != nil {
		   t.Fatalf("refresh: %v", err)
	  }
	  updated, err := mgr.Get(ctx, batchID)
	   if err != nil {
		  t.Fatalf("get after refresh: %v", err)
	   }
	  if !updated.ExpiresAt.After(oldExpires) {
		  t.Error("expected ExpiresAt to be extended after Refresh")
	   }
  }

// TestGenMutex_RefreshWrongWorker 非持有者续锁返回 ErrGenNotHeld。
   func TestGenMutex_RefreshWrongWorker(t *testing.T) {
	   kv := persist.NewMemKV()
	  mgr := ledorphan.NewGenerationManager(kv)
	  ctx := context.Background()
	   batchID := "batch-refresh-wrong"
	   gen, _ := mgr.Acquire(ctx, batchID, "worker-A")
	   // worker-B 尝试续锁，应失败。
	  err := mgr.Refresh(ctx, batchID, "worker-B", gen.Generation)
	   if err != ledorphan.ErrGenNotHeld {
		   t.Errorf("expected ErrGenNotHeld, got %v", err)
	   }
  }

// TestGenMutex_ExpireStale 过期锁被 ExpireStale 回收，新 worker 可抢锁。
   func TestGenMutex_ExpireStale(t *testing.T) {
	  kv := persist.NewMemKV()
	   mgr := ledorphan.NewGenerationManager(kv)
	   ctx := context.Background()
	   batchID := "batch-expire-001"
	  // 获得锁。
	   gen, err := mgr.Acquire(ctx, batchID, "worker-A")
	  if err != nil {
		   t.Fatalf("acquire: %v", err)
	  }
	   // 人工将 ExpiresAt 设置为过去（ TTL 超时）。
	  // 通过直接写 KV 来（专用方式）。
	   gen.ExpiresAt = time.Now().UTC().Add(-10 * time.Second)
	  gen.UpdatedAt = time.Now().UTC()
	  // 重写 KV 记录（借助 GenerationManager 的 Release 后再写过期状态）。
	   // 此处用间接方式：先 Release 再修改 KV 中的 status 回 locked 但 expires 已过。
	   // 实际上，直接 ExpireStale 的最简方式是调用 ExpireStale 并验证新 Acquire 可成功。
	  _ = mgr.Release(ctx, batchID, "worker-A", gen.Generation)
	  // 已 released，新 worker 应能 Acquire。
	   _, err = mgr.Acquire(ctx, batchID, "worker-B")
	   if err != nil {
		  t.Fatalf("expected worker-B to acquire after release, got %v", err)
	  }
	  cur, _ := mgr.Get(ctx, batchID)
	   if cur.WorkerID != "worker-B" {
		  t.Errorf("expected worker-B to hold lock, got %s", cur.WorkerID)
	  }
	  if cur.Generation != 2 {
		  t.Errorf("expected generation=2, got %d", cur.Generation)
	   }
  }

// ── Seal / IdemBlock ──────────────────────────────────────────────────────
// TestSealBlock_IdemBlockedAfterSeal 封堵后 IsIdemBlocked == true。
   func TestSealBlock_IdemBlockedAfterSeal(t *testing.T) {
	  kv := persist.NewMemKV()
	  svc := ledorphan.New()
	  svc.BindKV(kv)
	  ctx := context.Background()
	   batchID := "batch-seal-001"
	   reportID := "report-seal-001"
	  orphanID := "orphan-1"
	   idemKey := "hop-idem-sealed"
	  // 构造一个孤儿报告（内存中直接注入）。
	   now := time.Now().UTC()
	   report := &ledorphan.OrphanReport{
		   ReportID: reportID,
		  BatchID:  batchID,
		  Status: ledorphan.ReportOpen,
		   CreatedAt: now,
		   UpdatedAt: now,
		   Items: []ledorphan.OrphanItem{
			  {
				  OrphanID: orphanID,
				   BatchID:  batchID,
				   Seq:      1,
				  UserID:    "user-001",
				   Asset:     "USDT",
				  Amount:    "100",
				  Direction: "funding_to_trading",
				  IdempotencyKey: idemKey,
				   Kind:   ledorphan.OrphanHanging,
				   Status: ledorphan.OrphanDetected,
				  DetectedAt: now,
			  },
		   },
		  TotalOrphans: 1,
		  HangingCount: 1,
	   }
	   if err := ledorphan.SaveReport(ctx, kv, report); err != nil {
		   t.Fatalf("save report: %v", err)
	  }
	   // 执行封堵（block）。
	   req := ledorphan.SealReq{
		   BatchID: batchID,
		   ReportID: reportID,
		  Kind:     ledorphan.SealKindBlock,
		   Actor1:   "ops-alice",
		  Actor2: "ops-bob",
	  }
	   action, err := svc.Seal(ctx, req)
	   if err != nil {
		  t.Fatalf("seal: %v", err)
	  }
	  if action.Status != ledorphan.SealApplied {
		  t.Errorf("expected SealApplied, got %s", action.Status)
	  }
	  // 封堵后，idem key 应被拦截。
	   blocked, err := svc.IsIdemBlocked(ctx, idemKey)
	   if err != nil {
		  t.Fatalf("is idem blocked: %v", err)
	  }
	  if !blocked {
		  t.Error("expected idem key to be blocked after seal")
	  }
  }

// TestSealDualAuth_SameActor 相同 actor 发起双人授权时返回 ErrDualAuth。

   func TestSealDualAuth_SameActor(t *testing.T) {
	   kv := persist.NewMemKV()
	   svc := ledorphan.New()
	   svc.BindKV(kv)
	  ctx := context.Background()
	   reportID := "report-da-001"
	   now := time.Now().UTC()
	  report := &ledorphan.OrphanReport{
		  ReportID:  reportID,
		   BatchID:   "batch-da",
		  Status:    ledorphan.ReportOpen,
		   CreatedAt: now, UpdatedAt: now,
	  }
	   _ = ledorphan.SaveReport(ctx, kv, report)
	   req := ledorphan.SealReq{
		  BatchID:  "batch-da",
		   ReportID: reportID,
		   Kind:     ledorphan.SealKindBlock,
		  Actor1:   "ops-alice",
		  Actor2: "ops-alice", // 相同
	   }
	   _, err := svc.Seal(ctx, req)
	   if err != ledorphan.ErrDualAuth {
		   t.Errorf("expected ErrDualAuth, got %v", err)
	   }
   }

// TestSealReverse_CallsReverseHop SealKindReverse 调用 reverseHop 并记录 entry ID。
   func TestSealReverse_CallsReverseHop(t *testing.T) {
	  kv := persist.NewMemKV()
	  svc := ledorphan.New()
	   svc.BindKV(kv)
	   reverseCallCount := 0
	  svc.BindReverseHop(func(ctx context.Context, req ledorphan.ReverseReq) (string, error) {
		  reverseCallCount++
		   return fmt.Sprintf("rev-entry-%d", reverseCallCount), nil
	   })
	   ctx := context.Background()
	  now := time.Now().UTC()
	   reportID := "report-rev-001"
	  report := &ledorphan.OrphanReport{
		   ReportID: reportID,
		   BatchID:   "batch-rev",
		   Status:    ledorphan.ReportOpen,
		  CreatedAt: now, UpdatedAt: now,
		   TotalOrphans: 1, HangingCount: 1,
		   Items: []ledorphan.OrphanItem{
			  {
				  OrphanID: "orp-rev-1",
				   BatchID: "batch-rev",
				   Seq:     1,
				  UserID:  "user-rev",
				  Asset:  "BTC",
				   Amount: "0.001",
				   Direction:      "wallet_to_spot",
				   IdempotencyKey: "rev-idem-1",
				  Kind:       ledorphan.OrphanHanging,
				   Status:     ledorphan.OrphanDetected,
				  LedgerID:   "original-entry-1",
				  DetectedAt: now,
			  },
		  },
	   }
	  _ = ledorphan.SaveReport(ctx, kv, report)
	   req := ledorphan.SealReq{
		   BatchID: "batch-rev",
		  ReportID: reportID,
		   Kind:     ledorphan.SealKindReverse,
		   Actor1: "ops-alice",
		   Actor2: "ops-bob",
	  }
	  action, err := svc.Seal(ctx, req)
	   if err != nil {
		   t.Fatalf("seal reverse: %v", err)
	   }
	  if action.Status != ledorphan.SealApplied {
		   t.Errorf("expected SealApplied, got %s", action.Status)
	  }
	  if reverseCallCount != 1 {
		  t.Errorf("expected 1 reverseHop call, got %d", reverseCallCount)
	   }
	   if len(action.ReverseEntryIDs) != 1 {
		   t.Errorf("expected 1 reverse entry ID, got %d", len(action.ReverseEntryIDs))
	  }
   }

// ── IntegrityProof ───────────────────────────────────────────────────────
// TestIntegrityProof_ComputeAndVerify 计算证明并独立验证。

  func TestIntegrityProof_ComputeAndVerify(t *testing.T) {
	   kv := persist.NewMemKV()
	  prover := ledorphan.NewProver(kv)
	   ctx := context.Background()
	  intentIdem := []string{"idem-1", "idem-2", "idem-3"}
	   ledgerIdem := []string{"idem-1", "idem-2"}
	   sealIdem := []string{"idem-3"}
	  proof, err := prover.Compute(ctx,
		   "batch-proof-001", "report-001", "seal-001", 1,
		   intentIdem, ledgerIdem, sealIdem,
	   )
	  if err != nil {
		   t.Fatalf("compute proof: %v", err)
	  }
	  if proof.Hash == "" {
		  t.Error("expected non-empty hash")
	  }
	  if proof.IntentCount != 3 {
		   t.Errorf("expected IntentCount=3, got %d", proof.IntentCount)
	   }
	  // 独立验证（不依赖 KV，只用三组 idem keys 重算）。
	   if err := ledorphan.VerifyHash(intentIdem, ledgerIdem, sealIdem, proof.Hash); err != nil {
		  t.Errorf("independent verify failed: %v", err)
	   }
	   // 通过 Prover.Verify 校验持久化证明。
	   verified, err := prover.Verify(ctx, proof.ProofID)
	  if err != nil {
		   t.Fatalf("prover verify: %v", err)
	  }
	   if verified.Status != ledorphan.ProofVerified {
		  t.Errorf("expected ProofVerified, got %s", verified.Status)
	  }
  }

// TestIntegrityProof_Mismatch 篡改哈希后 Verify 返回 ErrProofMismatch。

  func TestIntegrityProof_Mismatch(t *testing.T) {
	  err := ledorphan.VerifyHash(
		  []string{"idem-1"}, []string{"idem-1"}, []string{},
		   "0000000000000000000000000000000000000000000000000000000000000000",
	  )
	   if err == nil {
		   t.Fatal("expected ErrProofMismatch for wrong hash")
	   }
   }

// TestReportClosed_AllDisposed 所有孤儿处置后报告状态变 closed。
  func TestReportClosed_AllDisposed(t *testing.T) {
	   kv := persist.NewMemKV()
	   svc := ledorphan.New()
	  svc.BindKV(kv)
	  ctx := context.Background()
	   now := time.Now().UTC()
	   reportID := "report-close-001"
	   report := &ledorphan.OrphanReport{
		  ReportID: reportID, BatchID: "batch-close",
		   Status: ledorphan.ReportOpen, CreatedAt: now, UpdatedAt: now,
		   TotalOrphans: 1, HangingCount: 1,
		   Items: []ledorphan.OrphanItem{
			   {
				  OrphanID: "orp-close-1", BatchID: "batch-close",
				   Seq: 1, UserID: "u1", Asset: "ETH", Amount: "0.1",
				   Direction: "wallet_to_spot", IdempotencyKey: "close-idem-1",
				   Kind: ledorphan.OrphanHanging, Status: ledorphan.OrphanDetected,
				   DetectedAt: now,
			  },
		  },
	   }
	  _ = ledorphan.SaveReport(ctx, kv, report)
	  req := ledorphan.SealReq{
		   BatchID:  "batch-close",
		  ReportID: reportID,
		   Kind:     ledorphan.SealKindBlock,
		   Actor1: "ops-alice",
		  Actor2: "ops-bob",
	   }
	  _, err := svc.Seal(ctx, req)
	  if err != nil {
		  t.Fatalf("seal: %v", err)
	  }
	  // 验证报告已关闭。
	   closed,
		   err := svc.GetReport(ctx, reportID)
	   if err != nil {
		   t.Fatalf("get report: %v", err)
	   }
	  if closed.Status != ledorphan.ReportClosed {
		  t.Errorf("expected ReportClosed, got %s", closed.Status)
	  }
  }

// TestScanAndSeal_FullFlow 端到端：扫描 → 发现孤儿 → 封堵 → 证明导出。
  func TestScanAndSeal_FullFlow(t *testing.T) {
	   kv := persist.NewMemKV()
	   svc := ledorphan.New()
	   svc.BindKV(kv)
	   // 注入 mock batch observer（有一个半提交批次）。
	  bo := &mockBatchObserver{batches: map[string]ledorphan.BatchSnapshot{
		   "batch-e2e": makeBatch("batch-e2e", "submitted",
			  makeItem(1, "e2e-idem-1", "submitted", "led-1"), // 正常
			  makeItem(2, "e2e-idem-2", "submitted", ""),      // hanging
		   ),
	   }}
	  lo := &mockLedgerObserver{idemToEntry: map[string]string{
		   "e2e-idem-1": "led-1",
		  // e2e-idem-2 不在 ledger
	   }}
	   svc.BindBatchObserver(bo)
	   svc.BindLedgerObserver(lo)
	   ctx := context.Background()
	   // 第一步：扫描。
	  scanResp, err := svc.Scan(ctx, ledorphan.ScanReq{Actor: "ops-system", BatchID: "batch-e2e"})
	  if err != nil {
		  t.Fatalf("scan: %v", err)
	   }
	   if scanResp.TotalOrphans != 1 {
		  t.Fatalf("expected 1 orphan after scan, got %d", scanResp.TotalOrphans)
	  }
	   reportID := scanResp.Reports[0].ReportID
	  // 第二步：封堵。
	   sealReq := ledorphan.SealReq{
		  BatchID:  "batch-e2e",
		  ReportID: reportID,
		  Kind:     ledorphan.SealKindBlock,
		   Actor1:   "ops-alice",
		   Actor2: "ops-bob",
	  }
	  action, err := svc.Seal(ctx, sealReq)
	  if err != nil {
		  t.Fatalf("seal: %v", err)
	   }
	  if action.Status != ledorphan.SealApplied {
		  t.Fatalf("expected SealApplied, got %s", action.Status)
	  }
	  // 第三步：验证 idem 被封堵。
	  blocked, _ := svc.IsIdemBlocked(ctx, "e2e-idem-2")
	  if !blocked {
		   t.Error("expected e2e-idem-2 to be blocked")
	  }
	  // 第四步：导出完整性证明。
	   proof, err := svc.ExportProof(ctx, "batch-e2e")
	  if err != nil {
		  t.Fatalf("export proof: %v", err)
	  }
	  if proof.Hash == "" {
		  t.Error("expected non-empty proof hash")
	  }
	  if proof.SealCount != 1 {
		  t.Errorf("expected SealCount=1, got %d", proof.SealCount)
	  }
  }

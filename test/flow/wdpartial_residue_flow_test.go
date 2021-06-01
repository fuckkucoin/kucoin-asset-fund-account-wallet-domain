package flow

import (
	"errors"
	"fmt"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/wdpartial"
)

// ─── 共享时钟辅助 ────────────────────────────────────────────────────────────

   func wdpFixedClock(t time.Time) func() time.Time {
	   return func() time.Time { return t }
  }

// ─── 场景 1：5笔中2笔failed → ResidueItem=2；未释放前不得 closed ─────────────

  func TestWDPartialFlow_Scenario1_5Items2Failed(t *testing.T) {
	  t.Parallel()
	  base := time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)
	  clk := wdpFixedClock(base)
	  batchID := "flow-batch-5items-2failed"
	  // 构造 wdbatch 只读视图：5笔，第4笔 skipped，第5笔 failed
	   reader := wdpartial.NewMockBatchReader()
	   obs := wdpartial.NewMockObserver()
	  items := []wdpartial.BatchItemView{
		  {Seq: 1, UserID: "u1", Asset: "USDT", Amount: "100", Chain: "ETH",
			   ToAddress: "0xaaa", IdempotencyKey: "idem-1", Status: "submitted", LedgerID: "led-1"},
		   {Seq: 2, UserID: "u2", Asset: "USDT", Amount: "100", Chain: "ETH",
			  ToAddress: "0xbbb", IdempotencyKey: "idem-2", Status: "submitted", LedgerID: "led-2"},
		   {Seq: 3, UserID: "u3", Asset: "USDT", Amount: "100", Chain: "ETH",
			  ToAddress: "0xccc", IdempotencyKey: "idem-3", Status: "submitted", LedgerID: "led-3"},
		   {Seq: 4, UserID: "u4", Asset: "USDT", Amount: "50", Chain: "ETH",
			  ToAddress: "0xddd", IdempotencyKey: "idem-4", Status: "skipped",
			  Reason: "frozen"},
		  {Seq: 5, UserID: "u5", Asset: "USDT", Amount: "150", Chain: "ETH",
			   ToAddress: "0xeee", IdempotencyKey: "idem-5", Status: "failed",
			   Reason: "withdraw gate unbound"},
	   }
	   // 设置第4、5项有冻结占用
	   obs.SetFreeze("u4", "USDT", "", true)
	   obs.SetFreeze("u5", "USDT", "", true)
	   reader.AddBatch(batchID, items, "500", "300", "50", "150")
	   // 构建 Service
	  svc := buildTestService(t, reader, obs, clk)
	  // Step 1: 检测残留
	  result, err := svc.DetectResidues(batchID)
	  if err != nil {
		  t.Fatalf("DetectResidues: %v", err)
	  }
	   if result.ResidueCount != 2 {
		  t.Errorf("ResidueCount want=2, got=%d", result.ResidueCount)
	   }
	  residues, _ := svc.ListResidues(wdpartial.ResidueFilter{WDBatchID: batchID})
	   if len(residues) != 2 {
		   t.Fatalf("ListResidues want=2, got=%d", len(residues))
	   }
	  // Step 2: 获取世代锁
	   lease, err := svc.AcquireResumeLease(batchID, "ops-worker-1")
	   if err != nil {
		  t.Fatalf("AcquireResumeLease: %v", err)
	  }
	   // Step 3: 未证明前不得 CloseOut
	  _, err = svc.CloseOut(wdpartial.CloseOutReq{
		   Summary: wdpartial.BatchSummary{
			  WDBatchID: batchID,
			   Requested: "500",
			   Submitted: "300",
		  },
		  Actor: "ops-worker-1",
		   LeaseHolder: "ops-worker-1",
		  LeaseGen: lease.Gen,
	  })
	  if !errors.Is(err, wdpartial.ErrProofMissing) {
		  t.Errorf("未证明前 CloseOut 期望 ErrProofMissing, 得到 %v", err)
	  }
	   // Step 4: 为所有残留项记录证明
	   for _, r := range residues {
		  _, perr := svc.RecordReleaseProof(wdpartial.RecordProofReq{
			  WDBatchID: batchID,
			  ResidueID: r.ID,
			   Kind:      wdpartial.ProofKindFreezeRelease,
			  Actor:       "ops-worker-1",
			   Summary:     fmt.Sprintf("released residue seq=%d", r.Seq),
			  LeaseHolder: "ops-worker-1",
			  LeaseGen: lease.Gen,
		   })
		  if perr != nil {
			  t.Fatalf("RecordReleaseProof seq=%d: %v", r.Seq, perr)
		  }
	   }
	   // Step 5: 证明后可以 CloseOut
	  co, err := svc.CloseOut(wdpartial.CloseOutReq{
		   Summary: wdpartial.BatchSummary{
			   WDBatchID: batchID,
			  Requested: "500",
			  Submitted: "300",
		  },
		  Actor: "ops-worker-1",
		   LeaseHolder: "ops-worker-1",
		   LeaseGen:    lease.Gen,
	  })
	  if err != nil {
		   t.Fatalf("CloseOut: %v", err)
	  }
	  if co.Status != wdpartial.BatchResidueClosed {
		   t.Errorf("CloseOut Status want=closed, got=%s", co.Status)
	   }
	   if !co.ConserveOK {
		   t.Errorf("ConserveOK want=true, note=%s", co.ConserveNote)
	  }
   }

// ─── 场景 2：双进程 Resume 互斥 ───────────────────────────────────────────────

  func TestWDPartialFlow_Scenario2_DualWorkerMutex(t *testing.T) {
	   t.Parallel()
	   base := time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)
	  batchID := "flow-batch-dual-resume"
	   svc := buildTestService(t, wdpartial.NewMockBatchReader(), wdpartial.NewMockObserver(), wdpFixedClock(base))
	   // Step 1: worker-A 获取世代锁 Gen=1
	  leaseA, err := svc.AcquireResumeLease(batchID, "worker-A")
	  if err != nil {
		   t.Fatalf("worker-A AcquireResumeLease: %v", err)
	   }
	   if leaseA.Gen != 1 {
		   t.Errorf("worker-A Gen want=1, got=%d", leaseA.Gen)
	   }
	  // Step 2: worker-B 被拒 ErrLeaseConflict
	   _, err = svc.AcquireResumeLease(batchID, "worker-B")
	  if !errors.Is(err, wdpartial.ErrLeaseConflict) {
		   t.Errorf("worker-B 期望 ErrLeaseConflict, 得到 %v", err)
	   }
	  // Step 3: worker-A 释放
	   if err := svc.ReleaseLease(batchID, "worker-A", leaseA.Gen); err != nil {
		  t.Fatalf("worker-A ReleaseLease: %v", err)
	   }
	  // Step 4: worker-B 抢占 Gen=2
	   leaseB, err := svc.AcquireResumeLease(batchID, "worker-B")
	  if err != nil {
		   t.Fatalf("worker-B AcquireResumeLease 抢占: %v", err)
	  }
	   if leaseB.Gen != 2 {
		   t.Errorf("worker-B Gen want=2, got=%d", leaseB.Gen)
	  }
	  // Step 5: worker-A 用旧 Gen=1 Verify → ErrNoLease（holder 不匹配）
	   err = svc.VerifyLease(batchID, "worker-A", leaseA.Gen)
	   if !errors.Is(err, wdpartial.ErrNoLease) {
		   t.Errorf("worker-A 旧 Gen Verify 期望 ErrNoLease, 得到 %v", err)
	  }
	  // Step 6: worker-B 用 Gen=2 Verify 成功
	   if err := svc.VerifyLease(batchID, "worker-B", leaseB.Gen); err != nil {
		   t.Errorf("worker-B Verify 期望 nil, 得到 %v", err)
	  }
   }

// ─── 场景 3：释放证明后 Conserve 成立，再提不双冻 ────────────────────────────

  func TestWDPartialFlow_Scenario3_ProofConserveNoDoubleFreeze(t *testing.T) {
	   t.Parallel()
	   base := time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)
	   clk := wdpFixedClock(base)
	   batchID := "flow-batch-proof-conserve"
	  reader := wdpartial.NewMockBatchReader()
	   obs := wdpartial.NewMockObserver()
	   items := []wdpartial.BatchItemView{
		  {Seq: 1, UserID: "u1", Asset: "USDT", Amount: "300", Chain: "ETH",
			  ToAddress: "0xaaa", IdempotencyKey: "idem-p1", Status: "submitted", LedgerID: "led-p1"},
		  {Seq: 2, UserID: "u2", Asset: "USDT", Amount: "50", Chain: "ETH",
			  ToAddress: "0xbbb", IdempotencyKey: "idem-p2", Status: "skipped", Reason: "frozen"},
		   {Seq: 3, UserID: "u3", Asset: "USDT", Amount: "150", Chain: "ETH",
			   ToAddress: "0xccc", IdempotencyKey: "idem-p3", Status: "failed", Reason: "gate error"},
	   }
	  reader.AddBatch(batchID, items, "500", "300", "50", "150")
	   svc := buildTestService(t,
		  reader, obs, clk)
	   // 检测残留
	   dr, _ := svc.DetectResidues(batchID)
	   if dr.ResidueCount != 2 {
		   t.Fatalf("期望 2 残留, got %d", dr.ResidueCount)
	  }
	  residues, _ := svc.ListResidues(wdpartial.ResidueFilter{WDBatchID: batchID})
	   // 获取锁
	   lease, _ := svc.AcquireResumeLease(batchID, "ops")
	  // 记录释放证明
	   for _, r := range residues {
		  _, err := svc.RecordReleaseProof(wdpartial.RecordProofReq{
			   WDBatchID: batchID,
			  ResidueID: r.ID,
			  Kind:        wdpartial.ProofKindFreezeRelease,
			   Actor:       "ops",
			   Summary:     "released",
			  LeaseHolder: "ops",
			   LeaseGen:    lease.Gen,
		   })
		  if err != nil {
			  t.Fatalf("RecordReleaseProof: %v", err)
		   }
	   }
	  // CheckConserve dry-run
	  cr, err := svc.CheckConserve(wdpartial.BatchSummary{
		  WDBatchID: batchID,
		  Requested: "500",
		  Submitted: "300",
	   })
	   if err != nil {
		   t.Fatalf("CheckConserve: %v", err)
	  }
	  if !cr.Balanced {
		  t.Errorf("Conserve 期望平衡, note=%s", cr.Note)
	  }
	  // 双冻检测：相同 idem_key 被拒
	   err = wdpartial.AssertNoDoubleFreezeOnRetry(
		  []wdpartial.BatchItemView{{IdempotencyKey: "idem-p3"}},
		  residues,
	   )
	   if !errors.Is(err, wdpartial.ErrConserve) {
		   t.Errorf("双冻检测: 期望 ErrConserve, 得到 %v", err)
	  }
	   // 双冻检测：加后缀后允许
	   err = wdpartial.AssertNoDoubleFreezeOnRetry(
		  []wdpartial.BatchItemView{{IdempotencyKey: "idem-p3-retry"}},
		  residues,
	   )
	   if err != nil {
		   t.Errorf("加后缀后不应报错: %v", err)
	   }
	  // CloseOut
	   co, err := svc.CloseOut(wdpartial.CloseOutReq{
		  Summary: wdpartial.BatchSummary{
			  WDBatchID: batchID,
			   Requested: "500",
			   Submitted: "300",
		   },
		  Actor: "ops",
		   LeaseHolder: "ops",
		   LeaseGen:    lease.Gen,
	  })
	   if err != nil {
		   t.Fatalf("CloseOut: %v", err)
	  }
	   if !co.ConserveOK {
		  t.Errorf("CloseOut ConserveOK want=true")
	   }
   }

// ─── 场景 4：源 ID 隔离 ───────────────────────────────────────────────────────

  func TestWDPartialFlow_Scenario4_SourceIsolation(t *testing.T) {
	  t.Parallel()
	  // wdpartial 的残留项只能由 wdbatch ID 来驱动
	  // 尝试用 ledbatch ID 查询，BatchReader 返回 ErrBatchNotFound
	  reader := wdpartial.NewMockBatchReader()
	  // 注意：不添加任何 batch， ledbatch ID 找不到
	   svc := buildTestService(t, reader, wdpartial.NewMockObserver(), wdpFixedClock(time.Now()))
	   // 用 ledbatch 形态的 ID 调用 Detect → 应得 ErrBatchNotFound（不是 wdbatch 视图）
	   _, err := svc.DetectResidues("ledbatch-hop-orphan-xxxxx")
	  if !errors.Is(err, wdpartial.ErrBatchNotFound) {
		  t.Errorf("ledbatch ID 期望 ErrBatchNotFound, 得到 %v", err)
	  }
	   // 验证 BatchCloseout.WDBatchID 只接受 wdbatch ID（通过正常流程写入）
	   reader.AddBatch("wdbatch-real-001", []wdpartial.BatchItemView{
		   {Seq: 1, UserID: "u1", Asset: "USDT", Amount: "100", Chain: "ETH",
			   ToAddress: "0xaaa", IdempotencyKey: "idem-real-1", Status: "submitted", LedgerID: "led-1"},
	   }, "100", "100", "0", "0")
	   dr, err := svc.DetectResidues("wdbatch-real-001")
	   if err != nil {
		   t.Fatalf("wdbatch ID Detect: %v", err)
	   }
	   if dr.ResidueCount != 0 {
		  t.Errorf("全部提交的批次应无残留, got %d", dr.ResidueCount)
	   }
  }

// ─── buildTestService helper ─────────────────────────────────────────────────

   func buildTestService(
	   t *testing.T,
	   reader *wdpartial.MockBatchReader,
	  obs *wdpartial.MockObserver,
	   clk func() time.Time,
) *wdpartial.Service {
	t.Helper()
	return wdpartial.NewService().
		WithBatchReader(reader).
		WithObserver(obs).
		WithClock(clk)
}

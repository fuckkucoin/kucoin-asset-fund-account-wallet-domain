package wdpartial_test

import (
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/wdpartial"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

   func newProofTestEnv(t *testing.T, batchID string, nFailed int) (
	  *wdpartial.ProofManager,
	   *wdpartial.MutexManager,
	   *wdpartial.MemResidueBook,
	   *wdpartial.MemLeaseBook,
	  []*wdpartial.ResidueItem,
   ) {
	   t.Helper()
	  now := time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)
	   clk := fixedClock(now)
	  leaseBook := wdpartial.NewMemLeaseBook()
	  residueBook := wdpartial.NewMemResidueBook()
	  proofBook := wdpartial.NewMemProofBook()
	   mutex := wdpartial.NewMutexManager(leaseBook).WithClock(clk)
	  mgr := wdpartial.NewProofManager(proofBook, residueBook, mutex).WithClock(clk)
	   // 创建残留项
	  items := make([]*wdpartial.ResidueItem,
		  nFailed)
	   for i := 0; i < nFailed; i++ {
		  items[i] = &wdpartial.ResidueItem{
			   ID:        wdpartial.MustNewUUID(),
			   WDBatchID: batchID,
			  Seq:     i + 1,
			  UserID:  "user-" + batchID,
			  Asset:   "USDT",
			   Amount:  "100",
			   IdemKey: "idem-" + batchID + "-" + string(rune('a'+i)),
			   Origin: wdpartial.ResidueOriginFailed,
			  Status: wdpartial.ResidueOpen,
			  DetectedAt: now,
			  UpdatedAt: now,
		   }
	  }
	  if err := residueBook.Save(items); err != nil {
		   t.Fatalf("Save residues: %v", err)
	  }
	   return mgr, mutex, residueBook, leaseBook, items
   }

// ─── 表驱动 ───────────────────────────────────────────────────────────────

   func TestProofManagerRecordProof(t *testing.T) {
	   t.Parallel()
	   cases := []struct {
		  name     string
		  batchID  string
		   nFailed  int
		  proofIdx int
		   kind   wdpartial.ReleaseProofKind
		   holder string
		   acquireFirst bool
		   doubleProof  bool
		  wantStatus wdpartial.ResidueStatus
		   wantErr error
	  }{
		  {
			   name: "正常记录 freeze_release 证明",
			  batchID: "batch-proof-ok",
			   nFailed: 2,
			   proofIdx: 0,
			  kind:     wdpartial.ProofKindFreezeRelease,
			  holder:   "ops-worker",
			  acquireFirst: true,
			  wantStatus:   wdpartial.ResidueReleased,
		  },
		  {
			  name: "escalate_susp → status=escalated",
			   batchID:  "batch-proof-esc",
			  nFailed:  1,
			  proofIdx: 0,
			   kind:         wdpartial.ProofKindEscalateSusp,
			  holder:       "ops-worker",
			  acquireFirst: true,
			   wantStatus:   wdpartial.ResidueEscalated,
		  },
		  {
			  name: "no_occupy → status=released",
			  batchID: "batch-proof-nooccupy",
			  nFailed:  1,
			  proofIdx: 0,
			  kind:     wdpartial.ProofKindNoOccupy,
			  holder:   "ops-worker",
			  acquireFirst: true,
			   wantStatus:   wdpartial.ResidueReleased,
		   },
		   {
			   name: "无租约记录 → ErrNoLease",
			  batchID:      "batch-proof-nolease",
			  nFailed:      1,
			   proofIdx:     0,
			  kind:         wdpartial.ProofKindFreezeRelease,
			  holder:       "ops-worker",
			  acquireFirst: false,
			   wantErr: wdpartial.ErrNoLease,
		   },
		  {
			   name:    "重复记录证明第一次成功再次调用ErrProofDuplicate",
			  batchID: "batch-proof-dup",
			   nFailed:  1,
			   proofIdx: 0,
			  kind:     wdpartial.ProofKindFreezeRelease,
			   holder: "ops-worker",
			   acquireFirst: true,
			  doubleProof:  true,
			  wantStatus:   wdpartial.ResidueReleased,
		  },
	   }
	  for _, tc := range cases {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			  mgr, mutex, residueBook, _, items := newProofTestEnv(t, tc.batchID, tc.nFailed)
			  var gen int64
			   if tc.acquireFirst {
				  lease, err := mutex.Acquire(tc.batchID, tc.holder)
				   if err != nil {
					  t.Fatalf("Acquire lease: %v", err)
				   }
				  gen = lease.Gen
			  }
			  req := wdpartial.RecordProofReq{
				   WDBatchID: tc.batchID,
				  ResidueID: items[tc.proofIdx].ID,
				  Kind:    tc.kind,
				  Actor:   tc.holder,
				  Summary: "test proof",
				  LeaseHolder: tc.holder,
				   LeaseGen: gen,
			  }
			  proof, err := mgr.RecordProof(req)
			  if tc.wantErr != nil {
				   if !errors.Is(err, tc.wantErr) {
					  t.Errorf("期望 %v, 得到 %v", tc.wantErr, err)
				  }
				  return
			  }
			  if err != nil {
				  t.Fatalf("意外错误: %v", err)
			  }
			  if proof == nil {
				   t.Fatal("proof 为 nil")
			   }
			   // 验证 Checksum
			   if verr := wdpartial.VerifyChecksum(proof); verr != nil {
				   t.Errorf("Checksum 校验失败: %v", verr)
			   }
			  // 验证状态
			   updated, err := residueBook.Get(items[tc.proofIdx].ID)
			   if err != nil {
				   t.Fatalf("Get residue: %v", err)
			   }
			  if updated.Status != tc.wantStatus {
				   t.Errorf("ResidueItem.Status: want=%s got=%s", tc.wantStatus, updated.Status)
			  }
			  // 验证双重记录
			   if tc.doubleProof {
				   _, err2 := mgr.RecordProof(req)
				   if !errors.Is(err2, wdpartial.ErrProofDuplicate) {
					  t.Errorf("重复证明期望 ErrProofDuplicate, 得到 %v", err2)
				   }
			  }
		  })
	   }
   }

// ─── AssertAllProved ─────────────────────────────────────────────────────

   func TestAssertAllProved(t *testing.T) {
	   t.Parallel()
	  t.Run("全部已证明 → nil", func(t *testing.T) {
		  t.Parallel()
		   mgr, mutex, _, _, items := newProofTestEnv(t, "batch-allproved", 3)
		   lease, _ := mutex.Acquire("batch-allproved", "ops")
		  for _, it := range items {
			   _, err := mgr.RecordProof(wdpartial.RecordProofReq{
				  WDBatchID: "batch-allproved",
				   ResidueID: it.ID,
				   Kind: wdpartial.ProofKindNoOccupy,
				  Actor: "ops",
				  Summary:     "all clear",
				   LeaseHolder: "ops",
				   LeaseGen:    lease.Gen,
			   })
			   if err != nil {
				   t.Fatalf("RecordProof %s: %v", it.ID, err)
			  }
		   }
		   if err := mgr.AssertAllProved("batch-allproved"); err != nil {
			   t.Errorf("AssertAllProved 应 nil, 得到 %v", err)
		  }
	   })
	   t.Run("缺少证明 → ErrProofMissing", func(t *testing.T) {
		   t.Parallel()
		  mgr, mutex, _, _, items := newProofTestEnv(t, "batch-missing-proof", 3)
		  lease, _ := mutex.Acquire("batch-missing-proof", "ops")
		  // 只记录前两条
		   for _, it := range items[:2] {
			   _, err := mgr.RecordProof(wdpartial.RecordProofReq{
				   WDBatchID: "batch-missing-proof",
				  ResidueID: it.ID,
				  Kind:    wdpartial.ProofKindNoOccupy,
				   Actor:   "ops",
				  Summary: "partial clear",
				  LeaseHolder: "ops",
				  LeaseGen: lease.Gen,
			   })
			  if err != nil {
				  t.Fatalf("RecordProof: %v", err)
			   }
		   }
		   // 第三条未证明
		   err := mgr.AssertAllProved("batch-missing-proof")
		  if !errors.Is(err, wdpartial.ErrProofMissing) {
			   t.Errorf("期望 ErrProofMissing, 得到 %v", err)
		  }
	   })
  }

// ─── VerifyChecksum ──────────────────────────────────────────────────────

  func TestVerifyChecksum(t *testing.T) {
	   t.Parallel()
	  t.Run("正确 checksum", func(t *testing.T) {
		  t.Parallel()
		  mgr, mutex, _, _, items := newProofTestEnv(t, "batch-chksum-ok", 1)
		  lease, _ := mutex.Acquire("batch-chksum-ok", "ops")
		   proof, _ := mgr.RecordProof(wdpartial.RecordProofReq{
			  WDBatchID: "batch-chksum-ok",
			   ResidueID: items[0].ID,
			  Kind:        wdpartial.ProofKindFreezeRelease,
			   Actor:       "ops",
			   Summary:     "checksum test",
			   LeaseHolder: "ops",
			  LeaseGen:    lease.Gen,
		   })
		  if err := wdpartial.VerifyChecksum(proof); err != nil {
			   t.Errorf("正确 checksum 验证失败: %v", err)
		   }
	   })
	   t.Run("篡改 checksum", func(t *testing.T) {
		  t.Parallel()
		   mgr, mutex, _, _, items := newProofTestEnv(t, "batch-chksum-bad", 1)
		  lease, _ := mutex.Acquire("batch-chksum-bad", "ops")
		   proof, _ := mgr.RecordProof(wdpartial.RecordProofReq{
			   WDBatchID: "batch-chksum-bad",
			   ResidueID: items[0].ID,
			  Kind: wdpartial.ProofKindFreezeRelease,
			  Actor: "ops",
			   Summary:     "tamper test",
			  LeaseHolder: "ops",
			  LeaseGen:    lease.Gen,
		  })
		   proof.Checksum = "00000000000000000000000000000000"
		  if err := wdpartial.VerifyChecksum(proof); !errors.Is(err, wdpartial.ErrProofChecksum) {
			  t.Errorf("期望 ErrProofChecksum, 得到 %v", err)
		   }
	   })
  }

// ─── MarkRetried ────────────────────────────────────────────────────────

  func TestMarkRetried(t *testing.T) {
	   t.Parallel()
	  mgr, mutex, residueBook, _, items := newProofTestEnv(t, "batch-retried", 2)
	   lease, _ := mutex.Acquire("batch-retried", "ops")
	  // 标记 item[0] 为 retried
	   if err := mgr.MarkRetried(items[0].ID, "batch-retry-new-001", "ops", lease.Gen); err != nil {
		  t.Fatalf("MarkRetried: %v", err)
	  }
	  updated, err := residueBook.Get(items[0].ID)
	   if err != nil {
		  t.Fatalf("Get: %v", err)
	   }
	   if updated.Status != wdpartial.ResidueRetried {
		  t.Errorf("Status want=retried, got=%s", updated.Status)
	  }
	  if updated.RetryBatchID != "batch-retry-new-001" {
		   t.Errorf("RetryBatchID want=batch-retry-new-001, got=%s", updated.RetryBatchID)
	   }
	   // 已终态的不可再 MarkRetried
	   err = mgr.MarkRetried(items[0].ID, "batch-retry-dup", "ops", lease.Gen)
	  if !errors.Is(err, wdpartial.ErrTerminal) {
		   t.Errorf("终态二次 MarkRetried 期望 ErrTerminal, 得到 %v", err)
	  }
  }

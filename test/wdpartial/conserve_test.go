package wdpartial_test

import (
	"errors"
	"fmt"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/wdpartial"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

type conserveTestEnv struct {
	batchID string
	guard   *wdpartial.ConserveGuard
	proofMgr *wdpartial.ProofManager
	mutex    *wdpartial.MutexManager
	book     *wdpartial.MemResidueBook
	proofs   *wdpartial.MemProofBook
	closeout *wdpartial.MemCloseoutBook
	items []*wdpartial.ResidueItem
}

  func newConserveTestEnv(t *testing.T, batchID string, nFailed, nSkipped int) conserveTestEnv {
	   t.Helper()
	  now := time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)
	   clk := fixedClock(now)
	  leaseBook := wdpartial.NewMemLeaseBook()
	  residueBook := wdpartial.NewMemResidueBook()
	   proofBook := wdpartial.NewMemProofBook()
	   closeoutBook := wdpartial.NewMemCloseoutBook()
	  mutex := wdpartial.NewMutexManager(leaseBook).WithClock(clk)
	   proofMgr := wdpartial.NewProofManager(proofBook,
		   residueBook, mutex).WithClock(clk)
	   guard := wdpartial.NewConserveGuard(residueBook, proofBook, closeoutBook, mutex).WithClock(clk)
	  items := make([]*wdpartial.ResidueItem, 0, nFailed+nSkipped)
	   for i := 0; i < nFailed; i++ {
		   items = append(items, &wdpartial.ResidueItem{
			  ID: wdpartial.MustNewUUID(),
			  WDBatchID: batchID,
			   Seq:    i + 1,
			   UserID: fmt.Sprintf("user-failed-%d", i),
			  Asset:  "USDT",
			  Amount: "100",
			  IdemKey: fmt.Sprintf("idem-failed-%d", i),
			  Origin: wdpartial.ResidueOriginFailed,
			   Status: wdpartial.ResidueOpen,
			  DetectedAt: now,
			  UpdatedAt: now,
		   })
	   }
	   for i := 0; i < nSkipped; i++ {
		  items = append(items, &wdpartial.ResidueItem{
			  ID:        wdpartial.MustNewUUID(),
			   WDBatchID: batchID,
			   Seq: nFailed + i + 1,
			   UserID: fmt.Sprintf("user-skipped-%d", i),
			  Asset: "USDT",
			  Amount:  "50",
			  IdemKey: fmt.Sprintf("idem-skipped-%d", i),
			  Origin:  wdpartial.ResidueOriginSkipped,
			  Status: wdpartial.ResidueOpen,
			  DetectedAt: now,
			  UpdatedAt:  now,
		   })
	  }
	   if err := residueBook.Save(items); err != nil {
		  t.Fatalf("Save residues: %v", err)
	  }
	   return conserveTestEnv{
		  batchID: batchID,
		   guard:    guard,
		  proofMgr: proofMgr,
		   mutex:  mutex,
		   book:   residueBook,
		  proofs: proofBook,
		   closeout: closeoutBook,
		   items:    items,
	  }
  }

   func (e *conserveTestEnv) proveAll(t *testing.T, holder string, gen int64, kind wdpartial.ReleaseProofKind) {
	  t.Helper()
	  for _, it := range e.items {
		  _, err := e.proofMgr.RecordProof(wdpartial.RecordProofReq{
			   WDBatchID: e.batchID,
			   ResidueID: it.ID,
			  Kind:        kind,
			   Actor:       holder,
			   Summary:     "test all proved",
			  LeaseHolder: holder,
			  LeaseGen:    gen,
		  })
		   if err != nil {
			   t.Fatalf("RecordProof %s: %v", it.ID, err)
		   }
	  }
   }

// ─── CloseOut 表驱动 ──────────────────────────────────────────────────────

  func TestConserveGuardCloseOut(t *testing.T) {
	  t.Parallel()
	   cases := []struct {
		  name string
		   batchID string
		   nFailed int
		   nSkipped int
		  requested string
		  submitted string
		   proveAll  bool
		   proveKind    wdpartial.ReleaseProofKind
		  leaveOpenIdx int // -1 = 全部证明；>=0 = 此 idx 不证明
		  wantStatus   wdpartial.BatchResidueStatus
		  wantConserve bool
		  wantErr error
	   }{
		  {
			   name:     "验收3: 2failed全证明 → closed + conserve=true",
			   batchID:  "batch-closeout-acc3",
			   nFailed:  2,
			   nSkipped: 0,
			  requested: "500",
			   submitted: "300",
			  proveAll: true,
			  proveKind:    wdpartial.ProofKindFreezeRelease,
			   leaveOpenIdx: -1,
			  wantStatus:   wdpartial.BatchResidueClosed,
			  wantConserve: true,
		  },
		   {
			   name:    "含1 escalated → status=escalated + conserve=true",
			  batchID: "batch-closeout-esc",
			  nFailed:   2,
			   nSkipped:  0,
			  requested: "500",
			  submitted:    "300",
			   proveAll:     true,
			  proveKind:    wdpartial.ProofKindEscalateSusp,
			  leaveOpenIdx: -1,
			   wantStatus:   wdpartial.BatchResidueEscalated,
			   wantConserve: true,
		   },
		   {
			  name: "有open项未证明 → ErrProofMissing",
			  batchID:  "batch-closeout-missing",
			  nFailed:  2,
			   nSkipped: 0,
			  requested: "500",
			   submitted: "300",
			   proveAll:     false,
			  leaveOpenIdx: 1,
			   wantErr:      wdpartial.ErrProofMissing,
		  },
		   {
			   name: "Conserve不平衡 → ErrConserve",
			   batchID: "batch-closeout-unbalanced",
			  nFailed:   1,
			  nSkipped:  0,
			  requested: "500",
			  submitted: "300",
			  proveAll: true,
			   proveKind:    wdpartial.ProofKindNoOccupy,
			  leaveOpenIdx: -1,
			   wantErr: wdpartial.ErrConserve,
		  },
	  }
	  for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			  env := newConserveTestEnv(t, tc.batchID, tc.nFailed, tc.nSkipped)
			  lease, err := env.mutex.Acquire(tc.batchID, "ops")
			   if err != nil {
				   t.Fatalf("Acquire: %v", err)
			   }
			  if tc.proveAll && tc.leaveOpenIdx < 0 {
				   env.proveAll(t, "ops", lease.Gen, tc.proveKind)
			   } else if tc.leaveOpenIdx >= 0 {
				  for i, it := range env.items {
					  if i == tc.leaveOpenIdx {
						  continue
					   }
					  _,
						  perr := env.proofMgr.RecordProof(wdpartial.RecordProofReq{
						   WDBatchID:   tc.batchID,
						  ResidueID:   it.ID,
						   Kind:        wdpartial.ProofKindNoOccupy,
						   Actor:       "ops",
						  Summary:     "partial",
						  LeaseHolder: "ops",
						  LeaseGen:    lease.Gen,
					   })
					  if perr != nil {
						   t.Fatalf("RecordProof: %v", perr)
					  }
				   }
			   }
			  req := tc.requested
			  if req == "" {
				  req = "500"
			  }
			  sub := tc.submitted
			  if sub == "" {
				  sub = "300"
			   }
			  co, err := env.guard.CloseOut(wdpartial.CloseOutReq{
				   Summary: wdpartial.BatchSummary{
					  WDBatchID: tc.batchID,
					   Requested: req,
					   Submitted: sub,
				   },
				   Actor:       "ops",
				  LeaseHolder: "ops",
				  LeaseGen:    lease.Gen,
			   })
			  if tc.wantErr != nil {
				  if !errors.Is(err, tc.wantErr) {
					  t.Errorf("期望 %v, 得到 %v", tc.wantErr, err)
				   }
				   return
			  }
			  if err != nil {
				  t.Fatalf("意外错误: %v", err)
			  }
			  if co.Status != tc.wantStatus {
				  t.Errorf("Status want=%s, got=%s", tc.wantStatus, co.Status)
			  }
			   if co.ConserveOK != tc.wantConserve {
				   t.Errorf("ConserveOK want=%v, got=%v (note=%s)", tc.wantConserve, co.ConserveOK, co.ConserveNote)
			   }
		   })
	   }
  }

// ─── CloseOut 幂等 ────────────────────────────────────────────────────────

  func TestConserveGuardCloseOutIdempotent(t *testing.T) {
	   t.Parallel()
	   env := newConserveTestEnv(t, "batch-closeout-idem", 1, 0)
	  lease, _ := env.mutex.Acquire("batch-closeout-idem", "ops")
	   env.proveAll(t, "ops", lease.Gen, wdpartial.ProofKindNoOccupy)
	  req := wdpartial.CloseOutReq{
		   Summary: wdpartial.BatchSummary{
			   WDBatchID: "batch-closeout-idem",
			   Requested: "400",
			   Submitted: "300",
		  },
		  Actor: "ops",
		   LeaseHolder: "ops",
		   LeaseGen:    lease.Gen,
	   }
	   co1, err1 := env.guard.CloseOut(req)
	  if err1 != nil {
		  t.Fatalf("第一次 CloseOut: %v", err1)
	  }
	   co2, err2 := env.guard.CloseOut(req)
	  if err2 != nil {
		   t.Fatalf("第二次 CloseOut (幂等): %v", err2)
	   }
	  if co1.ID != co2.ID {
		   t.Errorf("幂等 CloseOut 应返回相同 ID: %s vs %s", co1.ID, co2.ID)
	  }
   }

// ─── AssertNoDoubleFreezeOnRetry 表驱动 ──────────────────────────────────

  func TestAssertNoDoubleFreezeOnRetry(t *testing.T) {
	  t.Parallel()
	   // 构造已 released 的残留项
	  existing := []*wdpartial.ResidueItem{
		   {
			   ID: wdpartial.MustNewUUID(),
			  IdemKey: "idem-batch-001-item-5",
			   Status: wdpartial.ResidueReleased,
			   Origin: wdpartial.ResidueOriginFailed,
		   },
		  {
			   ID: wdpartial.MustNewUUID(),
			   IdemKey: "idem-batch-001-item-4",
			  Status:  wdpartial.ResidueReleased,
			   Origin:  wdpartial.ResidueOriginSkipped,
		   },
	  }
	  cases := []struct {
		   name string
		  retryItems []wdpartial.BatchItemView
		   wantErr    bool
	  }{
		   {
			  name: "验收3: 相同 idem_key 被拒（双冻检测）",
			  retryItems: []wdpartial.BatchItemView{
				   {IdempotencyKey: "idem-batch-001-item-5"},
			   },
			  wantErr: true,
		  },
		  {
			  name: "验收3: 加后缀 -retry 的 idem_key 被允许",
			  retryItems: []wdpartial.BatchItemView{
				  {IdempotencyKey: "idem-batch-001-item-5-retry"},
			   },
			  wantErr: false,
		   },
		  {
			   name: "全新 idem_key 被允许",
			  retryItems: []wdpartial.BatchItemView{
				   {IdempotencyKey: "idem-new-batch-002-item-1"},
			  },
			   wantErr: false,
		  },
		   {
			  name: "混合: 一个冲突一个安全 → 整体拒绝",
			  retryItems: []wdpartial.BatchItemView{
				  {IdempotencyKey: "idem-batch-001-item-4"},
				  {IdempotencyKey: "idem-batch-001-item-4-retry"},
			   },
			   wantErr: true,
		   },
	   }
	  for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   err := wdpartial.AssertNoDoubleFreezeOnRetry(tc.retryItems, existing)
			   if tc.wantErr {
				   if !errors.Is(err, wdpartial.ErrConserve) {
					  t.Errorf("期望 ErrConserve, 得到 %v", err)
				  }
			   } else {
				   if err != nil {
					   t.Errorf("意外错误: %v", err)
				  }
			  }
		  })
	   }
   }

// ─── CheckConserve dry-run ───────────────────────────────────────────────

  func TestCheckConserveDryRun(t *testing.T) {
	  t.Parallel()
	  t.Run("全部终态 Conserve 平衡", func(t *testing.T) {
		   t.Parallel()
		   env := newConserveTestEnv(t, "batch-dryrun-ok", 2, 1)
		   lease, _ := env.mutex.Acquire("batch-dryrun-ok", "ops")
		  env.proveAll(t, "ops", lease.Gen, wdpartial.ProofKindFreezeRelease)
		  // 2 failed × 100 + 1 skipped × 50 = 250; submitted = 250 → requested = 500
		   cr, err := env.guard.CheckConserve(wdpartial.BatchSummary{
			   WDBatchID: "batch-dryrun-ok",
			   Requested: "500",
			   Submitted: "250",
		  })
		  if err != nil {
			  t.Fatalf("CheckConserve: %v", err)
		   }
		  if !cr.Balanced {
			   t.Errorf("期望 Balanced=true, note=%s", cr.Note)
		  }
	   })
	  t.Run("有非终态 → Balanced=false", func(t *testing.T) {
		   t.Parallel()
		  env := newConserveTestEnv(t, "batch-dryrun-open", 1, 0)
		   cr, err := env.guard.CheckConserve(wdpartial.BatchSummary{
			   WDBatchID: "batch-dryrun-open",
			   Requested: "400",
			   Submitted: "300",
		  })
		   if err != nil {
			  t.Fatalf("CheckConserve: %v", err)
		  }
		   if cr.Balanced {
			  t.Errorf("有 open 项时 Balanced 应为 false")
		  }
	   })
   }

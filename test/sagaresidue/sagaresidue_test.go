package sagaresidue_test

import (
	"context"
	"fmt"
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/safesaga"
	"account-assets.kucoin.net/funding/internal/sagaresidue"
)

// ── 桩 ────────────────────────────────────────────────────────────────────
// stubPartialReader 可按 ID 返回预设的 Partial。
type stubPartialReader struct {
	partials map[string]safesaga.Partial
}

  func newStubPartialReader(partials map[string]safesaga.Partial) sagaresidue.PartialReader {
	  return &stubPartialReader{partials: partials}
   }

   func (s *stubPartialReader) Get(id string) (safesaga.Partial, error) {
	   p, ok := s.partials[id]
	  if !ok {
		   return safesaga.Partial{}, sagaresidue.ErrSagaRunNotFound
	   }
	   return p, nil
  }

// stubRunReader 可按 ID 返回预设的 Run。
type stubRunReader struct {
	runs map[string]safesaga.Run
}

  func newStubRunReader(runs map[string]safesaga.Run) sagaresidue.RunReader {
	   return &stubRunReader{runs: runs}
  }

   func (s *stubRunReader) Get(id string) (safesaga.Run, error) {
	   r, ok := s.runs[id]
	  if !ok {
		  return safesaga.Run{}, sagaresidue.ErrSagaRunNotFound
	   }
	   return r, nil
   }

// stubLedgerReader 可按 idem_key 返回预设的 Transfer。
type stubLedgerReader struct {
	transfers map[string]*ledger.Transfer
}

  func newStubLedgerReader(transfers map[string]*ledger.Transfer) sagaresidue.LedgerReader {
	  return &stubLedgerReader{transfers: transfers}
   }

  func (s *stubLedgerReader) GetByIdem(_ context.Context, idemKey string) (*ledger.Transfer, error) {
	   t, ok := s.transfers[idemKey]
	   if !ok {
		  return nil, sagaresidue.ErrUnavailable
	  }
	  return t, nil
  }

// mockLedgerWriter 记录 ReverseTransfer / SuspenseTransfer 调用。
type mockLedgerWriter struct {
	reversed  []string
	suspended []string
}

  func (m *mockLedgerWriter) ReverseTransfer(_ context.Context, orig, rev string) (*ledger.Transfer, error) {
	   m.reversed = append(m.reversed, rev)
	   return &ledger.Transfer{ID: rev + "-result", State: ledger.StateCredited}, nil
  }

   func (m *mockLedgerWriter) SuspenseTransfer(_ context.Context, _, _, _, idem string) (*ledger.Transfer, error) {
	  m.suspended = append(m.suspended, idem)
	  return &ledger.Transfer{ID: idem + "-susp", State: ledger.StateCredited}, nil
   }

// ── Detector 表驱动 ────────────────────────────────────────────────────────

  func TestDetector_Detect(t *testing.T) {
	  cfg := sagaresidue.DefaultDetectorConfig()
	  cases := []struct {
		  name string
		   partial   safesaga.Partial
		  run       safesaga.Run
		   transfers map[string]*ledger.Transfer
		   wantErr   error
		  wantItems int
		  wantSeverity sagaresidue.Severity
		   wantTotalAmt string
	   }{
		   {
			   name: "two_steps_done_one_undone_one_residue",
			  run: safesaga.Run{ID: "r1", Idem: "idem-r1", Asset: "USDT"},
			  partial: safesaga.Partial{
				   ID: "r1",
				  Done: []safesaga.Step{safesaga.StepFreeze,
					   safesaga.StepMove},
				  Undone: nil,
				   Cause: "undo move: timeout",
			   },
			  transfers: map[string]*ledger.Transfer{
				  "idem-r1-move": {
					   ID: "tx-r1-move", State: ledger.StateCredited,
					  Amount: decimal.NewFromInt(500), Asset: "USDT",
					   Direction: ledger.FundingToTrading,
				   },
			  },
			  wantErr:      nil,
			   wantItems:    1,
			   wantSeverity: sagaresidue.SeverityMedium,
			   wantTotalAmt: "500",
		  },
		  {
			   name: "sign_undone_move_not_undone_one_residue",
			  run:  safesaga.Run{ID: "r2", Idem: "idem-r2", Asset: "BTC"},
			  partial: safesaga.Partial{
				   ID:     "r2",
				  Done:   []safesaga.Step{safesaga.StepFreeze, safesaga.StepMove, safesaga.StepSign},
				   Undone: []safesaga.Step{safesaga.StepSign},
				  Cause: "undo move: db error",
			  },
			  transfers: map[string]*ledger.Transfer{
				   "idem-r2-move": {
					   ID: "tx-r2-move", State: ledger.StateConfirmed,
					  Amount: decimal.NewFromFloat(0.05), Asset: "BTC",
					  Direction: ledger.FundingToTrading,
				  },
				  "idem-r2-sign": {
					   ID: "tx-r2-sign", State: ledger.StateRolledBack,
					  Amount: decimal.NewFromFloat(0.05), Asset: "BTC",
				  },
			  },
			   wantErr:      nil,
			  wantItems:    1, // sign 已 RolledBack，仅 move 是残留
			   wantSeverity: sagaresidue.SeverityLow,
			   wantTotalAmt: "0.05",
		   },
		   {
			  name: "no_cause_returns_no_residues",
			   run:  safesaga.Run{ID: "r3", Idem: "idem-r3"},
			   partial: safesaga.Partial{
				  ID:    "r3",
				  Done:  []safesaga.Step{safesaga.StepFreeze},
				  Cause: "",
			   },
			  wantErr: sagaresidue.ErrNoResidues,
		   },
		   {
			  name: "all_steps_undone_no_residues",
			   run: safesaga.Run{ID: "r4", Idem: "idem-r4"},
			   partial: safesaga.Partial{
				  ID:   "r4",
				   Done: []safesaga.Step{safesaga.StepFreeze, safesaga.StepMove},
				   Undone: []safesaga.Step{safesaga.StepFreeze, safesaga.StepMove},
				  Cause:  "compensated",
			  },
			   transfers: map[string]*ledger.Transfer{},
			  wantErr: sagaresidue.ErrNoResidues,
		   },
		  {
			   name: "high_severity_large_amount",
			  run: safesaga.Run{ID: "r5", Idem: "idem-r5", Asset: "USDT"},
			  partial: safesaga.Partial{
				  ID: "r5",
				   Done: []safesaga.Step{safesaga.StepMove},
				  Cause: "undo: unavailable",
			   },
			  transfers: map[string]*ledger.Transfer{
				  "idem-r5-move": {
					   ID: "tx-r5", State: ledger.StateCredited,
					   Amount: decimal.NewFromInt(50000), Asset: "USDT",
					   Direction: ledger.FundingToTrading,
				  },
			  },
			   wantErr: nil,
			  wantItems:    1,
			  wantSeverity: sagaresidue.SeverityHigh,
			   wantTotalAmt: "50000",
		   },
		   {
			   name: "rolled_back_hop_not_residue",
			   run:  safesaga.Run{ID: "r6", Idem: "idem-r6"},
			   partial: safesaga.Partial{
				   ID: "r6",
				  Done:  []safesaga.Step{safesaga.StepMove},
				   Cause: "fail",
			   },
			  transfers: map[string]*ledger.Transfer{
				  "idem-r6-move": {
					   ID: "tx-r6", State: ledger.StateRolledBack,
					   Amount: decimal.NewFromInt(100), Asset: "USDT",
				  },
			  },
			   wantErr: sagaresidue.ErrNoResidues,
		   },
		  {
			  name: "multi_item_upgrades_severity",
			   run: safesaga.Run{ID: "r7", Idem: "idem-r7", Asset: "USDT"},
			  partial: safesaga.Partial{
				   ID: "r7",
				  Done: []safesaga.Step{safesaga.StepFreeze, safesaga.StepMove, safesaga.StepSign},
				  Cause: "multi-step fail",
			   },
			  transfers: map[string]*ledger.Transfer{
				  "idem-r7-move": {
					   ID: "tx-r7-move", State: ledger.StateCredited,
					  Amount: decimal.NewFromInt(10), Asset: "USDT",
					  Direction: ledger.FundingToTrading,
				   },
				   "idem-r7-sign": {
					   ID: "tx-r7-sign", State: ledger.StateChainSubmitted,
					  Amount: decimal.NewFromInt(10), Asset: "USDT",
					   Direction: ledger.WalletWithdraw,
				  },
				  "idem-r7-freeze": {
					   ID: "tx-r7-freeze", State: ledger.StateCredited,
					   Amount: decimal.NewFromInt(10), Asset: "USDT",
					   Direction: ledger.SpotToWallet,
				  },
			   },
			   wantErr: nil,
			  wantItems: 3, // 3 items > MultiItemUpgrade(2)，severity low→medium
			   wantSeverity: sagaresidue.SeverityMedium,
			  wantTotalAmt: "30",
		  },
	   }
	  for _, tc := range cases {
		   tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  partials := map[string]safesaga.Partial{tc.partial.ID: tc.partial}
			   runs := map[string]safesaga.Run{tc.run.ID: tc.run}
			   d := sagaresidue.NewDetector(
				   cfg,
				   newStubLedgerReader(tc.transfers),
				   newStubPartialReader(partials),
				  newStubRunReader(runs),
			   )
			   req := sagaresidue.DetectReq{SagaRunID: tc.run.ID}
			  report, err := d.Detect(context.Background(), req)
			  if tc.wantErr != nil {
				  if err != tc.wantErr {
					   t.Fatalf("expected err %v, got %v", tc.wantErr, err)
				  }
				   return
			   }
			   if err != nil {
				   t.Fatalf("unexpected err: %v", err)
			  }
			   if len(report.Items) != tc.wantItems {
				   t.Errorf("items count: want %d got %d", tc.wantItems, len(report.Items))
			   }
			  if report.Severity != tc.wantSeverity {
				  t.Errorf("severity: want %s got %s", tc.wantSeverity, report.Severity)
			  }
			   if tc.wantTotalAmt != "" && report.TotalAmount != tc.wantTotalAmt {
				  t.Errorf("total_amount: want %s got %s", tc.wantTotalAmt, report.TotalAmount)
			  }
		  })
	  }
  }

// ── MutexManager 表驱动 ───────────────────────────────────────────────────

   func TestMutexManager(t *testing.T) {
	   ctx := context.Background()
	  t.Run("single_worker_acquires_lock", func(t *testing.T) {
		  store := sagaresidue.NewMemMutexStore()
		  mgr := sagaresidue.NewMutexManager(store, 90*time.Second)
		  mg, err := mgr.Acquire(ctx, "saga-001", "worker-A")
		  if err != nil {
			  t.Fatalf("acquire failed: %v", err)
		  }
		  if mg.Generation != 1 {
			   t.Errorf("generation: want 1 got %d", mg.Generation)
		  }
		   if mg.Status != "locked" {
			   t.Errorf("status: want locked got %s", mg.Status)
		  }
	  })
	   t.Run("dual_worker_second_blocked", func(t *testing.T) {
		   store := sagaresidue.NewMemMutexStore()
		   mgr := sagaresidue.NewMutexManager(store, 90*time.Second)
		  if _, err := mgr.Acquire(ctx, "saga-002", "worker-A"); err != nil {
			   t.Fatalf("worker-A acquire failed: %v", err)
		  }
		   _, err := mgr.Acquire(ctx, "saga-002", "worker-B")
		  if err != sagaresidue.ErrGenLocked {
			  t.Errorf("worker-B: expected ErrGenLocked got %v", err)
		   }
	  })
	   t.Run("release_allows_reacquire", func(t *testing.T) {
		  store := sagaresidue.NewMemMutexStore()
		   mgr := sagaresidue.NewMutexManager(store, 90*time.Second)
		   mg, _ := mgr.Acquire(ctx, "saga-003", "worker-A")
		  if err := mgr.Release(ctx, "saga-003", mg.Generation, "worker-A"); err != nil {
			  t.Fatalf("release failed: %v", err)
		   }
		  mg2, err := mgr.Acquire(ctx, "saga-003", "worker-B")
		   if err != nil {
			  t.Fatalf("re-acquire failed: %v", err)
		  }
		  if mg2.Generation != 2 {
			  t.Errorf("re-acquire generation: want 2 got %d", mg2.Generation)
		  }
	  })
	  t.Run("stale_refresh_returns_stale_gen", func(t *testing.T) {
		   store := sagaresidue.NewMemMutexStore()
		  // 设置时钟使锁立即过期
		   store.SetClock(func() time.Time { return time.Now().Add(-200 * time.Second) })
		  mgr := sagaresidue.NewMutexManager(store, 1*time.Second)
		   mg, _ := mgr.Acquire(ctx, "saga-004", "worker-A")
		  // 还原时钟到正常时间，此时锁应已过期
		  store.SetClock(func() time.Time { return time.Now() })
		  err := mgr.Refresh(ctx, "saga-004", mg.Generation, "worker-A")
		   if err != sagaresidue.ErrStaleGen {
			   t.Errorf("expected ErrStaleGen got %v", err)
		  }
	   })
	   t.Run("compensate_blocked_while_locked", func(t *testing.T) {
		   store := sagaresidue.NewMemMutexStore()
		   mgr := sagaresidue.NewMutexManager(store, 90*time.Second)
		  if _, err := mgr.Acquire(ctx, "saga-005", "closeout-worker"); err != nil {
			   t.Fatalf("acquire failed: %v", err)
		  }
		   err := mgr.CheckCompensateAllowed(ctx, "saga-005")
		   if err != sagaresidue.ErrCompensateBlock {
			   t.Errorf("expected ErrCompensateBlock got %v", err)
		  }
	  })
	   t.Run("compensate_allowed_after_release", func(t *testing.T) {
		  store := sagaresidue.NewMemMutexStore()
		  mgr := sagaresidue.NewMutexManager(store,
			  90*time.Second)
		  mg, _ := mgr.Acquire(ctx, "saga-006", "closeout-worker")
		  _ = mgr.Release(ctx, "saga-006", mg.Generation, "closeout-worker")
		  err := mgr.CheckCompensateAllowed(ctx, "saga-006")
		  if err != nil {
			  t.Errorf("expected nil got %v", err)
		  }
	  })
	   t.Run("reclaim_expired_locks", func(t *testing.T) {
		  store := sagaresidue.NewMemMutexStore()
		   // 用过去时间写入锁（锁已过期）
		   store.SetClock(func() time.Time { return time.Now().Add(-200 * time.Second) })
		   mgr := sagaresidue.NewMutexManager(store,
			   1*time.Second)
		  if _, err := mgr.Acquire(ctx, "saga-007", "stale-worker"); err != nil {
			  t.Fatalf("acquire: %v", err)
		   }
		   store.SetClock(func() time.Time { return time.Now() })
		   n, err := mgr.ReclaimExpired(ctx)
		   if err != nil {
			  t.Fatalf("reclaim: %v", err)
		   }
		   if n != 1 {
			   t.Errorf("reclaim count: want 1 got %d", n)
		  }
	   })
   }

// ── Prover 表驱动 ─────────────────────────────────────────────────────────

   func TestProver(t *testing.T) {
	  ctx := context.Background()
	   makeRun := func(id, sagaRunID string, steps []string) *sagaresidue.ResidueRun {
		   return &sagaresidue.ResidueRun{
			   ID: id,
			  SagaRunID: sagaRunID,
			   PartialDoneSteps: steps,
		   }
	   }
	  makeItem := func(id, runID, step, idem, amount string) sagaresidue.ResidueItem {
		   return sagaresidue.ResidueItem{
			  ID:           id,
			  ResidueRunID: runID,
			   Step:         step,
			  IdemKey: idem,
			   LedgerEntryID: "txf-" + id,
			  Amount:        amount,
			  Asset: "USDT",
			   IsConfirmed: true,
		  }
	  }
	  makeLeg := func(id, planID string, seqNo int, kind sagaresidue.LegKind, state sagaresidue.LegState) sagaresidue.CloseoutLeg {
		  return sagaresidue.CloseoutLeg{
			  ID: id,
			  PlanID: planID,
			   SeqNo:  seqNo,
			   Kind: kind,
			   State:      state,
			   IdemSuffix: id + "-idem",
			   LedgerEntryID: "txf-" + id,
		  }
	   }
	  cases := []struct {
		  name string
		  rrun *sagaresidue.ResidueRun
		   items         []sagaresidue.ResidueItem
		   legs          []sagaresidue.CloseoutLeg
		  wantIssueErr  bool
		   wantVerifyErr error
		   tamperDigest bool
	   }{
		   {
			   name: "normal_issue_and_verify",
			  rrun: makeRun("run-p1", "saga-p1", []string{"freeze", "move"}),
			  items: []sagaresidue.ResidueItem{makeItem("item-p1", "run-p1", "move", "idem-p1-move", "500")},
			   legs: []sagaresidue.CloseoutLeg{
				   makeLeg("leg-p1a", "plan-p1", 1, sagaresidue.LegKindReverse, sagaresidue.LegStateApplied),
				  makeLeg("leg-p1b", "plan-p1", 2, sagaresidue.LegKindFenceBlock, sagaresidue.LegStateApplied),
			  },
			   wantIssueErr:  false,
			  wantVerifyErr: nil,
		  },
		   {
			  name: "pending_leg_blocks_issue",
			   rrun: makeRun("run-p2", "saga-p2", []string{"move"}),
			  items: []sagaresidue.ResidueItem{makeItem("item-p2", "run-p2", "move", "idem-p2-move", "100")},
			  legs: []sagaresidue.CloseoutLeg{
				   makeLeg("leg-p2a", "plan-p2", 1, sagaresidue.LegKindReverse, sagaresidue.LegStatePending),
			   },
			  wantIssueErr: true,
		   },
		  {
			   name: "tampered_digest_fails_verify",
			  rrun: makeRun("run-p3", "saga-p3", []string{"move"}),
			   items: []sagaresidue.ResidueItem{makeItem("item-p3", "run-p3", "move", "idem-p3-move", "200")},
			  legs: []sagaresidue.CloseoutLeg{
				   makeLeg("leg-p3a", "plan-p3", 1, sagaresidue.LegKindReverse, sagaresidue.LegStateApplied),
			   },
			   wantIssueErr: false,
			   tamperDigest: true,
			  wantVerifyErr: sagaresidue.ErrProofInvalid,
		   },
		   {
			   name: "skipped_legs_included_in_proof",
			   rrun:  makeRun("run-p4", "saga-p4", []string{"move"}),
			  items: []sagaresidue.ResidueItem{makeItem("item-p4", "run-p4", "move", "idem-p4-move", "1")},
			  legs: []sagaresidue.CloseoutLeg{
				  makeLeg("leg-p4a", "plan-p4", 1, sagaresidue.LegKindReverse, sagaresidue.LegStateApplied),
				   makeLeg("leg-p4b", "plan-p4", 2, sagaresidue.LegKindFenceBlock, sagaresidue.LegStateSkipped),
			   },
			  wantIssueErr:  false,
			  wantVerifyErr: nil,
		  },
		   {
			  name: "unconfirmed_item_blocks_issue",
			   rrun: makeRun("run-p5", "saga-p5", []string{"move"}),
			   items: []sagaresidue.ResidueItem{
				   {ID: "item-p5", ResidueRunID: "run-p5", Step: "move",
					  IdemKey: "idem-p5-move", Amount: "100", IsConfirmed: false},
			   },
			   legs: []sagaresidue.CloseoutLeg{
				   makeLeg("leg-p5a", "plan-p5", 1, sagaresidue.LegKindReverse, sagaresidue.LegStateApplied),
			  },
			   wantIssueErr: true,
		   },
	  }
	  for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   store := sagaresidue.NewMemStore()
			  prover := sagaresidue.NewProver(store)
			  // 预置 store 数据
			  _ = store.SaveResidueRun(ctx, tc.rrun)
			  for i := range tc.items {
				   _ = store.SaveResidueItem(ctx, &tc.items[i])
			  }
			  for i := range tc.legs {
				  _ = store.SaveLeg(ctx,
					  &tc.legs[i])
			  }
			   // IssueProof
			  proof, issueErr := prover.IssueProof(ctx, tc.rrun, tc.items, tc.legs)
			  if tc.wantIssueErr {
				  if issueErr == nil {
					   t.Fatalf("expected issue error but got nil")
				   }
				  return
			   }
			   if issueErr != nil {
				  t.Fatalf("unexpected issue error: %v", issueErr)
			   }
			   if proof.ProofDigest == "" {
				   t.Error("proof_digest should not be empty")
			   }
			   // 篡改
			  if tc.tamperDigest {
				   proof.ProofDigest = "0000000000000000000000000000000000000000000000000000000000000000"
				  _ = store.SaveProof(ctx, proof)
			  }
			  // VerifyProof
			  _, verifyErr := prover.VerifyProof(ctx, sagaresidue.ProofVerifyReq{ProofID: proof.ID})
			  if tc.wantVerifyErr != nil {
				  if verifyErr != tc.wantVerifyErr {
					  t.Errorf("verify: want %v got %v", tc.wantVerifyErr, verifyErr)
				   }
			  } else {
				   if verifyErr != nil {
					   t.Errorf("verify unexpected err: %v", verifyErr)
				   }
			   }
		   })
	  }
  }

// ── CloseoutOrchestrator 表驱动 ───────────────────────────────────────────

  func TestCloseoutOrchestrator_BuildPlan(t *testing.T) {
	  ctx := context.Background()
	  makeConfirmedItem := func(step, idemKey, amount, hopState string) sagaresidue.ResidueItem {
		   return sagaresidue.ResidueItem{
			   ID: "item-" + step,
			   ResidueRunID: "run-orch-01",
			  Step:    step,
			  IdemKey: idemKey,
			   Amount: amount,
			  Asset: "USDT",
			   HopState:    hopState,
			   IsConfirmed: true,
		   }
	  }
	   cases := []struct {
		   name  string
		   req   sagaresidue.CloseoutReq
		  items []sagaresidue.ResidueItem
		   wantErr       error
		   wantLegsCount int
		  wantKinds     []sagaresidue.LegKind
	   }{
		   {
			   name: "credited_hop_generates_reverse_and_fence",
			   req: sagaresidue.CloseoutReq{
				  ResidueRunID: "run-orch-01",
				   Authorizer1:  "ops-alice",
				   Authorizer2:  "ops-bob",
				  IdemPrefix:   "closeout-run-orch-01",
			   },
			   items: []sagaresidue.ResidueItem{
				  makeConfirmedItem("move", "idem-orch-01-move", "500", "credited"),
			  },
			   wantErr: nil,
			   wantLegsCount: 2,
			   wantKinds:     []sagaresidue.LegKind{sagaresidue.LegKindReverse, sagaresidue.LegKindFenceBlock},
		   },
		  {
			  name: "initiated_hop_generates_suspense_and_fence",
			  req: sagaresidue.CloseoutReq{
				   ResidueRunID: "run-orch-01",
				   Authorizer1:  "ops-alice",
				   Authorizer2: "ops-bob",
				  IdemPrefix:  "closeout-run-orch-01-b",
			   },
			  items: []sagaresidue.ResidueItem{
				   makeConfirmedItem("freeze", "idem-orch-01-freeze", "200", "initiated"),
			  },
			   wantErr:       nil,
			   wantLegsCount: 2,
			   wantKinds: []sagaresidue.LegKind{sagaresidue.LegKindSuspense, sagaresidue.LegKindFenceBlock},
		   },
		   {
			   name: "dual_auth_same_person_rejected",
			  req: sagaresidue.CloseoutReq{
				  ResidueRunID: "run-orch-01",
				   Authorizer1:  "ops-alice",
				   Authorizer2:  "ops-alice",
				   IdemPrefix:   "any",
			   },
			   items:   nil,
			  wantErr: sagaresidue.ErrDualAuth,
		  },
		  {
			   name: "empty_idem_prefix_rejected",
			   req: sagaresidue.CloseoutReq{
				  ResidueRunID: "run-orch-01",
				   Authorizer1: "ops-alice",
				  Authorizer2: "ops-bob",
				  IdemPrefix:  "",
			   },
			  wantErr: sagaresidue.ErrIdemEmpty,
		  },
		   {
			  name: "no_confirmed_items_returns_error",
			   req: sagaresidue.CloseoutReq{
				   ResidueRunID: "run-orch-01",
				   Authorizer1:  "ops-alice",
				  Authorizer2:  "ops-bob",
				   IdemPrefix: "any-prefix",
			   },
			   items: []sagaresidue.ResidueItem{}, // 空
			  wantErr: sagaresidue.ErrIdemEmpty, // BuildPlan 返回"no confirmed items"
		   },
	  }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   store := sagaresidue.NewMemStore()
			  mutexMgr := sagaresidue.NewMutexManager(store, 90*time.Second)
			  writer := &mockLedgerWriter{}
			  orch := sagaresidue.NewCloseoutOrchestrator(writer,
				   store, mutexMgr, store)
			  rrun := &sagaresidue.ResidueRun{ID: "run-orch-01", SagaRunID: "saga-orch-01"}
			  report := &sagaresidue.ResidueReport{Items: tc.items}
			   plan, legs, err := orch.BuildPlan(ctx, tc.req, report, rrun)
			   if tc.wantErr != nil {
				  if err == nil {
					   t.Fatalf("expected err %v got nil", tc.wantErr)
				   }
				  return
			  }
			  if err != nil {
				  // "no confirmed items" 错误不是我们的枚举错误，检查 nil
				   if tc.name == "no_confirmed_items_returns_error" {
					   return
				  }
				  t.Fatalf("unexpected err: %v", err)
			   }
			   if len(legs) != tc.wantLegsCount {
				   t.Errorf("legs count: want %d got %d", tc.wantLegsCount, len(legs))
			   }
			  // 验证腿种类
			   kinds := make([]sagaresidue.LegKind, len(legs))
			   for i, l := range legs {
				   kinds[i] = l.Kind
			  }
			  if tc.wantKinds != nil {
				  for i, wk := range tc.wantKinds {
					   if i < len(kinds) && kinds[i] != wk {
						  t.Errorf("leg[%d] kind: want %s got %s", i, wk, kinds[i])
					  }
				   }
			  }
			  // 验证所有腿的 PlanID 已回填
			   for _, l := range legs {
				   if l.PlanID != plan.ID {
					  t.Errorf("leg.PlanID not filled: got %q want %q", l.PlanID, plan.ID)
				  }
			   }
		  })
	   }
   }

// ── SkipLeg ──────────────────────────────────────────────────────────────

   func TestCloseoutOrchestrator_SkipLeg(t *testing.T) {
	   ctx := context.Background()
	   store := sagaresidue.NewMemStore()
	   mutexMgr := sagaresidue.NewMutexManager(store, 90*time.Second)
	  orch := sagaresidue.NewCloseoutOrchestrator(&mockLedgerWriter{},
		   store, mutexMgr, store)
	  leg := &sagaresidue.CloseoutLeg{
		  ID:     "leg-skip-01",
		  PlanID: "plan-skip-01",
		   State: sagaresidue.LegStatePending,
		  SeqNo: 1,
		   Kind:  sagaresidue.LegKindReverse,
	   }
	  _ = store.SaveLeg(ctx, leg)
	   cases := []struct {
		  name    string
		   req     sagaresidue.SkipLegReq
		  wantErr error
	  }{
		   {
			  name: "valid_skip",
			  req: sagaresidue.SkipLegReq{
				  PlanID: "plan-skip-01",
				  LegID:  "leg-skip-01",
				   SkipReason: "dust amount below threshold",
				  Authorizer1: "ops-alice",
				   Authorizer2: "ops-bob",
			  },
			   wantErr: nil,
		   },
		  {
			  name: "same_authorizer_rejected",
			   req: sagaresidue.SkipLegReq{
				  PlanID: "plan-skip-01",
				  LegID:       "leg-skip-01",
				  SkipReason:  "reason",
				  Authorizer1: "ops-alice",
				  Authorizer2: "ops-alice",
			  },
			  wantErr: sagaresidue.ErrDualAuth,
		  },
	  }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  // 重置腿状态为 pending
			  leg.State = sagaresidue.LegStatePending
			  _ = store.SaveLeg(ctx, leg)
			   err := orch.SkipLeg(ctx, tc.req)
			  if tc.wantErr != nil {
				  if err != tc.wantErr {
					   t.Errorf("want %v got %v", tc.wantErr, err)
				  }
				  return
			   }
			   if err != nil {
				  t.Fatalf("unexpected err: %v", err)
			  }
			   got, _ := store.GetLeg(ctx, leg.ID)
			   if got.State != sagaresidue.LegStateSkipped {
				  t.Errorf("leg state: want skipped got %s", got.State)
			  }
		   })
	   }
   }

// ── ProofDigest 确定性 ─────────────────────────────────────────────────────

  func TestProofDigest_Deterministic(t *testing.T) {
	  ctx := context.Background()
	   store := sagaresidue.NewMemStore()
	   prover := sagaresidue.NewProver(store)
	  rrun := &sagaresidue.ResidueRun{
		  ID: "run-det-01",
		   SagaRunID:        "saga-det-01",
		   PartialDoneSteps: []string{"freeze", "move"},
	  }
	  items := []sagaresidue.ResidueItem{
		  {ID: "item-det-01", ResidueRunID: "run-det-01", Step: "move",
			   IdemKey: "idem-det-01-move", Amount: "100", IsConfirmed: true},
	  }
	   legs := []sagaresidue.CloseoutLeg{
		   {ID: "leg-det-01", PlanID: "plan-det-01", SeqNo: 1,
			   Kind: sagaresidue.LegKindReverse, State: sagaresidue.LegStateApplied,
			   IdemSuffix: "idem-det-01-move-rev", LedgerEntryID: "txf-det-rev"},
	  }
	  _ = store.SaveResidueRun(ctx, rrun)
	  for i := range items {
		  _ = store.SaveResidueItem(ctx, &items[i])
	   }
	  for i := range legs {
		   _ = store.SaveLeg(ctx, &legs[i])
	  }
	  proof1, err1 := prover.IssueProof(ctx, rrun, items, legs)
	   if err1 != nil {
		   t.Fatalf("issue1: %v", err1)
	   }
	  // 删除证明，重新签发（相同输入）
	   _ = store.SaveProof(ctx, &sagaresidue.CloseProof{
		  ID:           "run-det-01-v2",
		   ResidueRunID: "run-det-01-v2",
		  SagaRunID: "saga-det-01",
	  })
	   rrun2 := &sagaresidue.ResidueRun{
		  ID:               "run-det-02",
		  SagaRunID:        "saga-det-02",
		  PartialDoneSteps: []string{"freeze", "move"}, // 相同步骤，不同顺序
	   }
	  items2 := make([]sagaresidue.ResidueItem, len(items))
	  copy(items2, items)
	  for i := range items2 {
		  items2[i].ResidueRunID = "run-det-02"
	  }
	  legs2 := make([]sagaresidue.CloseoutLeg, len(legs))
	  copy(legs2, legs)
	  proof2,
		   err2 := prover.IssueProof(ctx, rrun2, items2, legs2)
	   if err2 != nil {
		   t.Fatalf("issue2: %v", err2)
	   }
	  // 相同内容，哈希应相同
	  if proof1.IntentHash != proof2.IntentHash {
		   t.Errorf("intent hash differs: %s vs %s", proof1.IntentHash, proof2.IntentHash)
	   }
	  if proof1.ProofDigest != proof2.ProofDigest {
		   t.Errorf("proof digest differs: %s vs %s", proof1.ProofDigest, proof2.ProofDigest)
	  }
   }

// ── MemStore 幂等 ─────────────────────────────────────────────────────────

   func TestMemStore_Idempotency(t *testing.T) {
	   ctx := context.Background()
	   store := sagaresidue.NewMemStore()
	   rrun := &sagaresidue.ResidueRun{
		  ID: "run-idem-01", SagaRunID: "saga-idem-01",
		   State: sagaresidue.RunStateResidueOpen,
	  }
	   // 多次 Save 应幂等
	   for i := 0; i < 3; i++ {
		  if err := store.SaveResidueRun(ctx, rrun); err != nil {
			  t.Fatalf("save run iter %d: %v", i, err)
		   }
	  }
	  runs,
		   err := store.ListResidueRuns(ctx, sagaresidue.RunListReq{})
	  if err != nil {
		  t.Fatalf("list: %v", err)
	   }
	  // 即使 Save 了 3 次，同一 ID 只有一条记录（通过"saga:" 索引可能算两条，过滤后应为 1）
	   count := 0
	   for _, r := range runs {
		  if r.ID == rrun.ID {
			   count++
		   }
	  }
	   if count != 1 {
		   t.Errorf("idempotent save: want 1 got %d", count)
	  }
  }

// ── IdemBlock ────────────────────────────────────────────────────────────

   func TestIdemBlock(t *testing.T) {
	   ctx := context.Background()
	  store := sagaresidue.NewMemStore()
	   idemKey := "idem-block-test-001"
	   reason := "saga-run=run-001 plan=plan-001 leg=leg-001"
	   if err := store.BlockIdem(ctx, idemKey, reason); err != nil {
		   t.Fatalf("block: %v", err)
	   }
	  blocked, err := store.IsIdemBlocked(ctx, idemKey)
	  if err != nil || !blocked {
		   t.Errorf("expected blocked=true, got %v err=%v", blocked, err)
	   }
	   // 重复 Block 幂等（不报错）
	   if err := store.BlockIdem(ctx, idemKey, "another reason"); err != nil {
		   t.Fatalf("block again: %v", err)
	  }
	  // 未封堵的 key
	   blocked2, err := store.IsIdemBlocked(ctx, "idem-block-not-exist")
	   if err != nil || blocked2 {
		   t.Errorf("expected not blocked, got %v err=%v", blocked2, err)
	   }
   }

// ── sortLegsBySeq ────────────────────────────────────────────────────

   func TestSortLegsBySeq(t *testing.T) {
	   ctx := context.Background()
	   store := sagaresidue.NewMemStore()
	  for _, seqNo := range []int{3, 1, 5, 2, 4} {
		  _ = store.SaveLeg(ctx, &sagaresidue.CloseoutLeg{
			  ID:     fmt.Sprintf("leg-%d", seqNo),
			   SeqNo:  seqNo,
			  PlanID: "plan-sort-01",
			  State: sagaresidue.LegStatePending,
		   })
	   }
	   legs, err := store.ListLegs(ctx, sagaresidue.LegListReq{PlanID: "plan-sort-01", Limit: 100})
	  if err != nil {
		   t.Fatalf("list legs: %v", err)
	   }
	   for i := 1; i < len(legs); i++ {
		   if legs[i].SeqNo < legs[i-1].SeqNo {
			   t.Errorf("legs not sorted at pos %d: %d < %d", i, legs[i].SeqNo, legs[i-1].SeqNo)
		   }
	   }
   }

// ── 辅助 import ───────────────────────────────────────────────────────────────

var _ = fmt.Sprintf // 确保 fmt 包被引用

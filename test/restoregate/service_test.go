package restoregate_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/restoregate"
)

  func newSvc() *restoregate.Service {
	  return restoregate.NewService(restoregate.NewMemStore())
  }

  func makeRequest(blobID string, targetGen, liveWmark int64, balRoot, creator string) restoregate.CreatePlanRequest {
	  return restoregate.CreatePlanRequest{
		   BlobID:    blobID,
		   TargetGen: targetGen,
		   LiveWatermark: liveWmark,
		  BalanceRootPre: balRoot,
		   Creator: creator,
		   Reason: "unit test",
	  }
  }

// ── FSM 合法转换 ─────────────────────────────────────────────────────────────

   func TestFSM_ValidTransitions(t *testing.T) {
	  m := restoregate.NewFSM()
	   steps := []restoregate.PlanState{
		  restoregate.StateFreezing,
		  restoregate.StateImporting,
		  restoregate.StateVerifying,
		   restoregate.StateSealed,
	  }
	   for _, next := range steps {
		   if err := m.Transition(next); err != nil {
			  t.Fatalf("transition to %s failed: %v", next, err)
		   }
	   }
	  if m.State() != restoregate.StateSealed {
		   t.Fatalf("expected sealed, got %s", m.State())
	  }
   }

   func TestFSM_InvalidTransition(t *testing.T) {
	   m := restoregate.NewFSM() // planned
	   err := m.Transition(restoregate.StateVerifying)
	   if !errors.Is(err, restoregate.ErrInvalidState) {
		  t.Fatalf("expected ErrInvalidState, got %v", err)
	   }
  }

   func TestFSM_AbortAnyNonSealed(t *testing.T) {
	   states := []restoregate.PlanState{
		  restoregate.StatePlanned,
		  restoregate.StateFreezing,
		  restoregate.StateImporting,
		  restoregate.StateVerifying,
	  }
	  for _, s := range states {
		  m := restoregate.NewFSMFrom(s)
		   if err := m.Abort(); err != nil {
			  t.Fatalf("abort from %s: %v", s, err)
		   }
		   if m.State() != restoregate.StateAborted {
			   t.Fatalf("expected aborted, got %s", m.State())
		   }
	  }
   }

   func TestFSM_SealedCannotAbort(t *testing.T) {
	   m := restoregate.NewFSMFrom(restoregate.StateSealed)
	  if err := m.Abort(); !errors.Is(err, restoregate.ErrPlanSealed) {
		   t.Fatalf("expected ErrPlanSealed, got %v", err)
	  }
  }

// ── CreatePlan 参数校验 ───────────────────────────────────────────────────────

   func TestCreatePlan_Validation(t *testing.T) {
	  cases := []struct {
		  name string
		  req  restoregate.CreatePlanRequest
		   wantErr error
	   }{
		  {
			   name:    "missing blob_id",
			  req:     makeRequest("", 10, 5, "root", "ops"),
			   wantErr: restoregate.ErrEmpty,
		   },
		   {
			   name: "missing creator",
			   req:  makeRequest("blob", 10, 5, "root", ""),
			   wantErr: restoregate.ErrEmpty,
		  },
		  {
			  name:    "zero target_gen",
			   req:     makeRequest("blob", 0, 0, "root", "ops"),
			   wantErr: restoregate.ErrEmpty,
		   },
		  {
			   name: "gen conflict on create",
			  req:  makeRequest("blob", 5, 10, "root", "ops"),
			  wantErr: restoregate.ErrGenConflict,
		   },
		   {
			  name: "valid",
			  req:  makeRequest("blob", 10, 5, "root", "ops"),
			  wantErr: nil,
		  },
	   }
	  for _, tc := range cases {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  svc := newSvc()
			   _, err := svc.CreatePlan(context.Background(), tc.req)
			  if !errors.Is(err, tc.wantErr) {
				   t.Fatalf("want %v got %v", tc.wantErr, err)
			   }
		  })
	  }
  }

// ── WriteMutex 冲突 ──────────────────────────────────────────────────────────

   func TestWriteMutex_Conflict(t *testing.T) {
	  store := restoregate.NewMemStore()
	  guard := restoregate.NewWriteGuard(store)
	   ctx := context.Background()
	   // Plan A 申请锁
	  _, err := guard.Acquire(ctx, "plan-A", 10)
	   if err != nil {
		  t.Fatalf("acquire plan-A: %v", err)
	  }
	   // Plan B 申请锁应冲突
	  _, err = guard.Acquire(ctx, "plan-B", 11)
	  if !errors.Is(err, restoregate.ErrMutexConflict) {
		  t.Fatalf("expected ErrMutexConflict, got %v", err)
	  }
	  // 写路径应被阻断
	  if err := guard.CheckWrite(ctx); !errors.Is(err, restoregate.ErrWriteFrozen) {
		   t.Fatalf("expected ErrWriteFrozen, got %v", err)
	   }
	  // 释放后写路径恢复
	   if err := guard.Release(ctx, "plan-A"); err != nil {
		   t.Fatalf("release: %v", err)
	  }
	  if err := guard.CheckWrite(ctx); err != nil {
		   t.Fatalf("expected write allowed after release, got %v", err)
	   }
  }

   func TestWriteMutex_Idempotent(t *testing.T) {
	   store := restoregate.NewMemStore()
	   guard := restoregate.NewWriteGuard(store)
	  ctx := context.Background()
	   m1, err := guard.Acquire(ctx, "plan-X", 1)
	  if err != nil {
		  t.Fatalf("first acquire: %v", err)
	  }
	  m2, err := guard.Acquire(ctx, "plan-X", 1) // 幂等
	  if err != nil {
		  t.Fatalf("second acquire: %v", err)
	  }
	   if m1.ID != m2.ID {
		   t.Fatalf("expected same mutex ID on idempotent acquire, got %s vs %s", m1.ID, m2.ID)
	   }
   }

   func TestWriteMutex_Expired(t *testing.T) {
	  store := restoregate.NewMemStore()
	   now := time.Now().UTC()
	  guard := restoregate.NewWriteGuard(store).
		  WithTTL(1 * time.Millisecond).
		   WithClock(func() time.Time { return now })
	   _, _ = guard.Acquire(context.Background(), "plan-Y", 1)
	   // 推进时钟超过 TTL
	  guard2 := restoregate.NewWriteGuard(store).
		  WithClock(func() time.Time { return now.Add(1 * time.Hour) })
	  // 过期后写路径应畅通
	   if err := guard2.CheckWrite(context.Background()); err != nil {
		   t.Fatalf("expected write allowed after TTL expiry, got %v", err)
	  }
   }

// ── HalfSeal ─────────────────────────────────────────────────────────────────

   func TestHalfSeal_CreateAndResume(t *testing.T) {
	   store := restoregate.NewMemStore()
	   mgr := restoregate.NewHalfSealManager(store)
	   ctx := context.Background()
	  keys := []string{"account|u1|BTC", "account|u2|USDT"}
	  if err := mgr.Seal(ctx, "plan-1", keys, "test failure"); err != nil {
		  t.Fatalf("Seal: %v", err)
	   }
	   // 不可重封
	   if err := mgr.Seal(ctx, "plan-1", nil, "again"); !errors.Is(err, restoregate.ErrHalfSealExists) {
		  t.Fatalf("expected ErrHalfSealExists, got %v", err)
	  }
	  // CheckResumable 返回 ErrHalfSealExists
	  if err := mgr.CheckResumable(ctx, "plan-1"); !errors.Is(err, restoregate.ErrHalfSealExists) {
		   t.Fatalf("expected ErrHalfSealExists, got %v", err)
	   }
	   // 续封印
	  if err := mgr.Resume(ctx, "plan-1"); err != nil {
		   t.Fatalf("Resume: %v", err)
	   }
	   if err := mgr.CheckResumable(ctx, "plan-1"); err != nil {
		  t.Fatalf("after resume, expected resumable: %v", err)
	  }
	   // 幂等续
	  if err := mgr.Resume(ctx, "plan-1"); err != nil {
		  t.Fatalf("idempotent Resume: %v", err)
	   }
  }

  func TestHalfSeal_NoSealAllowsImport(t *testing.T) {
	   store := restoregate.NewMemStore()
	   mgr := restoregate.NewHalfSealManager(store)
	  ctx := context.Background()
	   // 无封印记录 → 可 Import
	   if err := mgr.CheckResumable(ctx, "plan-new"); err != nil {
		  t.Fatalf("expected nil for no seal, got %v", err)
	   }
   }

// ── SplitBrainGuard ──────────────────────────────────────────────────────────

  func TestSplitBrain_Open(t *testing.T) {
	   store := restoregate.NewMemStore()
	  sb := restoregate.NewSplitBrainGuard(store)
	   ctx := context.Background()
	  err := sb.CheckAndOpen(ctx, "plan-1", "pre-root", "target-root", "wrong-root", "BTC mismatch")
	   if !errors.Is(err, restoregate.ErrChecksumMismatch) {
		   t.Fatalf("expected ErrChecksumMismatch, got %v", err)
	  }
	   proofs, err := sb.ListOpenProofs(ctx)
	  if err != nil {
		   t.Fatalf("ListOpenProofs: %v", err)
	   }
	   if len(proofs) != 1 {
		   t.Fatalf("expected 1 proof, got %d", len(proofs))
	  }
   }

  func TestSplitBrain_MatchNoProof(t *testing.T) {
	   store := restoregate.NewMemStore()
	  sb := restoregate.NewSplitBrainGuard(store)
	  ctx := context.Background()
	   err := sb.CheckAndOpen(ctx, "plan-1", "same-root", "same-root", "same-root", "")
	   if err != nil {
		   t.Fatalf("expected nil for matching roots, got %v", err)
	   }
	  proofs, _ := sb.ListOpenProofs(ctx)
	   if len(proofs) != 0 {
		   t.Fatal("expected no proofs for matching root")
	  }
   }

   func TestSplitBrain_ApproveClears(t *testing.T) {
	  store := restoregate.NewMemStore()
	   sb := restoregate.NewSplitBrainGuard(store)
	  ctx := context.Background()
	  _ = sb.CheckAndOpen(ctx, "plan-1", "pre", "target", "post", "mismatch")
	   proofs, _ := sb.ListOpenProofs(ctx)
	  proofID := proofs[0].ID
	   _ = sb.Approve(ctx, proofID, "alice", "ops", "ok")
	  _ = sb.Approve(ctx, proofID, "bob", "risk", "ok")
	   proof, _ := sb.ExportProof(ctx, proofID)
	   if proof.State != restoregate.SplitBrainCleared {
		  t.Fatalf("expected cleared, got %s", proof.State)
	  }
   }

// ── 表格驱动：GenerationGate 值 ─────────────────────────────────────────

   func TestGenerationGate_TableDriven(t *testing.T) {
	   cases := []struct {
		   systemGen int64
		  targetGen int64
		  liveWmark int64
		   allowed bool
		  wantErr error
	   }{
		  {0, 1, 0, true, nil},
		   {5, 10, 8, true, nil},
		   {5, 10, 15, false, restoregate.ErrGenConflict},
		   {10, 5, 3, false, restoregate.ErrGenLag},
		  {10, 10, 9, true, nil},
		   {0, 100, 99, true, nil},
		  {99, 100, 100, true, nil}, // live == target: not a conflict (strictly >, not >=)
	   }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(fmt.Sprintf("sys=%d tgt=%d live=%d", tc.systemGen, tc.targetGen, tc.liveWmark), func(t *testing.T) {
			  store := restoregate.NewMemStore()
			  _ = store.UpsertGenRecord(context.Background(), restoregate.RestoreGenRecord{
				  ID:         "global",
				  CurrentGen: tc.systemGen,
				   UpdatedAt: time.Now().UTC(),
			  })
			  gate := restoregate.NewGenerationGate(store)
			   err := gate.CheckAndRecord(context.Background(), "plan-x", tc.targetGen, tc.liveWmark)
			   if !errors.Is(err, tc.wantErr) {
				   t.Fatalf("want %v got %v", tc.wantErr, err)
			   }
			  gates, _ := store.ListGenGates(context.Background(), "plan-x")
			  if len(gates) != 1 {
				  t.Fatalf("expected 1 gate record, got %d", len(gates))
			   }
			  if gates[0].Allowed != tc.allowed {
				   t.Fatalf("gate.Allowed: want %v got %v", tc.allowed, gates[0].Allowed)
			  }
		  })
	   }
   }

// ── BalanceRoot 计算正确性 ────────────────────────────────────────────────────

  func TestBalanceRoot_Empty(t *testing.T) {
	  root := restoregate.BalanceRoot(nil)
	   if root != "" {
		  t.Fatalf("expected empty root for nil entries, got %q", root)
	   }
   }

  func TestBalanceRoot_OrderIndependent(t *testing.T) {
	   e1 := map[string]string{"a|B|C": "1|0", "d|E|F": "2|0"}
	  e2 := map[string]string{"d|E|F": "2|0", "a|B|C": "1|0"}
	  if restoregate.BalanceRoot(e1) != restoregate.BalanceRoot(e2) {
		   t.Fatal("BalanceRoot should be order-independent")
	   }
  }

   func TestBalanceRoot_DifferentValues(t *testing.T) {
	   e1 := map[string]string{"u|S|BTC": "1.0|0.0"}
	   e2 := map[string]string{"u|S|BTC": "2.0|0.0"}
	  if restoregate.BalanceRoot(e1) == restoregate.BalanceRoot(e2) {
		   t.Fatal("different values should produce different roots")
	   }
  }

// ── Service 幂等操作 ──────────────────────────────────────────────────────────

   func TestService_AbortIdempotent(t *testing.T) {
	   svc := newSvc()
	   ctx := context.Background()
	  p, _ := svc.CreatePlan(ctx, makeRequest("b", 10, 5, "r", "ops"))
	  _, err := svc.AbortPlan(ctx, p.ID, "test")
	   if err != nil {
		  t.Fatalf("first abort: %v", err)
	  }
	  _, err = svc.AbortPlan(ctx, p.ID, "again")
	   if !errors.Is(err, restoregate.ErrPlanAborted) {
		  t.Fatalf("expected ErrPlanAborted on second abort, got %v", err)
	   }
  }

  func TestService_ListPlans(t *testing.T) {
	   svc := newSvc()
	  ctx := context.Background()
	  for i := 0; i < 5; i++ {
		   _, err := svc.CreatePlan(ctx, makeRequest(fmt.Sprintf("blob-%d", i), int64(i+1), 0, "r", "ops"))
		   if err != nil {
			   t.Fatalf("CreatePlan %d: %v", i, err)
		   }
	   }
	   plans,
		   err := svc.ListPlans(ctx, 3)
	   if err != nil {
		  t.Fatalf("ListPlans: %v", err)
	   }
	   if len(plans) != 3 {
		   t.Fatalf("expected 3 (limited), got %d", len(plans))
	   }
   }

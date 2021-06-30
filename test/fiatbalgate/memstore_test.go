package fiatbalgate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/fiatbalgate"
)

  func newStore(t *testing.T) *fiatbalgate.MemStore {
	   t.Helper()
	  return fiatbalgate.NewMemStore()
   }

var testBase = time.Date(2025, 11, 8, 10, 0, 0, 0, time.UTC)

// ─── JournalGeneration ────────────────────────────────────────────────────────

  func TestMemStore_SaveGeneration(t *testing.T) {
	   store := newStore(t)
	  ctx := context.Background()
	  g := &fiatbalgate.JournalGeneration{
		   GenID: "gen-001",
		   Seq: 1,
		   EntryID: "entry-001",
		  Kind:    "post",
		  Asset: "USDT",
		  BumpedAt: testBase,
		   SyncedAt: testBase,
	   }
	   if err := store.SaveGeneration(ctx, g); err != nil {
		   t.Fatalf("save gen: %v", err)
	   }
	   // 幂等
	   if err := store.SaveGeneration(ctx, g); err != nil {
		   t.Fatalf("idempotent save gen: %v", err)
	   }
	  got, err := store.GetGenerationBySeq(ctx, 1)
	  if err != nil {
		  t.Fatalf("get gen: %v", err)
	   }
	  if got.EntryID != "entry-001" {
		   t.Errorf("want entry-001 got %s", got.EntryID)
	   }
	  // 未找到
	  _, err = store.GetGenerationBySeq(ctx, 999)
	   if !errors.Is(err, fiatbalgate.ErrGenNotFound) {
		  t.Errorf("want ErrGenNotFound got %v", err)
	   }
	  maxSeq, err := store.MaxGenSeq(ctx)
	   if err != nil {
		  t.Fatalf("max gen seq: %v", err)
	  }
	  if maxSeq != 1 {
		  t.Errorf("want 1 got %d", maxSeq)
	   }
   }

// ─── FiatImbalanceCase ────────────────────────────────────────────────────────

   func TestMemStore_CaseCRUD(t *testing.T) {
	  store := newStore(t)
	  ctx := context.Background()
	  now := testBase
	  c := &fiatbalgate.FiatImbalanceCase{
		   CaseID: "case-001",
		  Asset: "USDT",
		   ScanGenSeq:      1,
		  DetectedEntryID: "entry-001",
		  SumDebit:        "100",
		  SumCredit: "90",
		   ImbalanceKind: fiatbalgate.ImbalanceBalancedMismatch,
		  State:         fiatbalgate.CaseOpen,
		  ScannerID:     "scanner-01",
		   OpenedAt:      now,
		  CreatedAt:     now,
		   UpdatedAt:     now,
	  }
	  if err := store.SaveCase(ctx, c); err != nil {
		   t.Fatalf("save case: %v", err)
	   }
	   got, err := store.GetCase(ctx, "case-001")
	  if err != nil {
		   t.Fatalf("get case: %v", err)
	   }
	  if got.State != fiatbalgate.CaseOpen {
		  t.Errorf("want open got %s", got.State)
	  }
	  // CAS open→blocking
	   blockAt := now.Add(10 * time.Second)
	   patch := &fiatbalgate.FiatImbalanceCase{BlockingAt: &blockAt}
	   if err := store.UpdateCaseState(ctx, "case-001", fiatbalgate.CaseOpen, fiatbalgate.CaseBlocking, patch); err != nil {
		   t.Fatalf("update state: %v", err)
	   }
	  // 重复 CAS（当前是 blocking，不是 open）→ ErrInvalidTransition
	   if err := store.UpdateCaseState(ctx, "case-001", fiatbalgate.CaseOpen, fiatbalgate.CaseBlocking, nil); !errors.Is(err, fiatbalgate.ErrInvalidTransition) {
		  t.Errorf("want ErrInvalidTransition got %v", err)
	   }
	  // ListOpenOrBlockingCases
	  list, err := store.ListOpenOrBlockingCases(ctx, "USDT", 10)
	  if err != nil {
		   t.Fatalf("list cases: %v", err)
	   }
	   if len(list) != 1 {
		  t.Errorf("want 1 got %d", len(list))
	   }
	   // HasActiveBlockingForAsset
	   has, err := store.HasActiveBlockingForAsset(ctx, "USDT")
	   if err != nil {
		  t.Fatalf("has blocking: %v", err)
	  }
	  if !has {
		   t.Error("want has blocking=true")
	   }
	  // 未找到
	  _, err = store.GetCase(ctx, "no-such")
	  if !errors.Is(err, fiatbalgate.ErrCaseNotFound) {
		   t.Errorf("want ErrCaseNotFound got %v", err)
	   }
   }

// ─── FiatImbalanceProof ───────────────────────────────────────────────────────

   func TestMemStore_ProofCRUD(t *testing.T) {
	  store := newStore(t)
	  ctx := context.Background()
	   now := testBase
	   p := &fiatbalgate.FiatImbalanceProof{
		   ProofID: "proof-001",
		  CaseID:          "case-001",
		  Asset:           "USDT",
		  ScanGenSeq:      1,
		   EntriesSnapshot: `["e1"]`,
		   LinesSnapshot: `[]`,
		  SumDebit:      "100",
		  SumCredit: "90",
		   DigestSHA256: "abcd1234",
		   State: fiatbalgate.ProofPending,
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	   if err := store.SaveProof(ctx, p); err != nil {
		   t.Fatalf("save proof: %v", err)
	  }
	   // 重复写同 case_id → ErrAlreadyProven
	   if err := store.SaveProof(ctx, p); !errors.Is(err, fiatbalgate.ErrAlreadyProven) {
		  t.Errorf("dup proof want ErrAlreadyProven got %v", err)
	   }
	  got, err := store.GetProofByCase(ctx, "case-001")
	  if err != nil {
		  t.Fatalf("get proof by case: %v", err)
	   }
	  if got.ProofID != "proof-001" {
		  t.Errorf("want proof-001 got %s", got.ProofID)
	   }
	   // 推进状态
	  expAt := now.Add(5 * time.Second)
	   if err := store.UpdateProofState(ctx, "proof-001", fiatbalgate.ProofPending, fiatbalgate.ProofExported, &fiatbalgate.FiatImbalanceProof{ExportedAt: &expAt}); err != nil {
		   t.Fatalf("update proof state: %v", err)
	  }
	  // 状态不符 → ErrInvalidTransition
	  if err := store.UpdateProofState(ctx, "proof-001", fiatbalgate.ProofPending, fiatbalgate.ProofExported, nil); !errors.Is(err, fiatbalgate.ErrInvalidTransition) {
		   t.Errorf("want ErrInvalidTransition got %v", err)
	  }
   }

// ─── ClearApproval ────────────────────────────────────────────────────────────

   func TestMemStore_ApprovalList(t *testing.T) {
	  store := newStore(t)
	   ctx := context.Background()
	   a1 := &fiatbalgate.ClearApproval{
		   ApprovalID: "app-001",
		  CaseID:     "case-001",
		  Step:  fiatbalgate.ClearStepFirst,
		  Actor: "ops-alice",
		   Note:  "第一眼清障审核",
		   CreatedAt: testBase,
	  }
	   a2 := &fiatbalgate.ClearApproval{
		   ApprovalID: "app-002",
		  CaseID:     "case-001",
		  Step:      fiatbalgate.ClearStepDone,
		  Actor:     "ops-bob",
		  Note:      "第二眼清障授权",
		  CreatedAt: testBase.Add(5 * time.Second),
	   }
	  _ = store.SaveApproval(ctx, a1)
	   _ = store.SaveApproval(ctx, a2)
	   approvals, err := store.ListApprovalsByCase(ctx, "case-001")
	   if err != nil {
		   t.Fatalf("list approvals: %v", err)
	   }
	  if len(approvals) != 2 {
		   t.Errorf("want 2 approvals got %d", len(approvals))
	   }
	   // 空 case_id 返回空列表
	   empty, err := store.ListApprovalsByCase(ctx, "no-such-case")
	  if err != nil {
		  t.Fatalf("list empty approvals: %v", err)
	  }
	  if len(empty) != 0 {
		   t.Errorf("want 0 got %d", len(empty))
	   }
  }

// ─── ListExpiredOpenCases ─────────────────────────────────────────────────────

  func TestMemStore_ListExpiredOpenCases(t *testing.T) {
	  store := newStore(t)
	   ctx := context.Background()
	  old := testBase.Add(-5 * time.Minute)
	   recent := testBase.Add(-10 * time.Second)
	   oldCase := &fiatbalgate.FiatImbalanceCase{
		   CaseID: "case-old", Asset: "USDT", State: fiatbalgate.CaseOpen,
		  ScannerID: "s", ImbalanceKind: fiatbalgate.ImbalanceBalancedMismatch,
		   OpenedAt: old, CreatedAt: old, UpdatedAt: old,
	   }
	  recentCase := &fiatbalgate.FiatImbalanceCase{
		  CaseID: "case-recent", Asset: "BTC", State: fiatbalgate.CaseOpen,
		  ScannerID: "s", ImbalanceKind: fiatbalgate.ImbalanceBalancedMismatch,
		  OpenedAt: recent, CreatedAt: recent, UpdatedAt: recent,
	  }
	   _ = store.SaveCase(ctx, oldCase)
	  _ = store.SaveCase(ctx, recentCase)
	   // threshold = 1 分钟前，只有 old 应被返回
	  threshold := testBase.Add(-1 * time.Minute)
	   expired,
		  err := store.ListExpiredOpenCases(ctx, threshold, 10)
	  if err != nil {
		   t.Fatalf("list expired: %v", err)
	  }
	  if len(expired) != 1 || expired[0].CaseID != "case-old" {
		  t.Errorf("want [case-old] got %v", expired)
	  }
   }

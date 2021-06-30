package flow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/brmismatch"
	"account-assets.kucoin.net/funding/internal/stablebr"
)

// ─── 辅助 ────────────────────────────────────────────────────────────────────
// newStablebrWithChecks 构造一个已通过全部证明的 stablebr.Service。
  func newStablebrWithChecks(t *testing.T, kind, asset string, now time.Time) *stablebr.Service {
	  t.Helper()
	   svc := stablebr.New()
	  svc.SetClock(func() time.Time { return now })
	  items := map[string][]stablebr.Item{
		  "mint":   {stablebr.ItemReserve, stablebr.ItemIssuer, stablebr.ItemBank, stablebr.ItemKYT},
		  "redeem": {stablebr.ItemReserve, stablebr.ItemIssuer, stablebr.ItemBurn, stablebr.ItemKYT},
	  }
	  for _, item := range items[kind] {
		  _, err := svc.Attest(stablebr.AttestReq{
			   Kind: kind,
			   Asset: asset,
			  Item:   string(item),
			  Actor:  "ops-alice",
			   TTLSec: 3600,
		   })
		  if err != nil {
			   t.Fatalf("attest %s: %v", item, err)
		  }
	   }
	  return svc
   }

  func newMismatchSvc() *brmismatch.Service {
	  return brmismatch.New()
   }

// ─── 场景 1–2：AttestSnapshot TTL ────────────────────────────────────────

   func TestAttestSnapshotTTLEdgeFailClosed(t *testing.T) {
	  base := time.Date(2026, 9, 21, 19, 0, 0, 0, time.UTC)
	   svc := newStablebrWithChecks(t, "mint", "USDT", base)
	   snap, err := svc.SnapshotAttest("mint", "USDT")
	  if err != nil {
		   t.Fatalf("snapshot: %v", err)
	   }
	  if snap.ExpiresAt.IsZero() {
		  t.Fatal("ExpiresAt must not be zero")
	   }
	   // 精确到期时刻：ExpiresAt.After(now) = false → fail-closed
	   exactExpiry := snap.ExpiresAt
	   if snap.Valid(exactExpiry) {
		   t.Fatal("snapshot must be invalid at exact expiry (fail-closed)")
	   }
	   // 到期前 1ns：仍有效
	  justBefore := exactExpiry.Add(-1)
	   if !snap.Valid(justBefore) {
		  t.Fatal("snapshot must be valid 1ns before expiry")
	  }
  }

  func TestAttestSnapshotValidBeforeExpiry(t *testing.T) {
	   base := time.Date(2026, 9, 21, 19, 0, 0, 0, time.UTC)
	   svc := newStablebrWithChecks(t, "mint", "USDC", base)
	   snap, err := svc.SnapshotAttest("mint", "USDC")
	   if err != nil {
		  t.Fatalf("snapshot: %v", err)
	  }
	   // 当前时刻（base）应有效
	   svc.SetClock(func() time.Time { return base })
	   if err := svc.VerifySnapshot(snap, stablebr.KindMint, "USDC"); err != nil {
		   t.Fatalf("verify before expiry: %v", err)
	   }
  }

  func TestAttestSnapshotMissingItemFailClosed(t *testing.T) {
	   svc := stablebr.New()
	  now := time.Now().UTC()
	  svc.SetClock(func() time.Time { return now })
	  // 只写入部分证明
	  _, _ = svc.Attest(stablebr.AttestReq{Kind: "mint", Asset: "USDT", Item: string(stablebr.ItemReserve), Actor: "ops-a", TTLSec: 3600})
	   _, err := svc.SnapshotAttest("mint", "USDT")
	  if err == nil {
		   t.Fatal("expected error when checklist incomplete")
	   }
	   if !strings.Contains(err.Error(), "attest") && !strings.Contains(err.Error(), "expired") {
		   t.Fatalf("unexpected error: %v", err)
	  }
   }

// ─── 场景 3–8：Matcher ────────────────────────────────────────────────────────

type matcherCase struct {
	name string
	events  []brmismatch.ChainEvent
	entries []brmismatch.BookEntry
	wantMatched    int
	wantMismatches int
	wantKinds []brmismatch.MismatchKind
}

  func TestMatcherTableDriven(t *testing.T) {
	  window := "2026-04-28T19:00Z/1h"
	  now := time.Date(2026, 9, 21, 19, 30, 0, 0, time.UTC)
	   cases := []matcherCase{
		  {
			  name: "MintOver_USDT",
			  events: []brmismatch.ChainEvent{
				  {TxHash: "0xaaaa", LogIndex: 0, Chain: "ETH", Asset: "USDT", Amount: "100.0", Kind: "mint", At: now},
			   },
			  entries: nil,
			   wantMatched: 0,
			  wantMismatches: 1,
			   wantKinds: []brmismatch.MismatchKind{brmismatch.KindMintOver},
		   },
		  {
			  name: "BurnOver_USDC",
			  events: []brmismatch.ChainEvent{
				   {TxHash: "0xbbbb", LogIndex: 1, Chain: "ETH", Asset: "USDC", Amount: "500.0", Kind: "burn", At: now},
			   },
			  entries:     nil,
			   wantMatched: 0,
			   wantMismatches: 1,
			   wantKinds: []brmismatch.MismatchKind{brmismatch.KindBurnOver},
		   },
		  {
			  name: "MintMiss_USDT",
			  events: nil,
			   entries: []brmismatch.BookEntry{
				   {ID: "e1", UserID: "user-alice", Asset: "USDT", Amount: "200.0", Kind: "mint", At: now},
			  },
			   wantMatched:    0,
			   wantMismatches: 1,
			  wantKinds:      []brmismatch.MismatchKind{brmismatch.KindMintMiss},
		   },
		   {
			  name: "BurnMiss_USDC",
			  events: nil,
			  entries: []brmismatch.BookEntry{
				   {ID: "e2", UserID: "user-bob", Asset: "USDC", Amount: "750.50", Kind: "redeem", At: now},
			  },
			  wantMatched: 0,
			   wantMismatches: 1,
			   wantKinds: []brmismatch.MismatchKind{brmismatch.KindBurnMiss},
		  },
		   {
			  name: "FullMatch_MultiAsset",
			   events: []brmismatch.ChainEvent{
				  {TxHash: "0xcccc", LogIndex: 0, Chain: "ETH", Asset: "USDT", Amount: "300.0", Kind: "mint", At: now},
				  {TxHash: "0xdddd", LogIndex: 0, Chain: "ETH", Asset: "USDC", Amount: "400.0", Kind: "mint", At: now},
			   },
			  entries: []brmismatch.BookEntry{
				  {ID: "e3", UserID: "user-charlie", Asset: "USDT", Amount: "300.0", Kind: "mint", At: now},
				   {ID: "e4", UserID: "user-dave", Asset: "USDC", Amount: "400.0", Kind: "mint", At: now},
			  },
			  wantMatched:    2,
			   wantMismatches: 0,
			   wantKinds:      nil,
		   },
		   {
			   name: "CompoundFault_MultiAsset",
			  events: []brmismatch.ChainEvent{
				   {TxHash: "0xeeee", LogIndex: 0, Chain: "ETH", Asset: "USDT", Amount: "100.0", Kind: "mint", At: now},
				  {TxHash: "0xffff", LogIndex: 0, Chain: "ETH", Asset: "USDC", Amount: "600.0", Kind: "burn", At: now},
			   },
			   entries: []brmismatch.BookEntry{
				  {ID: "e5", UserID: "user-eve", Asset: "USDT", Amount: "100.0", Kind: "mint", At: now},
				   {ID: "e6", UserID: "user-frank", Asset: "USDT", Amount: "250.0", Kind: "redeem", At: now},
			   },
			  wantMatched: 1,
			   wantMismatches: 2,
			  wantKinds: []brmismatch.MismatchKind{brmismatch.KindBurnOver, brmismatch.KindBurnMiss},
		  },
		  {
			  name: "EmptyBoth",
			   events:  nil,
			  entries: nil,
			  wantMatched:    0,
			  wantMismatches: 0,
			   wantKinds:      nil,
		   },
	   }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   result, err := brmismatch.Match(tc.events, tc.entries, window, now)
			   if err != nil {
				   t.Fatalf("Match: %v", err)
			   }
			   if result.Matched != tc.wantMatched {
				  t.Errorf("matched: got %d, want %d", result.Matched, tc.wantMatched)
			   }
			  if len(result.Cases) != tc.wantMismatches {
				  t.Errorf("mismatch count: got %d, want %d (cases: %+v)", len(result.Cases), tc.wantMismatches, result.Cases)
			   }
			  // 验证每个案件都有 ProofHash
			   for _, mc := range result.Cases {
				   if mc.ProofHash == "" {
					  t.Errorf("case %s has empty ProofHash", mc.Kind)
				   }
				   if mc.WindowKey != window {
					  t.Errorf("case %s window_key mismatch", mc.Kind)
				  }
			  }
			  if len(tc.wantKinds) > 0 {
				  kindSet := map[brmismatch.MismatchKind]bool{}
				  for _, mc := range result.Cases {
					  kindSet[mc.Kind] = true
				   }
				   for _, wk := range tc.wantKinds {
					   if !kindSet[wk] {
						   t.Errorf("expected mismatch kind %s not found", wk)
					  }
				   }
			   }
		   })
	   }
  }

// ─── 场景 9：双边证明哈希 ─────────────────────────────────────────────────────

   func TestBilateralProofDeterminism(t *testing.T) {
	   ceh := brmismatch.ChainEventHash("0xabcd", 0)
	  eh := brmismatch.EntryHash("entry-1", "100.0")
	  wk := "2026-04-28T19:00Z/1h"
	   p1, err := brmismatch.ComputeProof(ceh, eh, wk)
	   if err != nil {
		  t.Fatal(err)
	   }
	   p2, err := brmismatch.ComputeProof(ceh, eh, wk)
	   if err != nil {
		   t.Fatal(err)
	   }
	  if p1 != p2 {
		  t.Errorf("proof not deterministic: %s != %s", p1, p2)
	   }
	  // 不同窗口产生不同 proof
	  p3, _ := brmismatch.ComputeProof(ceh, eh, "2026-04-28T20:00Z/1h")
	  if p1 == p3 {
		   t.Error("different window should produce different proof")
	  }
	   // 空输入应报错
	   if _, err := brmismatch.ComputeProof("", eh, wk); err == nil {
		  t.Error("empty chainEventHash should error")
	  }
  }

   func TestVerifyProof(t *testing.T) {
	  ceh := brmismatch.ChainEventHash("0x1111", 2)
	  eh := brmismatch.EntryHash("e-99", "999.99")
	  wk := "window-key-x"
	   proof, _ := brmismatch.ComputeProof(ceh, eh, wk)
	  if err := brmismatch.VerifyProof(ceh, eh, wk, proof); err != nil {
		  t.Fatalf("verify: %v", err)
	   }
	  if err := brmismatch.VerifyProof(ceh, eh, wk, "tampered"); err == nil {
		   t.Fatal("tampered proof should fail")
	   }
  }

// ─── 场景 10–11：账户暂停与四眼清除 ─────────────────────────────────────────

   func TestSuspendAndFourEyesClear(t *testing.T) {
	  svc := newMismatchSvc()
	  now := time.Date(2026, 9, 21, 19, 0, 0, 0, time.UTC)
	   svc.SetClock(func() time.Time { return now })
	  // 注册一个案件
	   mc := brmismatch.MismatchCase{
		  Kind:  brmismatch.KindMintMiss,
		   Asset: "USDT",
		   UserID: "user-grace",
		   ChainEventHash: brmismatch.ChainEventHash("", 0),
		   EntryHash:      brmismatch.EntryHash("entry-x", "100.0"),
		  WindowKey: "2026-04-28T19:00Z/1h",
	  }
	   mc.ProofHash, _ = brmismatch.ComputeProof(mc.ChainEventHash, mc.EntryHash, mc.WindowKey)
	  registered, err := svc.RegisterCase(mc)
	  if err != nil {
		  t.Fatalf("register: %v", err)
	  }
	  // 暂停前：不阻断
	   if err := svc.IsSuspended("user-grace", "USDT"); err != nil {
		   t.Fatalf("should not be suspended before suspend call: %v", err)
	  }
	  // 暂停账户
	   _, err = svc.SuspendAccount(brmismatch.SuspendReq{
		  CaseID: registered.ID,
		  UserID: "user-grace",
		   Asset:  "USDT",
		   Actor: "ops-alice",
		   Note:  "mismatch detected",
	   })
	   if err != nil {
		   t.Fatalf("suspend: %v", err)
	   }
	  // 暂停后：阻断
	   if err := svc.IsSuspended("user-grace", "USDT"); err != brmismatch.ErrSuspended {
		  t.Fatalf("expected ErrSuspended, got %v", err)
	   }
	   // 四眼清除
	  _, err = svc.FourEyesClear(brmismatch.FourEyesClearReq{
		  CaseID: registered.ID,
		  Actor1: "ops-alice",
		   Actor2: "ops-bob",
		  Note:   "reviewed and cleared",
	  })
	  if err != nil {
		   t.Fatalf("clear: %v", err)
	   }
	  // 清除后：不再阻断
	  if err := svc.IsSuspended("user-grace", "USDT"); err != nil {
		   t.Fatalf("should not be suspended after clear: %v", err)
	   }
	   // 案件状态确认
	  got, err := svc.GetCase(registered.ID)
	   if err != nil {
		   t.Fatal(err)
	  }
	  if got.State != brmismatch.StateCleared {
		  t.Errorf("state: got %s, want cleared", got.State)
	   }
	   if got.ClearedBy1 != "ops-alice" || got.ClearedBy2 != "ops-bob" {
		  t.Errorf("cleared_by: %s / %s", got.ClearedBy1, got.ClearedBy2)
	   }
  }

// ─── 场景 12：四眼同人拒绝 ──────────────────────────────────────────────────

   func TestFourEyesSameActorDenied(t *testing.T) {
	  svc := newMismatchSvc()
	  now := time.Now().UTC()
	   svc.SetClock(func() time.Time { return now })
	   mc := brmismatch.MismatchCase{
		  Kind: brmismatch.KindBurnMiss,
		  Asset:  "USDC",
		   UserID: "user-helen",
		   ChainEventHash: brmismatch.ChainEventHash("", 0),
		   EntryHash: brmismatch.EntryHash("e-h", "50.0"),
		  WindowKey: "w1",
	   }
	  mc.ProofHash, _ = brmismatch.ComputeProof(mc.ChainEventHash, mc.EntryHash, mc.WindowKey)
	  registered, _ := svc.RegisterCase(mc)
	  _, _ = svc.SuspendAccount(brmismatch.SuspendReq{CaseID: registered.ID, UserID: "user-helen", Asset: "USDC", Actor: "ops-x"})
	  _, err := svc.FourEyesClear(brmismatch.FourEyesClearReq{
		   CaseID: registered.ID,
		  Actor1: "ops-x",
		  Actor2: "ops-x",
	   })
	  if err != brmismatch.ErrFourEyes {
		   t.Fatalf("expected ErrFourEyes, got %v", err)
	  }
  }

// ─── 场景 13：状态机非法转移 ─────────────────────────────────────────────────

  func TestClearNonSuspendedCaseDenied(t *testing.T) {
	  svc := newMismatchSvc()
	   now := time.Now().UTC()
	   svc.SetClock(func() time.Time { return now })
	   mc := brmismatch.MismatchCase{
		  Kind:           brmismatch.KindMintOver,
		  Asset:          "USDT",
		  ChainEventHash: brmismatch.ChainEventHash("0x9999", 0),
		   EntryHash:      brmismatch.EntryHash("", ""),
		  WindowKey:      "w2",
	   }
	   mc.ProofHash, _ = brmismatch.ComputeProof(mc.ChainEventHash, mc.EntryHash, mc.WindowKey)
	   registered, _ := svc.RegisterCase(mc)
	   // 直接尝试清除 open 状态的案件
	   _, err := svc.FourEyesClear(brmismatch.FourEyesClearReq{
		   CaseID: registered.ID,
		  Actor1: "ops-a",
		  Actor2: "ops-b",
	  })
	  if err != brmismatch.ErrClearDenied {
		  t.Fatalf("expected ErrClearDenied for open case, got %v", err)
	  }
  }

// ─── 场景 14：去重 ────────────────────────────────────────────────────────────

  func TestRegisterCaseDuplicateDenied(t *testing.T) {
	   svc := newMismatchSvc()
	  now := time.Now().UTC()
	   svc.SetClock(func() time.Time { return now })
	   ceh := brmismatch.ChainEventHash("0xdead", 0)
	  eh := brmismatch.EntryHash("e-dup", "77.0")
	   wk := "w3"
	  proof, _ := brmismatch.ComputeProof(ceh, eh, wk)
	   mc := brmismatch.MismatchCase{
		  Kind: brmismatch.KindMintOver, Asset: "USDT",
		   ChainEventHash: ceh, EntryHash: eh, ProofHash: proof, WindowKey: wk,
	  }
	   if _, err := svc.RegisterCase(mc); err != nil {
		   t.Fatalf("first register: %v", err)
	  }
	  if _, err := svc.RegisterCase(mc); err != brmismatch.ErrDuplicate {
		  t.Fatalf("expected ErrDuplicate, got %v", err)
	  }
   }

// ─── 场景 15：Golden fixture 驱动 ─────────────────────────────────────────────

type goldenFixture struct {
	Comment string `json:"_comment"`
	WindowKey string `json:"window_key"`
	Events                []brmismatch.ChainEvent `json:"events"`
	Entries               []brmismatch.BookEntry  `json:"entries"`
	ExpectedMatched       int                     `json:"expected_matched"`
	ExpectedMismatchCount int                     `json:"expected_mismatch_count"`
	ExpectedMismatchKinds []string `json:"expected_mismatch_kinds"`
}

  func TestGoldenFixtures(t *testing.T) {
	   dir := filepath.Join("..", "..", "testdata", "golden", "stablebr-mismatch")
	   ents, err := os.ReadDir(dir)
	  if err != nil {
		   t.Fatal(err)
	   }
	  now := time.Date(2026, 9, 22, 0, 30, 0, 0, time.UTC)
	   for _, ent := range ents {
		   if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			  continue
		  }
		   // attest_ttl_edge.json 有不同结构，单独跳过（由 TestAttestSnapshotTTLEdgeFailClosed 覆盖）
		  if strings.Contains(ent.Name(), "attest_ttl") {
			  continue
		  }
		   t.Run(ent.Name(), func(t *testing.T) {
			  raw, err := os.ReadFile(filepath.Join(dir, ent.Name()))
			   if err != nil {
				  t.Fatal(err)
			   }
			   var fx goldenFixture
			  if err := json.Unmarshal(raw, &fx); err != nil {
				   t.Fatal(err)
			   }
			   if fx.WindowKey == "" {
				  t.Skip("no window_key in fixture")
			  }
			   result, err := brmismatch.Match(fx.Events, fx.Entries, fx.WindowKey, now)
			   if err != nil {
				   t.Fatalf("Match: %v", err)
			  }
			   if result.Matched != fx.ExpectedMatched {
				   t.Errorf("matched: got %d, want %d", result.Matched, fx.ExpectedMatched)
			   }
			  if len(result.Cases) != fx.ExpectedMismatchCount {
				  t.Errorf("mismatch count: got %d, want %d", len(result.Cases), fx.ExpectedMismatchCount)
			  }
			   if len(fx.ExpectedMismatchKinds) > 0 {
				  kindSet := map[string]bool{}
				   for _, mc := range result.Cases {
					  kindSet[string(mc.Kind)] = true
				   }
				  for _, wk := range fx.ExpectedMismatchKinds {
					  if !kindSet[wk] {
						  t.Errorf("expected kind %q not found in results", wk)
					  }
				  }
			  }
		  })
	   }
   }

package snap_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/snap"
)

  func makeSnapshot(userID string, positions []snap.Position) snap.Snapshot {
	  return snap.Snapshot{
		   ID:        "snap-test-001",
		  UserID:    userID,
		   TakenAt:   time.Now().UTC(),
		  Positions: positions,
		   Count:     len(positions),
	   }
   }

  func TestSnapshotBalanceRoot_Deterministic(t *testing.T) {
	   pos := []snap.Position{
		  {Book: "SPOT", Asset: "BTC", Available: "1.0", Frozen: "0.0"},
		   {Book: "SPOT", Asset: "USDT", Available: "5000.0", Frozen: "200.0"},
	   }
	  s := makeSnapshot("user1", pos)
	  r1 := s.BalanceRoot()
	  r2 := s.BalanceRoot()
	   if r1 != r2 {
		  t.Fatal("BalanceRoot not deterministic")
	  }
	   if len(r1) != 64 {
		  t.Fatalf("expected 64-char hex, got len=%d val=%q", len(r1), r1)
	  }
   }

   func TestSnapshotBalanceRoot_EmptyPositions(t *testing.T) {
	   s := makeSnapshot("user1", nil)
	  root := s.BalanceRoot()
	   if root != "" {
		  t.Fatalf("expected empty root for no positions, got %q", root)
	   }
  }

  func TestSnapshotBalanceRoot_DifferentAmounts(t *testing.T) {
	  pos1 := []snap.Position{{Book: "SPOT", Asset: "BTC", Available: "1.0", Frozen: "0.0"}}
	   pos2 := []snap.Position{{Book: "SPOT", Asset: "BTC", Available: "2.0", Frozen: "0.0"}}
	  r1 := makeSnapshot("user1", pos1).BalanceRoot()
	   r2 := makeSnapshot("user1", pos2).BalanceRoot()
	  if r1 == r2 {
		   t.Fatal("different amounts should produce different roots")
	  }
  }

   func TestBalanceRootDiff_Match(t *testing.T) {
	   match, diff := snap.BalanceRootDiff("abc", "abc")
	  if !match {
		  t.Fatal("expected match for equal roots")
	   }
	  if diff != "" {
		  t.Fatalf("expected empty diff, got %q", diff)
	   }
  }

  func TestBalanceRootDiff_Mismatch(t *testing.T) {
	   match, diff := snap.BalanceRootDiff("abc", "xyz")
	   if match {
		   t.Fatal("expected mismatch for different roots")
	  }
	  if diff == "" {
		   t.Fatal("expected non-empty diff summary")
	  }
  }

  func TestBalanceRootMulti_MultiUser(t *testing.T) {
	   snaps := []snap.Snapshot{
		   makeSnapshot("user1", []snap.Position{
			   {Book: "SPOT", Asset: "BTC", Available: "1.0", Frozen: "0.0"},
		  }),
		  makeSnapshot("user2", []snap.Position{
			   {Book: "SPOT", Asset: "ETH", Available: "5.0", Frozen: "1.0"},
		   }),
	   }
	   root := snap.BalanceRootMulti(snaps)
	  if len(root) != 64 {
		  t.Fatalf("expected 64-char hex, got %q", root)
	  }
	  // 顺序无关
	   snapsRev := []snap.Snapshot{snaps[1], snaps[0]}
	  root2 := snap.BalanceRootMulti(snapsRev)
	   if root != root2 {
		   t.Fatal("BalanceRootMulti should be order-independent across users")
	   }
  }

  func TestCompareBalanceRoots(t *testing.T) {
	   pos := []snap.Position{{Book: "SPOT", Asset: "BTC", Available: "1.0", Frozen: "0.0"}}
	   pre := makeSnapshot("user1", pos)
	   post := makeSnapshot("user1", pos) // 相同
	  result := snap.CompareBalanceRoots(pre, post)
	  if !result.Match {
		  t.Fatal("expected match for same positions")
	  }
	  if result.DiffSummary != "" {
		  t.Fatalf("expected empty diff, got %q", result.DiffSummary)
	   }
	  // 修改余额后不一致
	  pos2 := []snap.Position{{Book: "SPOT", Asset: "BTC", Available: "2.0", Frozen: "0.0"}}
	   post2 := makeSnapshot("user1", pos2)
	  result2 := snap.CompareBalanceRoots(pre, post2)
	   if result2.Match {
		  t.Fatal("expected mismatch for different positions")
	   }
	   if result2.DiffSummary == "" {
		   t.Fatal("expected non-empty diff summary")
	  }
   }

package vipfee_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/vipfee"
)

  func TestCurrentGen_Initial(t *testing.T) {
	  svc := vipfee.New()
	   // 新用户初始世代应为 0。
	  got := svc.CurrentGen("u-gen-001")
	  if got != 0 {
		   t.Errorf("initial CurrentGen: got %d, want 0", got)
	  }
   }

  func TestBumpGen_CAS(t *testing.T) {
	  svc := vipfee.New()
	   userID := "u-gen-bump-001"
	   // CAS 成功：expected=0，实际=0。
	  newGen, err := svc.BumpGen(userID, 0, vipfee.TierVIP)
	  if err != nil {
		  t.Fatalf("BumpGen: %v", err)
	   }
	  if newGen != 1 {
		   t.Errorf("newGen: got %d, want 1", newGen)
	   }
	   if svc.CurrentGen(userID) != 1 {
		   t.Errorf("CurrentGen after bump: got %d, want 1", svc.CurrentGen(userID))
	   }
	   // CAS 失败：expected=0，实际=1。
	  _, err = svc.BumpGen(userID, 0, vipfee.TierVIPPlus)
	  if err == nil {
		  t.Error("expected CAS failure, got nil")
	   }
	   // CAS 成功：expected=1，实际=1。
	  newGen2, err := svc.BumpGen(userID, 1, vipfee.TierVIPPlus)
	  if err != nil {
		  t.Fatalf("second BumpGen: %v", err)
	   }
	   if newGen2 != 2 {
		   t.Errorf("second newGen: got %d, want 2", newGen2)
	  }
   }

   func TestGenSnapshotOf(t *testing.T) {
	  svc := vipfee.New()
	   userID := "u-gen-snap-001"
	   snap := svc.GenSnapshotOf(userID)
	  if snap.UserID != userID {
		   t.Errorf("snapshot UserID: got %s, want %s", snap.UserID, userID)
	   }
	   if snap.Generation != 0 {
		  t.Errorf("initial snapshot generation: got %d, want 0", snap.Generation)
	   }
	  // 推进世代后快照应更新。
	  svc.BumpGen(userID, 0, vipfee.TierVIP)
	  snap2 := svc.GenSnapshotOf(userID)
	  if snap2.Generation != 1 {
		   t.Errorf("snapshot after bump: got %d, want 1", snap2.Generation)
	  }
	   if snap2.LastTier != vipfee.TierVIP {
		   t.Errorf("snapshot LastTier: got %s, want vip", snap2.LastTier)
	   }
  }

  func TestQuoteWithdrawV2_ReturnsGenToken(t *testing.T) {
	   svc := vipfee.New()
	  userID := "u-gen-quote-001"
	  // 先推进一次世代。
	   svc.BumpGen(userID, 0, vipfee.TierVIP)
	  result, err := svc.QuoteWithdrawV2(userID, "BTC", "1.0")
	  if err != nil {
		  t.Fatalf("QuoteWithdrawV2: %v", err)
	   }
	   // 基础报价字段非空。
	  if result.UserID != userID {
		   t.Errorf("UserID: got %s, want %s", result.UserID, userID)
	   }
	  if result.Gross == "" {
		   t.Error("Gross must not be empty")
	  }
	   if result.Fee == "" {
		  t.Error("Fee must not be empty")
	   }
	  if result.Net == "" {
		   t.Error("Net must not be empty")
	   }
	  // 世代令牌应携带当前世代。
	  if result.GenToken.Generation != 1 {
		   t.Errorf("GenToken.Generation: got %d, want 1", result.GenToken.Generation)
	  }
	  if result.GenToken.UserID != userID {
		   t.Errorf("GenToken.UserID: got %s, want %s", result.GenToken.UserID, userID)
	   }
   }

   func TestBumpGen_MultipleUsers_Isolated(t *testing.T) {
	  svc := vipfee.New()
	  users := []string{"u-isol-A", "u-isol-B", "u-isol-C"}
	   for _, uid := range users {
		   if got := svc.CurrentGen(uid); got != 0 {
			  t.Errorf("%s initial gen: got %d, want 0", uid, got)
		  }
		   svc.BumpGen(uid, 0, vipfee.TierVIP)
	  }
	  // 每个用户世代独立。
	   for _, uid := range users {
		   if got := svc.CurrentGen(uid); got != 1 {
			  t.Errorf("%s gen after bump: got %d, want 1", uid, got)
		   }
	  }
	   // 推进 u-isol-A 再一次，不影响其他用户。
	   svc.BumpGen("u-isol-A", 1, vipfee.TierVIPPlus)
	   if got := svc.CurrentGen("u-isol-A"); got != 2 {
		   t.Errorf("u-isol-A gen: got %d, want 2", got)
	   }
	  if got := svc.CurrentGen("u-isol-B"); got != 1 {
		  t.Errorf("u-isol-B gen should still be 1, got %d", got)
	  }
  }

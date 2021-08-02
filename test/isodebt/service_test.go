package isodebt_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/isobook"
	"account-assets.kucoin.net/funding/internal/isodebt"
)

// ─────────────────────────────────────────────
// 辅助
// ─────────────────────────────────────────────

   func newTestService(t *testing.T) (*isodebt.Service, *isobook.Book) {
	  t.Helper()
	  book := isobook.New()
	   cfg := isodebt.Config{
		  MaxDebtRatio: decimal.NewFromFloat(0.97),
		  LeaseTTL:     5 * time.Second,
		   RecomputeInterval: time.Minute,
		  DriftPatrolInterval: time.Minute,
		  DriftThreshold:      decimal.NewFromFloat(0.01),
	  }
	   store := isodebt.NewMemStore()
	   svc := isodebt.NewService(store, book, cfg)
	  return svc, book
  }

  func mustCredit(t *testing.T, book *isobook.Book, userID, symbol, cur, amount string) {
	   t.Helper()
	   if _, err := book.Credit(userID, symbol, cur, amount); err != nil {
		   t.Fatalf("Credit failed: %v", err)
	   }
   }

   func mustDebit(t *testing.T, book *isobook.Book, userID, symbol, cur, amount string) {
	   t.Helper()
	   if _, err := book.Debit(userID, symbol, cur, amount); err != nil {
		   t.Fatalf("Debit failed: %v", err)
	  }
   }

// ─────────────────────────────────────────────
// 1. 世代递增
// ─────────────────────────────────────────────

   func TestGenerationBump(t *testing.T) {
	   ctx := context.Background()
	  cases := []struct {
		  name    string
		   ops     []string // "credit" | "debit"
		  wantGen int64
		   wantTrigger string
	  }{
		  {"zero_ops", nil, 0, ""},
		  {"single_credit", []string{"credit"}, 1, "credit"},
		   {"single_debit", []string{"debit"}, 1, "debit"},
		   {"credit_then_debit", []string{"credit", "debit"}, 2, "debit"},
		  {"three_ops", []string{"credit", "debit", "credit"}, 3, "credit"},
	  }
	  for _, tc := range cases {
		  t.Run(tc.name, func(t *testing.T) {
			  svc, book := newTestService(t)
			   // 初始化 book（保证 Pair 存在）
			   book.Ensure("u-gen", "BTC-USDT")
			   mustCredit(t, book, "u-gen", "BTC-USDT", "USDT", "1000")
			   var lastGen isodebt.PairGeneration
			  var lastErr error
			   for i, op := range tc.ops {
				  txnID := "txn-" + op + "-" + string(rune('0'+i))
				  switch op {
				   case "credit":
					  lastGen, lastErr = svc.CreditAndBump(ctx, "u-gen", "BTC-USDT", txnID)
				  case "debit":
					   lastGen, lastErr = svc.DebitAndBump(ctx, "u-gen", "BTC-USDT", txnID)
				  }
				   if lastErr != nil {
					   t.Fatalf("op=%s error: %v", op, lastErr)
				  }
			  }
			  gen, err := svc.CurrentGeneration(ctx, "u-gen", "BTC-USDT")
			  if err != nil {
				  t.Fatalf("CurrentGeneration: %v", err)
			   }
			  if gen.Generation != tc.wantGen {
				   t.Errorf("generation: got %d, want %d", gen.Generation, tc.wantGen)
			   }
			  if tc.wantTrigger != "" && lastGen.TriggeredBy != tc.wantTrigger {
				  t.Errorf("triggered_by: got %s, want %s", lastGen.TriggeredBy, tc.wantTrigger)
			   }
		  })
	   }
  }

// ─────────────────────────────────────────────
// 2. 租约签发与校验
// ─────────────────────────────────────────────

   func TestRatioLease(t *testing.T) {
	   ctx := context.Background()
	   t.Run("fresh_lease_valid", func(t *testing.T) {
		  svc, book := newTestService(t)
		   book.Ensure("u-lease", "ETH-USDT")
		   mustCredit(t, book, "u-lease", "ETH-USDT", "USDT", "10000")
		  lease, err := svc.IssueRatioLease(ctx, "u-lease", "ETH-USDT", "test")
		   if err != nil {
			   t.Fatalf("IssueRatioLease: %v", err)
		  }
		   if lease.State != isodebt.LeaseStateFresh {
			  t.Errorf("state: got %s, want fresh", lease.State)
		   }
		   validated, err := svc.ValidateLease(ctx, lease.ID)
		  if err != nil {
			  t.Fatalf("ValidateLease: %v", err)
		   }
		   if validated.State != isodebt.LeaseStateFresh {
			  t.Errorf("validated state: got %s, want fresh", validated.State)
		   }
	   })
	   t.Run("stale_after_debit_bump", func(t *testing.T) {
		   svc, book := newTestService(t)
		   book.Ensure("u-stale", "BTC-USDT")
		  mustCredit(t, book, "u-stale", "BTC-USDT", "USDT", "5000")
		   lease, err := svc.IssueRatioLease(ctx, "u-stale", "BTC-USDT", "test")
		   if err != nil {
			   t.Fatalf("IssueRatioLease: %v", err)
		   }
		   // Debit 递增世代
		  if _, err := svc.DebitAndBump(ctx, "u-stale", "BTC-USDT", "txn-stale-001"); err != nil {
			  t.Fatalf("DebitAndBump: %v", err)
		   }
		  // 旧租约应失效（世代不符）
		   _, err = svc.ValidateLease(ctx, lease.ID)
		  if err == nil {
			   t.Fatal("expected error for stale lease, got nil")
		  }
		  if !strings.Contains(err.Error(), "generation mismatch") && !strings.Contains(err.Error(), "stale") {
			  t.Errorf("error should mention generation mismatch or stale, got: %v", err)
		   }
	  })
	  t.Run("expired_lease_rejected", func(t *testing.T) {
		  svc, book := newTestService(t)
		   book.Ensure("u-exp", "BTC-USDT")
		  mustCredit(t, book, "u-exp", "BTC-USDT", "USDT", "5000")
		  // 注入过去时钟使租约立即过期
		  pastClock := func() time.Time { return time.Now().UTC().Add(-10 * time.Second) }
		  svc.WithClock(pastClock)
		   lease, err := svc.IssueRatioLease(ctx, "u-exp", "BTC-USDT", "test")
		  if err != nil {
			   t.Fatalf("IssueRatioLease: %v", err)
		   }
		  // 恢复系统时钟
		   svc.WithClock(func() time.Time { return time.Now().UTC() })
		   _, err = svc.ValidateLease(ctx, lease.ID)
		  if err == nil {
			   t.Fatal("expected error for expired lease")
		  }
		   if !strings.Contains(err.Error(), "expired") && !strings.Contains(err.Error(), "stale") {
			  t.Errorf("error should mention expiry, got: %v", err)
		   }
	   })
	  t.Run("over_ratio_blocked", func(t *testing.T) {
		  _, book := newTestService(t)
		  book.Ensure("u-over", "BTC-USDT")
		   mustCredit(t, book, "u-over", "BTC-USDT", "USDT", "100")
		  // 构造 gen with high ratio（直接通过 CreditAndBump 后人工注入高 ratio 不可行；
		   // 此处通过极低 equity 配合高 debt ）
		   // isobook credit 100 USDT；然后 debit 98 USDT → available=2，liability≈98/100=0.98>0.97
		   // isobook 本身不跟踪 liability；通过 store 注入高世代
		  store := isodebt.NewMemStore()
		  svc2 := isodebt.NewService(store, book, isodebt.Config{
			  MaxDebtRatio: decimal.NewFromFloat(0.97),
			   LeaseTTL:     30 * time.Second,
			   RecomputeInterval:   time.Minute,
			  DriftPatrolInterval: time.Minute,
			   DriftThreshold:      decimal.NewFromFloat(0.01),
		  })
		  // 手动保存一个高比率世代
		   highGen := isodebt.PairGeneration{
			   ID: "gen-high", UserID: "u-over2", Symbol: "BTC-USDT",
			   Generation: 1, DebtRatio: decimal.NewFromFloat(0.98),
			   TotalDebt: decimal.NewFromFloat(98), TotalEquity: decimal.NewFromFloat(100),
			   ComputedAt: time.Now().UTC(), TriggeredBy: "debit",
		   }
		   if err := store.SaveGeneration(ctx, highGen); err != nil {
			  t.Fatalf("SaveGeneration: %v", err)
		  }
		   _, err := svc2.IssueRatioLease(ctx, "u-over2", "BTC-USDT", "test")
		  if err == nil {
			   t.Fatal("expected error for over-limit ratio, got nil")
		  }
		  if !strings.Contains(err.Error(), "over limit") {
			  t.Errorf("error should mention over limit, got: %v", err)
		  }
	   })
   }

// ─────────────────────────────────────────────
// 3. TransferIn 裁决
// ─────────────────────────────────────────────

   func TestAdjudicateTransferIn(t *testing.T) {
	   ctx := context.Background()
	  t.Run("allowed_on_fresh_lease", func(t *testing.T) {
		  svc, book := newTestService(t)
		  book.Ensure("u-adj", "BTC-USDT")
		   mustCredit(t, book, "u-adj", "BTC-USDT", "USDT", "10000")
		   lease, err := svc.IssueRatioLease(ctx, "u-adj", "BTC-USDT", "test")
		   if err != nil {
			  t.Fatalf("IssueRatioLease: %v", err)
		   }
		  amt, _ := decimal.NewFromString("100")
		   gate, err := svc.AdjudicateTransferIn(ctx, isodebt.AdjudicateRequest{
			   LeaseID: lease.ID, UserID: "u-adj", Symbol: "BTC-USDT",
			  Currency: "USDT", Amount: amt, RequestedBy: "test",
		   })
		   if err != nil {
			  t.Fatalf("AdjudicateTransferIn: %v", err)
		   }
		  if gate.State != isodebt.GateStateAllowed {
			   t.Errorf("gate state: got %s, want allowed", gate.State)
		   }
	   })
	   t.Run("rejected_on_stale_lease", func(t *testing.T) {
		   svc, book := newTestService(t)
		   book.Ensure("u-rej", "ETH-USDT")
		  mustCredit(t, book, "u-rej", "ETH-USDT", "USDT", "10000")
		  lease, _ := svc.IssueRatioLease(ctx, "u-rej", "ETH-USDT", "test")
		   // Debit 推进世代
		  _, _ = svc.DebitAndBump(ctx, "u-rej", "ETH-USDT", "txn-rej-001")
		   amt, _ := decimal.NewFromString("200")
		   gate, err := svc.AdjudicateTransferIn(ctx, isodebt.AdjudicateRequest{
			  LeaseID: lease.ID, UserID: "u-rej", Symbol: "ETH-USDT",
			  Currency: "USDT", Amount: amt, RequestedBy: "test",
		   })
		  if err == nil {
			  t.Fatal("expected error for stale lease in adjudication")
		  }
		   if gate.State != isodebt.GateStateRejected {
			  t.Errorf("gate state: got %s, want rejected", gate.State)
		  }
	  })
	  t.Run("invalid_amount_rejected", func(t *testing.T) {
		  svc, book := newTestService(t)
		  book.Ensure("u-inv", "BTC-USDT")
		  mustCredit(t, book, "u-inv", "BTC-USDT", "USDT", "10000")
		  lease, _ := svc.IssueRatioLease(ctx, "u-inv", "BTC-USDT", "test")
		  _, err := svc.AdjudicateTransferIn(ctx, isodebt.AdjudicateRequest{
			  LeaseID: lease.ID, UserID: "u-inv", Symbol: "BTC-USDT",
			  Currency: "USDT", Amount: decimal.Zero, RequestedBy: "test",
		  })
		   if err == nil {
			   t.Fatal("expected error for zero amount")
		  }
	  })
   }

// ─────────────────────────────────────────────
// 4. 借记↔转入互斥
// ─────────────────────────────────────────────

  func TestDebitTransferMutex(t *testing.T) {
	  ctx := context.Background()
	  t.Run("transfer_blocked_during_debit_lock", func(t *testing.T) {
		  svc, book := newTestService(t)
		  book.Ensure("u-mutex", "BTC-USDT")
		   mustCredit(t, book, "u-mutex", "BTC-USDT", "USDT", "10000")
		   lease, err := svc.IssueRatioLease(ctx, "u-mutex", "BTC-USDT", "test")
		   if err != nil {
			   t.Fatalf("IssueRatioLease: %v", err)
		   }
		   // 锁定 Debit
		   unlock := svc.LockDebit("u-mutex", "BTC-USDT", "txn-debit-mutex")
		   defer unlock()
		  // 尝试 TransferIn → 应被 fail-closed
		  amt, _ := decimal.NewFromString("100")
		  _, err = svc.AdjudicateTransferIn(ctx, isodebt.AdjudicateRequest{
			  LeaseID: lease.ID, UserID: "u-mutex", Symbol: "BTC-USDT",
			   Currency: "USDT", Amount: amt, RequestedBy: "test",
		   })
		  if err == nil {
			  t.Fatal("expected mutex conflict error")
		  }
		  if !strings.Contains(err.Error(), "mutex conflict") {
			  t.Errorf("expected mutex conflict error, got: %v", err)
		  }
	   })
	  t.Run("transfer_allowed_after_unlock", func(t *testing.T) {
		  svc, book := newTestService(t)
		   book.Ensure("u-unlock", "BTC-USDT")
		  mustCredit(t, book, "u-unlock", "BTC-USDT", "USDT", "10000")
		   // 先锁再解锁
		   unlock := svc.LockDebit("u-unlock", "BTC-USDT", "txn-debit-unlock")
		   unlock()
		  // 重新签发租约
		   lease, err := svc.IssueRatioLease(ctx, "u-unlock", "BTC-USDT", "test")
		  if err != nil {
			  t.Fatalf("IssueRatioLease after unlock: %v", err)
		  }
		  amt, _ := decimal.NewFromString("100")
		   gate, err := svc.AdjudicateTransferIn(ctx, isodebt.AdjudicateRequest{
			  LeaseID: lease.ID, UserID: "u-unlock", Symbol: "BTC-USDT",
			   Currency: "USDT", Amount: amt, RequestedBy: "test",
		   })
		   if err != nil {
			   t.Fatalf("AdjudicateTransferIn after unlock: %v", err)
		   }
		  if gate.State != isodebt.GateStateAllowed {
			  t.Errorf("gate state: got %s, want allowed", gate.State)
		   }
	  })
	   t.Run("different_pair_not_blocked", func(t *testing.T) {
		   svc, book := newTestService(t)
		  book.Ensure("u-cross", "BTC-USDT")
		   book.Ensure("u-cross", "ETH-USDT")
		  mustCredit(t, book, "u-cross", "BTC-USDT", "USDT", "10000")
		  mustCredit(t, book, "u-cross", "ETH-USDT", "USDT", "10000")
		   // BTC-USDT 锁定 Debit
		   unlock := svc.LockDebit("u-cross", "BTC-USDT", "txn-cross-btc")
		   defer unlock()
		  // ETH-USDT 的 TransferIn 不应受影响
		   lease, err := svc.IssueRatioLease(ctx, "u-cross", "ETH-USDT", "test")
		  if err != nil {
			   t.Fatalf("IssueRatioLease ETH-USDT: %v", err)
		   }
		  amt, _ := decimal.NewFromString("100")
		  gate, err := svc.AdjudicateTransferIn(ctx, isodebt.AdjudicateRequest{
			   LeaseID: lease.ID, UserID: "u-cross", Symbol: "ETH-USDT",
			   Currency: "USDT", Amount: amt, RequestedBy: "test",
		  })
		  if err != nil {
			  t.Fatalf("cross-pair transfer blocked unexpectedly: %v", err)
		   }
		   if gate.State != isodebt.GateStateAllowed {
			  t.Errorf("gate state: got %s, want allowed", gate.State)
		   }
	  })
   }

// ─────────────────────────────────────────────
// 5. 漂移发现、证明与关闭
// ─────────────────────────────────────────────

  func TestRatioDrift(t *testing.T) {
	   ctx := context.Background()
	   t.Run("no_drift_when_synced", func(t *testing.T) {
		   svc, book := newTestService(t)
		  book.Ensure("u-nodrift", "BTC-USDT")
		  mustCredit(t, book, "u-nodrift", "BTC-USDT", "USDT", "10000")
		   // 世代与 book 同步（无负债）
		   _, _ = svc.CreditAndBump(ctx, "u-nodrift", "BTC-USDT", "txn-sync")
		  res, err := svc.DetectDrift(ctx, "u-nodrift", "BTC-USDT")
		  if err != nil {
			  t.Fatalf("DetectDrift: %v", err)
		   }
		  if res.Drifted {
			  t.Errorf("expected no drift, got drifted=true delta=%s", res.Delta.String())
		   }
	   })
	   t.Run("drift_detected_blocks_transfer", func(t *testing.T) {
		   _, book := newTestService(t)
		   book.Ensure("u-drift", "BTC-USDT")
		   mustCredit(t, book, "u-drift", "BTC-USDT", "USDT", "10000")
		  // 手动注入高 ratio 世代（实际 ratio > displayed ratio）
		  store := isodebt.NewMemStore()
		   svc2 := isodebt.NewService(store, book, isodebt.Config{
			   MaxDebtRatio: decimal.NewFromFloat(0.97),
			  LeaseTTL: 30 * time.Second,
			  RecomputeInterval: time.Minute,
			  DriftPatrolInterval: time.Minute,
			  DriftThreshold:      decimal.NewFromFloat(0.01),
		   })
		  // 保存实际高比率世代
		   _ = store.SaveGeneration(ctx, isodebt.PairGeneration{
			   ID: "gen-drift", UserID: "u-drift2", Symbol: "BTC-USDT",
			  Generation: 1, DebtRatio: decimal.NewFromFloat(0.75),
			  TotalDebt: decimal.NewFromFloat(75), TotalEquity: decimal.NewFromFloat(100),
			  ComputedAt: time.Now().UTC(), TriggeredBy: "debit",
		   })
		   // isobook DebtRatio 显示为 0（未同步）→ delta=0.75 > threshold=0.01
		   res, err := svc2.DetectDrift(ctx, "u-drift2", "BTC-USDT")
		  if err != nil {
			  t.Fatalf("DetectDrift: %v", err)
		   }
		   if !res.Drifted {
			   t.Fatalf("expected drift detected")
		  }
		  if res.CaseID == "" {
			  t.Fatal("expected drift case ID")
		  }
		  // 漂移后不允许签发租约
		  _, err = svc2.IssueRatioLease(ctx, "u-drift2", "BTC-USDT", "test")
		   if err == nil {
			   t.Fatal("expected lease issue blocked by drift")
		  }
		  if !strings.Contains(err.Error(), "drift") {
			  t.Errorf("expected drift error, got: %v", err)
		   }
	  })
	  t.Run("proof_submit_and_confirm_close", func(t *testing.T) {
		   _, book := newTestService(t)
		  book.Ensure("u-proof", "ETH-USDT")
		  mustCredit(t, book, "u-proof", "ETH-USDT", "USDT", "10000")
		  store := isodebt.NewMemStore()
		   svc2 := isodebt.NewService(store, book, isodebt.Config{
			  MaxDebtRatio:      decimal.NewFromFloat(0.97),
			  LeaseTTL:          30 * time.Second,
			   RecomputeInterval: time.Minute,
			   DriftPatrolInterval: time.Minute,
			  DriftThreshold:      decimal.NewFromFloat(0.01),
		  })
		   _ = store.SaveGeneration(ctx, isodebt.PairGeneration{
			   ID: "gen-proof", UserID: "u-proof2", Symbol: "ETH-USDT",
			   Generation: 1, DebtRatio: decimal.NewFromFloat(0.80),
			   TotalDebt: decimal.NewFromFloat(80), TotalEquity: decimal.NewFromFloat(100),
			   ComputedAt: time.Now().UTC(), TriggeredBy: "debit",
		  })
		   res, _ := svc2.DetectDrift(ctx, "u-proof2", "ETH-USDT")
		  if !res.Drifted {
			  t.Fatal("expected drift")
		   }
		   caseID := res.CaseID
		  // 提交证明
		  proof, err := svc2.SubmitDriftProof(ctx, caseID, "ops-01", "ratio sync lag", "{}")
		  if err != nil {
			   t.Fatalf("SubmitDriftProof: %v", err)
		  }
		  if proof.DriftCaseID != caseID {
			   t.Errorf("proof drift_case_id: got %s, want %s", proof.DriftCaseID, caseID)
		  }
		  // proven 状态仍阻断
		   _, err = svc2.IssueRatioLease(ctx, "u-proof2", "ETH-USDT", "test")
		  if err == nil {
			   t.Fatal("expected proven drift to block lease")
		   }
		   // 四眼确认关闭
		   dc, err := svc2.ConfirmDriftClose(ctx, caseID, "ops-02")
		   if err != nil {
			  t.Fatalf("ConfirmDriftClose: %v", err)
		   }
		  if dc.State != isodebt.DriftStateClosed {
			   t.Errorf("drift case state: got %s, want closed", dc.State)
		   }
		   // closed 后无漂移案例阻断
		  openCases, _ := svc2.ListOpenDriftCases(ctx, "u-proof2", "ETH-USDT")
		   if len(openCases) != 0 {
			   t.Errorf("expected 0 open drift cases after close, got %d", len(openCases))
		  }
		   // 导出证明
		  exported, err := svc2.ExportDriftProof(ctx, proof.ID)
		  if err != nil {
			   t.Fatalf("ExportDriftProof: %v", err)
		  }
		   if exported.ConfirmedBy != "ops-02" {
			  t.Errorf("confirmed_by: got %s, want ops-02", exported.ConfirmedBy)
		   }
	   })
	   t.Run("unproven_drift_cannot_restore_transfer", func(t *testing.T) {
		   store := isodebt.NewMemStore()
		   book := isobook.New()
		  svc2 := isodebt.NewService(store, book, isodebt.Config{
			   MaxDebtRatio:      decimal.NewFromFloat(0.97),
			  LeaseTTL:          30 * time.Second,
			  DriftThreshold:    decimal.NewFromFloat(0.01),
			  RecomputeInterval: time.Minute, DriftPatrolInterval: time.Minute,
		   })
		  _ = store.SaveGeneration(ctx, isodebt.PairGeneration{
			  ID: "gen-unproven", UserID: "u-unproven", Symbol: "BTC-USDT",
			  Generation: 1, DebtRatio: decimal.NewFromFloat(0.60),
			  TotalDebt: decimal.NewFromFloat(60), TotalEquity: decimal.NewFromFloat(100),
			   ComputedAt: time.Now().UTC(), TriggeredBy: "debit",
		  })
		   res, _ := svc2.DetectDrift(ctx, "u-unproven", "BTC-USDT")
		  if !res.Drifted {
			  t.Fatal("expected drift")
		  }
		  // 不提交证明，直接尝试 ConfirmClose → 应报 state!=proven 错误
		  _, err := svc2.ConfirmDriftClose(ctx, res.CaseID, "ops-02")
		   if err == nil {
			  t.Fatal("expected error: cannot close open drift without proof")
		   }
		  if !strings.Contains(err.Error(), "proven") {
			   t.Errorf("expected proven error, got: %v", err)
		  }
		   // TransferIn 仍被阻断
		   _, err = svc2.IssueRatioLease(ctx, "u-unproven", "BTC-USDT", "test")
		  if err == nil {
			   t.Fatal("expected lease blocked for unproven drift")
		   }
	   })
  }

// ─────────────────────────────────────────────
// 6. 比率超限阻断
// ─────────────────────────────────────────────

  func TestRatioOverLimit(t *testing.T) {
	   ctx := context.Background()
	   cases := []struct {
		  name     string
		  ratio    string
		  maxRatio string
		  wantErr  bool
	  }{
		   {"under_limit", "0.50", "0.97", false},
		   {"at_limit", "0.97", "0.97", true},
		   {"over_limit", "0.99", "0.97", true},
		  {"zero_ratio", "0.00", "0.97", false},
	  }
	   for _, tc := range cases {
		   t.Run(tc.name, func(t *testing.T) {
			  store := isodebt.NewMemStore()
			   book := isobook.New()
			  maxR, _ := decimal.NewFromString(tc.maxRatio)
			  svc := isodebt.NewService(store, book, isodebt.Config{
				  MaxDebtRatio: maxR, LeaseTTL: 30 * time.Second,
				   DriftThreshold:      decimal.NewFromFloat(0.01),
				  RecomputeInterval:   time.Minute,
				  DriftPatrolInterval: time.Minute,
			   })
			   r,
				   _ := decimal.NewFromString(tc.ratio)
			   _ = store.SaveGeneration(ctx, isodebt.PairGeneration{
				  ID: "gen-limit-" + tc.name, UserID: "u-limit", Symbol: "BTC-USDT",
				   Generation: 1, DebtRatio: r,
				   TotalDebt:   r.Mul(decimal.NewFromInt(100)),
				   TotalEquity: decimal.NewFromInt(100),
				   ComputedAt:  time.Now().UTC(), TriggeredBy: "debit",
			  })
			  _, err := svc.IssueRatioLease(ctx, "u-limit", "BTC-USDT", "test")
			   if tc.wantErr && err == nil {
				  t.Errorf("expected error for ratio=%s max=%s", tc.ratio, tc.maxRatio)
			  }
			   if !tc.wantErr && err != nil {
				   t.Errorf("unexpected error for ratio=%s max=%s: %v", tc.ratio, tc.maxRatio, err)
			   }
		   })
	   }
   }

// ─────────────────────────────────────────────
// 7. 漂移场景完整链路（验收 1-4 映射）
// ─────────────────────────────────────────────

   func TestAcceptanceScenarios(t *testing.T) {
	   ctx := context.Background()
	   // 验收 1：Debit 抬负债后旧 TransferIn 令牌失败；新令牌按新 DebtRatio 裁决
	  t.Run("acceptance1_debit_then_old_token_fails", func(t *testing.T) {
		   svc, book := newTestService(t)
		   book.Ensure("u-acc1", "BTC-USDT")
		   mustCredit(t, book, "u-acc1", "BTC-USDT", "USDT", "10000")
		   oldLease, err := svc.IssueRatioLease(ctx, "u-acc1", "BTC-USDT", "test")
		  if err != nil {
			  t.Fatalf("IssueRatioLease: %v", err)
		   }
		  oldGen := oldLease.Generation
		   // Debit 推进世代
		  newGenInfo, err := svc.DebitAndBump(ctx, "u-acc1", "BTC-USDT", "txn-acc1")
		  if err != nil {
			   t.Fatalf("DebitAndBump: %v", err)
		  }
		   if newGenInfo.Generation != oldGen+1 {
			   t.Errorf("generation not bumped: got %d, want %d", newGenInfo.Generation, oldGen+1)
		   }
		   // 旧令牌失败
		   amt, _ := decimal.NewFromString("100")
		  gate, err := svc.AdjudicateTransferIn(ctx, isodebt.AdjudicateRequest{
			  LeaseID: oldLease.ID, UserID: "u-acc1", Symbol: "BTC-USDT",
			   Currency: "USDT", Amount: amt, RequestedBy: "acc1-test",
		  })
		  if err == nil {
			   t.Fatal("old token should fail after debit bump")
		  }
		  if gate.State != isodebt.GateStateRejected {
			   t.Errorf("old gate state: got %s, want rejected", gate.State)
		  }
		   // 新令牌按新 DebtRatio 裁决
		   newLease, err := svc.IssueRatioLease(ctx, "u-acc1", "BTC-USDT", "test")
		  if err != nil {
			  t.Fatalf("IssueRatioLease new: %v", err)
		   }
		   if newLease.Generation != newGenInfo.Generation {
			  t.Errorf("new lease gen=%d, want %d", newLease.Generation, newGenInfo.Generation)
		  }
		  gate2, err := svc.AdjudicateTransferIn(ctx, isodebt.AdjudicateRequest{
			   LeaseID: newLease.ID, UserID: "u-acc1", Symbol: "BTC-USDT",
			   Currency: "USDT", Amount: amt, RequestedBy: "acc1-test",
		  })
		   if err != nil {
			  t.Fatalf("new token adjudication: %v", err)
		   }
		  if gate2.State != isodebt.GateStateAllowed {
			  t.Errorf("new gate state: got %s, want allowed", gate2.State)
		  }
	  })
	  // 验收 2：并发 Debit 与 TransferIn：无静默成功
	   t.Run("acceptance2_concurrent_debit_transfer", func(t *testing.T) {
		   svc, book := newTestService(t)
		   book.Ensure("u-acc2", "ETH-USDT")
		   mustCredit(t, book, "u-acc2", "ETH-USDT", "USDT", "10000")
		  lease, _ := svc.IssueRatioLease(ctx, "u-acc2", "ETH-USDT", "test")
		   unlock := svc.LockDebit("u-acc2", "ETH-USDT", "txn-acc2-debit")
		   amt, _ := decimal.NewFromString("500")
		  gate, err := svc.AdjudicateTransferIn(ctx, isodebt.AdjudicateRequest{
			  LeaseID: lease.ID, UserID: "u-acc2", Symbol: "ETH-USDT",
			  Currency: "USDT", Amount: amt, RequestedBy: "acc2-test",
		   })
		  unlock()
		  if err == nil {
			  t.Fatal("concurrent debit+transfer should fail-closed")
		   }
		   if gate.State != isodebt.GateStateRejected && gate.State != "" {
			  t.Errorf("unexpected gate state: %s", gate.State)
		   }
		  if !strings.Contains(err.Error(), "mutex") {
			  t.Errorf("expected mutex error, got: %v", err)
		  }
	  })
	   // 验收 3：RatioDriftCase 证明缺失不得恢复 TransferInEnabled
	  t.Run("acceptance3_unproven_drift_blocks_transfer", func(t *testing.T) {
		   store := isodebt.NewMemStore()
		  book := isobook.New()
		  svc2 := isodebt.NewService(store, book, isodebt.Config{
			  MaxDebtRatio: decimal.NewFromFloat(0.97), LeaseTTL: 30 * time.Second,
			  DriftThreshold: decimal.NewFromFloat(0.01),
			   RecomputeInterval: time.Minute, DriftPatrolInterval: time.Minute,
		   })
		   _ = store.SaveGeneration(ctx, isodebt.PairGeneration{
			   ID: "gen-acc3", UserID: "u-acc3", Symbol: "BTC-USDT",
			   Generation: 1,
			   DebtRatio: decimal.NewFromFloat(0.55),
			  TotalDebt: decimal.NewFromFloat(55), TotalEquity: decimal.NewFromFloat(100),
			  ComputedAt: time.Now().UTC(), TriggeredBy: "debit",
		  })
		   res, _ := svc2.DetectDrift(ctx, "u-acc3", "BTC-USDT")
		  if !res.Drifted {
			  t.Fatal("expected drift")
		   }
		   // 无证明 → 不能关闭
		  _, err := svc2.ConfirmDriftClose(ctx, res.CaseID, "ops-02")
		  if err == nil {
			  t.Fatal("should not close without proof")
		  }
		  // 转入仍被阻断
		  _, err = svc2.IssueRatioLease(ctx, "u-acc3", "BTC-USDT", "test")
		  if err == nil {
			   t.Fatal("transfer should be blocked by unproven drift")
		  }
	  })
	  // 验收 4：不触发、不改写 marginhook 强平计划
	  t.Run("acceptance4_no_marginhook_trigger", func(t *testing.T) {
		   // 验证：isodebt 模型字段中不存在 liq_plan_id
		  gen := isodebt.PairGeneration{}
		  b, _ := json.Marshal(gen)
		   s := string(b)
		   if strings.Contains(s, "liq_plan") {
			   t.Errorf("PairGeneration must not contain liq_plan fields, got: %s", s)
		   }
		  lease := isodebt.RatioLease{}
		  b, _ = json.Marshal(lease)
		   s = string(b)
		   if strings.Contains(s, "liq_plan") {
			  t.Errorf("RatioLease must not contain liq_plan fields")
		  }
		  drift := isodebt.RatioDriftCase{}
		  b, _ = json.Marshal(drift)
		   s = string(b)
		   if strings.Contains(s, "liq_plan") {
			  t.Errorf("RatioDriftCase must not contain liq_plan fields")
		   }
	   })
   }

// ─────────────────────────────────────────────
// 8. ComputeRatio 单元
// ─────────────────────────────────────────────

   func TestComputeRatio(t *testing.T) {
	  cases := []struct {
		   name string
		   debt      string
		  equity    string
		   wantRatio string
	   }{
		  {"zero_debt", "0", "100", "0"},
		   {"zero_equity_zero_debt", "0", "0", "0"},
		  {"zero_equity_with_debt", "50", "0", "1"},
		  {"normal", "50", "100", "0.5"},
		   {"full_debt", "100", "100", "1"},
		  {"over_equity", "150", "100", "1.5"},
	   }
	   for _, tc := range cases {
		   t.Run(tc.name, func(t *testing.T) {
			   d, _ := decimal.NewFromString(tc.debt)
			  e, _ := decimal.NewFromString(tc.equity)
			  ratio := isodebt.ComputeRatio(d, e)
			  want, _ := decimal.NewFromString(tc.wantRatio)
			   if !ratio.Equal(want) {
				   t.Errorf("ComputeRatio(%s, %s) = %s, want %s", tc.debt, tc.equity, ratio, want)
			   }
		   })
	   }
   }

// ─────────────────────────────────────────────
// 9. golden fixture 加载（验证 JSON 可解析）
// ─────────────────────────────────────────────

   func TestGoldenFixtures(t *testing.T) {
	   fixtures := []string{
		   "../../testdata/isodebt/fixture_debit_then_stale_transfer.json",
		  "../../testdata/isodebt/fixture_concurrent_debit_transfer.json",
		  "../../testdata/isodebt/fixture_ratio_drift_proof.json",
		   "../../testdata/isodebt/fixture_isolated_from_liq_plan.json",
	  }
	   for _, f := range fixtures {
		   t.Run(f, func(t *testing.T) {
			  data, err := os.ReadFile(f)
			  if err != nil {
				  t.Fatalf("read fixture: %v", err)
			  }
			  var m map[string]interface{}
			   if err := json.Unmarshal(data, &m); err != nil {
				   t.Fatalf("parse fixture JSON: %v", err)
			  }
			   if _, ok := m["scenario"]; !ok {
				   t.Error("fixture missing 'scenario' field")
			  }
		  })
	   }
  }

// ─────────────────────────────────────────────
// 10. IsTransferInBlocked 综合
// ─────────────────────────────────────────────

   func TestIsTransferInBlocked(t *testing.T) {
	   ctx := context.Background()
	  t.Run("not_blocked_normal", func(t *testing.T) {
		   svc, book := newTestService(t)
		  book.Ensure("u-block", "BTC-USDT")
		   mustCredit(t, book, "u-block", "BTC-USDT", "USDT", "10000")
		  _, _ = svc.CreditAndBump(ctx, "u-block", "BTC-USDT", "txn-block-sync")
		   blocked, reason, err := svc.IsTransferInBlocked(ctx, "u-block", "BTC-USDT")
		   if err != nil {
			  t.Fatalf("IsTransferInBlocked: %v", err)
		   }
		   if blocked {
			  t.Errorf("expected not blocked, got blocked: %s", reason)
		  }
	  })
	  t.Run("blocked_by_over_limit_ratio", func(t *testing.T) {
		   store := isodebt.NewMemStore()
		   book := isobook.New()
		  svc := isodebt.NewService(store, book, isodebt.Config{
			  MaxDebtRatio: decimal.NewFromFloat(0.97), LeaseTTL: 30 * time.Second,
			   DriftThreshold:    decimal.NewFromFloat(0.01),
			  RecomputeInterval: time.Minute,
			   DriftPatrolInterval: time.Minute,
		   })
		   _ = store.SaveGeneration(ctx, isodebt.PairGeneration{
			  ID: "gen-bl", UserID: "u-bl", Symbol: "BTC-USDT",
			   Generation: 1, DebtRatio: decimal.NewFromFloat(0.99),
			   TotalDebt:   decimal.NewFromFloat(99),
			  TotalEquity: decimal.NewFromFloat(100),
			   ComputedAt:  time.Now().UTC(), TriggeredBy: "debit",
		   })
		  blocked, reason, err := svc.IsTransferInBlocked(ctx, "u-bl", "BTC-USDT")
		   if err != nil {
			  t.Fatalf("IsTransferInBlocked: %v", err)
		  }
		  if !blocked {
			   t.Fatal("expected blocked by over-limit ratio")
		  }
		   if !strings.Contains(reason, "debt_ratio") {
			  t.Errorf("reason should mention debt_ratio, got: %s", reason)
		  }
	  })
  }

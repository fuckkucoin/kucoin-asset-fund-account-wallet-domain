package flow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/feeoracle"
	"account-assets.kucoin.net/funding/internal/feereserve"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/shopspring/decimal"
)

// ────────────────────────────────────────────────────────────────────────────
// 1. feeoracle.Normalize — 表驱动，覆盖全部链族
// ────────────────────────────────────────────────────────────────────────────

type normCase struct {
	Desc string `json:"desc"`
	Input feeoracle.ChainFeeInput `json:"input"`
	WantNativeAsset string `json:"want_native_asset"`
	WantNativeAmt string `json:"want_native_amount"`
	WantError     string `json:"want_error,omitempty"`
}

  func TestFeeOracleNormalize(t *testing.T) {
	  t.Parallel()
	   f, err := os.Open(filepath.Join("..", "..", "internal", "feeoracle", "testdata", "normalize_cases.json"))
	   if err != nil {
		   t.Fatalf("open normalize_cases.json: %v", err)
	   }
	   defer f.Close()
	  var cases []normCase
	   if err := json.NewDecoder(f).Decode(&cases); err != nil {
		  t.Fatalf("decode normalize_cases.json: %v", err)
	  }
	   if len(cases) == 0 {
		  t.Fatal("empty normalize_cases.json")
	  }
	  for _, tc := range cases {
		  tc := tc
		   t.Run(tc.Desc, func(t *testing.T) {
			  t.Parallel()
			  got, err := feeoracle.Normalize(tc.Input)
			  if tc.WantError != "" {
				   if err == nil {
					  t.Fatalf("expected error containing %q, got nil", tc.WantError)
				  }
				   return
			  }
			  if err != nil {
				  t.Fatalf("Normalize error: %v", err)
			  }
			  if got.NativeAsset != tc.WantNativeAsset {
				   t.Errorf("NativeAsset: got %q, want %q", got.NativeAsset, tc.WantNativeAsset)
			   }
			   wantDec, parseErr := decimal.NewFromString(tc.WantNativeAmt)
			   if parseErr != nil {
				  t.Fatalf("parse want_native_amount %q: %v", tc.WantNativeAmt, parseErr)
			  }
			  if !got.NativeAmount.Equal(wantDec) {
				  t.Errorf("NativeAmount: got %s, want %s (desc: %s)", got.NativeAmount, wantDec, tc.Desc)
			  }
		   })
	   }
   }

// ────────────────────────────────────────────────────────────────────────────
// 2. feeoracle.Oracle 聚合场景
// ────────────────────────────────────────────────────────────────────────────

type oracleScenarioTick struct {
	SourceID string `json:"source_id"`
	AgeSec   int    `json:"age_sec"`
	NativeAmount string `json:"native_amount"`
}

type oracleScenarioSource struct {
	ID    string `json:"id"`
	Chain string `json:"chain"`
	Family string `json:"family"`
	Weight int `json:"weight"`
	Enabled bool `json:"enabled"`
}

type oracleScenario struct {
	Desc string `json:"desc"`
	StaleSec int `json:"stale_sec"`
	DeviationBPS int `json:"deviation_bps"`
	Sources []oracleScenarioSource `json:"sources"`
	Ticks            []oracleScenarioTick `json:"ticks"`
	WantNativeAmount string               `json:"want_native_amount"`
	WantError string `json:"want_error"`
}

  func TestFeeOracleAggregate(t *testing.T) {
	  t.Parallel()
	  f, err := os.Open(filepath.Join("..", "..", "internal", "feeoracle", "testdata", "oracle_scenarios.json"))
	  if err != nil {
		  t.Fatalf("open oracle_scenarios.json: %v", err)
	   }
	   defer f.Close()
	  var scenarios map[string]oracleScenario
	  if err := json.NewDecoder(f).Decode(&scenarios); err != nil {
		  t.Fatalf("decode oracle_scenarios.json: %v", err)
	  }
	  fixedNow := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	  for name, sc := range scenarios {
		  name, sc := name, sc
		   t.Run(name, func(t *testing.T) {
			  t.Parallel()
			  devBPS := sc.DeviationBPS
			  if devBPS == 0 {
				   devBPS = 10000
			  }
			   oracle := feeoracle.NewOracle(devBPS).WithClock(func() time.Time { return fixedNow })
			   staleDur := time.Duration(sc.StaleSec) * time.Second
			   // identify the chain for GetAggregated
			  chain := ""
			  for _, src := range sc.Sources {
				  cfg := feeoracle.SourceConfig{
					  ID: src.ID,
					   Chain: src.Chain,
					   Family: feeoracle.Family(src.Family),
					   Weight: src.Weight,
					  Enabled:  src.Enabled,
					   StaleDur: staleDur,
				   }
				   if chain == "" {
					   chain = src.Chain
				  }
				  if regErr := oracle.RegisterSource(cfg); regErr != nil {
					  t.Fatalf("RegisterSource %s: %v", src.ID, regErr)
				   }
			  }
			  for _, tk := range sc.Ticks {
				  nativeAmt, parseErr := decimal.NewFromString(tk.NativeAmount)
				  if parseErr != nil {
					   t.Fatalf("parse tick native_amount %q: %v", tk.NativeAmount, parseErr)
				   }
				   recordedAt := fixedNow.Add(-time.Duration(tk.AgeSec) * time.Second)
				  tick := feeoracle.ChainFeeTick{
					   RecordedAt: recordedAt,
					   TickedAt:   recordedAt,
					  Normalized: feeoracle.NormalizedFee{NativeAmount: nativeAmt},
				  }
				   if pushErr := oracle.PushTick(tk.SourceID, tick); pushErr != nil {
					   t.Fatalf("PushTick %s: %v", tk.SourceID, pushErr)
				   }
			   }
			  if chain == "" {
				   chain = "NOCHAN"
			  }
			  got, aggErr := oracle.GetAggregated(chain)
			  if sc.WantError != "" {
				  if aggErr == nil {
					  t.Fatalf("[%s] expected error %q, got nil (aggregated=%s)", name, sc.WantError, got.Normalized.NativeAmount)
				  }
				  return
			  }
			  if aggErr != nil {
				  t.Fatalf("[%s] unexpected error: %v", name, aggErr)
			   }
			  if sc.WantNativeAmount != "" {
				   wantDec, parseErr := decimal.NewFromString(sc.WantNativeAmount)
				  if parseErr != nil {
					   t.Fatalf("parse want_native_amount %q: %v", sc.WantNativeAmount, parseErr)
				   }
				  if !got.Normalized.NativeAmount.Equal(wantDec) {
					   t.Errorf("[%s] NativeAmount: got %s, want %s", name, got.Normalized.NativeAmount, wantDec)
				   }
			   }
		   })
	   }
  }

// ────────────────────────────────────────────────────────────────────────────
// 3. feereserve settle journal — 表驱动，覆盖 zero/over/under + release
// ────────────────────────────────────────────────────────────────────────────

type settleReserveFixture struct {
	WithdrawID string `json:"withdraw_id"`
	UserID   string `json:"user_id"`
	Asset    string `json:"asset"`
	Chain    string `json:"chain"`
	Reserved string `json:"reserved"`
}

type settleCase struct {
	Desc    string               `json:"desc"`
	Reserve settleReserveFixture `json:"reserve"`
	ActualFee string `json:"actual_fee,omitempty"`
	Action    string `json:"action,omitempty"`
	WantDelta string `json:"want_delta,omitempty"`
	WantDeltaDir     string `json:"want_delta_dir,omitempty"`
	WantState        string `json:"want_state"`
	WantJournalLines int    `json:"want_journal_lines"`
	WantSystemFeeCredit string `json:"want_system_fee_credit"`
	WantUserRefund      string `json:"want_user_refund"`
	WantUserTopup string `json:"want_user_topup"`
}

   func TestFeeReserveSettleJournal(t *testing.T) {
	  t.Parallel()
	   f, err := os.Open(filepath.Join("..", "..", "internal", "feereserve", "testdata", "settle_cases.json"))
	  if err != nil {
		  t.Fatalf("open settle_cases.json: %v", err)
	   }
	   defer f.Close()
	   var cases []settleCase
	   if err := json.NewDecoder(f).Decode(&cases); err != nil {
		   t.Fatalf("decode settle_cases.json: %v", err)
	  }
	   if len(cases) == 0 {
		   t.Fatal("empty settle_cases.json")
	  }
	  for _, tc := range cases {
		   tc := tc
		  t.Run(tc.Desc, func(t *testing.T) {
			  t.Parallel()
			   if tc.Action == "release" {
				   testFeeReserveRelease(t, tc)
				   return
			  }
			  testFeeReserveSettle(t, tc)
		  })
	  }
  }

   func testFeeReserveSettle(t *testing.T, tc settleCase) {
	  t.Helper()
	   fix := tc.Reserve
	  r := &feereserve.FeeReserve{
		  ID: "test-" + fix.WithdrawID,
		   WithdrawID: fix.WithdrawID,
		  UserID:     fix.UserID,
		   Asset:      fix.Asset,
		  Chain:      fix.Chain,
		  Reserved: fix.Reserved,
		  State: feereserve.StateReserved,
	   }
	   delta, dir, err := r.ComputeDelta(tc.ActualFee)
	  if err != nil {
		  t.Fatalf("ComputeDelta: %v", err)
	   }
	  if tc.WantDeltaDir != "" && string(dir) != tc.WantDeltaDir {
		  t.Errorf("DeltaDir: got %q, want %q", dir, tc.WantDeltaDir)
	  }
	  if tc.WantDelta != "" {
		   var wantD money.Amount
		  if tc.WantDelta == "0" {
			  wantD = money.Zero()
		  } else {
			  var parseErr error
			  wantD, parseErr = money.Parse(tc.WantDelta)
			  if parseErr != nil {
				   t.Fatalf("parse want_delta %q: %v", tc.WantDelta, parseErr)
			  }
		  }
		  if !delta.Equal(wantD) {
			   t.Errorf("Delta: got %s, want %s", delta, wantD)
		  }
	  }
	  reserved, _ := money.Parse(fix.Reserved)
	   actual, _ := money.Parse(tc.ActualFee)
	  js, err := feereserve.BuildSettleJournal(fix.Asset, reserved, actual, dir, delta)
	  if err != nil {
		   t.Fatalf("BuildSettleJournal: %v", err)
	  }
	   if !js.Balanced {
		  t.Error("journal not balanced")
	  }
	  if len(js.Lines) != tc.WantJournalLines {
		  t.Errorf("journal lines: got %d, want %d", len(js.Lines), tc.WantJournalLines)
	   }
	   wantSysFee := parseMaybeZero(t, tc.WantSystemFeeCredit)
	   wantRefund := parseMaybeZero(t, tc.WantUserRefund)
	  wantTopup := parseMaybeZero(t, tc.WantUserTopup)
	  checkJournalAmounts(t, js, fix.Asset, wantSysFee, wantRefund, wantTopup)
  }

   func testFeeReserveRelease(t *testing.T, tc settleCase) {
	  t.Helper()
	   fix := tc.Reserve
	   svc := feereserve.NewMem()
	  ctx := context.Background()
	  reserveIn := feereserve.ReserveRequest{
		  WithdrawID: fix.WithdrawID,
		   UserID:     fix.UserID,
		   Asset:      fix.Asset,
		   Chain: fix.Chain,
		   ReservedFee: fix.Reserved,
	  }
	   reserved,
		  err := svc.Reserve(ctx, reserveIn)
	  if err != nil {
		   t.Fatalf("Reserve: %v", err)
	  }
	  if reserved.State != feereserve.StateReserved {
		   t.Fatalf("after Reserve: state=%s, want reserved", reserved.State)
	  }
	  released, err := svc.Release(ctx, fix.WithdrawID)
	  if err != nil {
		   t.Fatalf("Release: %v", err)
	  }
	   if string(released.State) != tc.WantState {
		  t.Errorf("state after Release: got %q, want %q", released.State, tc.WantState)
	  }
	  // 幂等：再次 Release 应成功（不报错）。
	   _, err = svc.Release(ctx, fix.WithdrawID)
	   if err != nil {
		  t.Errorf("idempotent Release: %v", err)
	   }
  }

// parseMaybeZero parses an amount string, returning money.Zero for "0" or empty.

  func parseMaybeZero(t *testing.T, s string) money.Amount {
	   t.Helper()
	  if s == "" || s == "0" {
		   return money.Zero()
	  }
	   a, err := money.Parse(s)
	   if err != nil {
		   t.Fatalf("parse amount %q: %v", s, err)
	   }
	   return a
  }

// checkJournalAmounts 验证 journal 中 system_fee credit、user_refund、user_topup 的金额。
   func checkJournalAmounts(t *testing.T, js feereserve.JournalSet, asset string, wantSysFee, wantRefund, wantTopup money.Amount) {
	  t.Helper()
	  var sysFee, userRefund, userTopup money.Amount
	   for _, l := range js.Lines {
		   if l.Asset != asset {
			   continue
		  }
		   switch l.Memo {
		   case feereserve.MemoSystemFee:
			   if l.Credit.IsPositive() {
				  sysFee, _ = sysFee.Add(l.Credit)
			  }
		  case feereserve.MemoUserFeeRefund:
			  if l.Credit.IsPositive() {
				  userRefund,
					   _ = userRefund.Add(l.Credit)
			  }
		  case feereserve.MemoUserFeeTopup:
			  if l.Debit.IsPositive() {
				   userTopup, _ = userTopup.Add(l.Debit)
			   }
		  }
	  }
	   if !sysFee.Equal(wantSysFee) {
		   t.Errorf("system_fee credit: got %s, want %s", sysFee, wantSysFee)
	  }
	  if !userRefund.Equal(wantRefund) {
		   t.Errorf("user_refund credit: got %s, want %s", userRefund, wantRefund)
	   }
	  if !userTopup.Equal(wantTopup) {
		   t.Errorf("user_topup debit: got %s, want %s", userTopup, wantTopup)
	   }
  }

// ────────────────────────────────────────────────────────────────────────────
// 4. ComputeDelta ：Reserved==0 / Actual==0 / 相等
// ────────────────────────────────────────────────────────────────────────────

   func TestFeeReserveComputeDeltaBoundary(t *testing.T) {
	   t.Parallel()
	   cases := []struct {
		  desc string
		  reserved string
		   actual    string
		   wantDir   feereserve.DeltaDir
		   wantDelta string
	  }{
		   {"equal amounts", "10", "10", feereserve.DeltaZero, "0"},
		  {"over: reserved > actual", "10", "6", feereserve.DeltaOver, "4"},
		  {"under: actual > reserved", "6", "10", feereserve.DeltaUnder, "4"},
		   {"tiny over BTC", "0.00010000", "0.00002115", feereserve.DeltaOver, "0.00007885"},
	   }
	   for _, tc := range cases {
		  tc := tc
		   t.Run(tc.desc, func(t *testing.T) {
			   t.Parallel()
			  r := &feereserve.FeeReserve{
				  ID: "boundary-test",
				   WithdrawID: "wd-boundary",
				   Asset:      "USDT",
				   Chain:      "ETH",
				   Reserved: tc.reserved,
				   State:    feereserve.StateReserved,
			  }
			   delta, dir, err := r.ComputeDelta(tc.actual)
			  if err != nil {
				  t.Fatalf("ComputeDelta: %v", err)
			  }
			   if dir != tc.wantDir {
				   t.Errorf("dir: got %q, want %q", dir, tc.wantDir)
			   }
			  want, _ := money.Parse(tc.wantDelta)
			  if !delta.Equal(want) {
				   t.Errorf("delta: got %s, want %s", delta, want)
			  }
		   })
	  }
   }

// ────────────────────────────────────────────────────────────────────────────
// 5. ReserveIn 空字段 / 状态门禁拒绝
// ────────────────────────────────────────────────────────────────────────────

  func TestFeeReserveStateGuards(t *testing.T) {
	   t.Parallel()
	   svc := feereserve.NewMem()
	  ctx := context.Background()
	  in := feereserve.ReserveRequest{
		  WithdrawID: "wd-guard-001",
		  UserID: "user-guard",
		  Asset:  "USDT",
		  Chain: "ETH",
		   ReservedFee: "5",
	  }
	  r, err := svc.Reserve(ctx, in)
	  if err != nil {
		  t.Fatalf("Reserve: %v", err)
	  }
	   // 在 StateReserved 时再次 Reserve（幂等应成功）
	  _, err = svc.Reserve(ctx, in)
	   if err != nil {
		   t.Errorf("idempotent Reserve: %v", err)
	   }
	  // Release → StateReleased
	   rel,
		  err := svc.Release(ctx, r.WithdrawID)
	   if err != nil {
		   t.Fatalf("Release: %v", err)
	   }
	  if rel.State != feereserve.StateReleased {
		  t.Errorf("after Release: state=%s", rel.State)
	   }
	   // 再次 Release 幂等
	   rel2, err := svc.Release(ctx, r.WithdrawID)
	   if err != nil {
		  t.Errorf("idempotent Release after released: %v", err)
	   }
	   if rel2.State != feereserve.StateReleased {
		  t.Errorf("idempotent state: %s", rel2.State)
	   }
	  // 已 released 的记录不能 Settle
	   _, settleErr := svc.Settle(ctx, feereserve.SettleRequest{
		  WithdrawID: r.WithdrawID,
		   ActualFee: "5",
		   Actor: "test",
	  })
	  if settleErr == nil {
		   t.Error("expected error settling a released record, got nil")
	  }
	  if !errors.Is(settleErr, feereserve.ErrState) && !errors.Is(settleErr, feereserve.ErrNotFound) {
		   t.Errorf("settle released: unexpected error type: %v", settleErr)
	   }
  }

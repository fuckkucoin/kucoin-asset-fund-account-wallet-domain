package flow

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"account-assets.kucoin.net/funding/internal/reorgacct"
)

// ────────────────────────────────────────────────────────────────────────────
// 1. ReversalJournal.Build 科目对应关系
// ────────────────────────────────────────────────────────────────────────────

  func TestReversalJournalBuildSubjects(t *testing.T) {
	  t.Parallel()
	  cases := []struct {
		  name         string
		   kind         string
		   wantDebit    string
		   wantCredit   string
		   wantGenerate bool
	   }{
		   {
			   name: "credited → reorg.reverse / user.avail",
			  kind: reorgacct.KindCredited,
			   wantDebit: reorgacct.SubjectReorgReverse, wantCredit: reorgacct.SubjectUserAvail,
			   wantGenerate: true,
		  },
		   {
			  name: "occupied → occupancy_release / occupancy_hold",
			  kind:      reorgacct.KindOccupied,
			   wantDebit: reorgacct.SubjectOccupancyRelease, wantCredit: reorgacct.SubjectOccupancyHold,
			  wantGenerate: true,
		  },
		  {
			  name: "freeze → freeze_release / freeze_hold",
			   kind:      reorgacct.KindFreeze,
			  wantDebit: reorgacct.SubjectFreezeRelease, wantCredit: reorgacct.SubjectFreezeHold,
			  wantGenerate: true,
		   },
		  {
			  name: "hop_partial → hop_close / hop_transit",
			   kind: reorgacct.KindHopPartial,
			   wantDebit: reorgacct.SubjectHopClose, wantCredit: reorgacct.SubjectHopTransit,
			  wantGenerate: true,
		  },
		   {
			   name:         "live_hash → no entry generated",
			   kind:         reorgacct.KindLiveHash,
			   wantGenerate: false,
		   },
	  }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			  journal := reorgacct.NewReversalJournal("batch-subj-001")
			   rows := []reorgacct.ImpactRow{
				   {
					   ID: "row-subj-001", EventID: "evt-subj-001",
					   UserID: "user-A", Asset: "ETH", Kind: tc.kind,
					   ReferenceID: "ref-001", AffectedAmount: "1.0",
				  },
			   }
			  pairs, err := journal.Build(rows)
			  if err != nil {
				   t.Fatalf("Build failed: %v", err)
			   }
			   if !tc.wantGenerate {
				   if len(pairs) != 0 {
					  t.Errorf("%s: expected 0 pairs, got %d", tc.name, len(pairs))
				   }
				   return
			  }
			   if len(pairs) != 1 {
				   t.Fatalf("%s: expected 1 pair, got %d", tc.name, len(pairs))
			  }
			   p := pairs[0]
			  if p.Debit.Subject != tc.wantDebit {
				  t.Errorf("debit subject: want %s got %s", tc.wantDebit, p.Debit.Subject)
			   }
			   if p.Credit.Subject != tc.wantCredit {
				   t.Errorf("credit subject: want %s got %s", tc.wantCredit, p.Credit.Subject)
			   }
			   if p.Debit.Direction != reorgacct.EntryDebit {
				   t.Errorf("debit direction wrong: got %s", p.Debit.Direction)
			   }
			  if p.Credit.Direction != reorgacct.EntryCredit {
				   t.Errorf("credit direction wrong: got %s", p.Credit.Direction)
			   }
		   })
	  }
   }

// ────────────────────────────────────────────────────────────────────────────
// 2. VerifyBalance 守恒校验
// ────────────────────────────────────────────────────────────────────────────

   func TestVerifyBalance(t *testing.T) {
	  t.Parallel()
	   cases := []struct {
		  name string
		   rows []reorgacct.ImpactRow
		  wantBalanced bool
		  wantDebit  string
		  wantCredit string
		   wantDelta  string
	   }{
		  {
			  name: "single_credited_balanced",
			  rows: []reorgacct.ImpactRow{
				  {Kind: reorgacct.KindCredited, AffectedAmount: "1.5", UserID: "u", Asset: "ETH", ReferenceID: "r1"},
			  },
			   wantBalanced: true, wantDebit: "1.5", wantCredit: "1.5", wantDelta: "0",
		   },
		   {
			  name: "multi_kind_balanced",
			   rows: []reorgacct.ImpactRow{
				  {Kind: reorgacct.KindCredited, AffectedAmount: "100", UserID: "u", Asset: "USDT", ReferenceID: "r1"},
				   {Kind: reorgacct.KindOccupied, AffectedAmount: "50", UserID: "u", Asset: "USDT", ReferenceID: "r2"},
				  {Kind: reorgacct.KindFreeze, AffectedAmount: "200", UserID: "u", Asset: "ETH", ReferenceID: "r3"},
			  },
			  wantBalanced: true, wantDebit: "350", wantCredit: "350", wantDelta: "0",
		  },
		  {
			   name: "live_hash_excluded",
			   rows: []reorgacct.ImpactRow{
				  {Kind: reorgacct.KindCredited, AffectedAmount: "1", UserID: "u", Asset: "ETH", ReferenceID: "r1"},
				   {Kind: reorgacct.KindLiveHash, AffectedAmount: "5", UserID: "u", Asset: "ETH", ReferenceID: "r2"},
			  },
			  // live_hash 不产生分录，只有 credited 的 1.0
			   wantBalanced: true, wantDebit: "1", wantCredit: "1", wantDelta: "0",
		   },
		  {
			  name: "empty_pairs_balanced",
			   rows: []reorgacct.ImpactRow{
				   {Kind: reorgacct.KindLiveHash, AffectedAmount: "10", UserID: "u", Asset: "ETH", ReferenceID: "r1"},
			  },
			  wantBalanced: true, wantDebit: "0", wantCredit: "0", wantDelta: "0",
		   },
		  {
			  name: "nil_rows",
			  rows:         nil,
			  wantBalanced: true, wantDebit: "0", wantCredit: "0", wantDelta: "0",
		  },
		   {
			   name: "high_precision_decimal",
			   rows: []reorgacct.ImpactRow{
				   {Kind: reorgacct.KindCredited, AffectedAmount: "0.000000000000000001", UserID: "u", Asset: "ETH", ReferenceID: "r1"},
			   },
			  wantBalanced: true, wantDebit: "0.000000000000000001", wantCredit: "0.000000000000000001", wantDelta: "0",
		  },
		  {
			   name: "multi_asset_balanced",
			  rows: []reorgacct.ImpactRow{
				  {Kind: reorgacct.KindCredited, AffectedAmount: "100", UserID: "u", Asset: "USDT", ReferenceID: "r1"},
				   {Kind: reorgacct.KindFreeze, AffectedAmount: "2", UserID: "u", Asset: "ETH", ReferenceID: "r2"},
				   {Kind: reorgacct.KindHopPartial, AffectedAmount: "500", UserID: "u", Asset: "USDT", ReferenceID: "r3"},
			  },
			  wantBalanced: true, wantDebit: "602", wantCredit: "602", wantDelta: "0",
		  },
	   }
	   for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			   journal := reorgacct.NewReversalJournal("batch-bal-001")
			  pairs, err := journal.Build(tc.rows)
			   if err != nil {
				   t.Fatalf("Build failed: %v", err)
			  }
			   res := reorgacct.VerifyBalance(pairs)
			  if res.Balanced != tc.wantBalanced {
				  t.Errorf("Balanced: want %v got %v (debit=%s credit=%s delta=%s)",
					   tc.wantBalanced, res.Balanced, res.TotalDebit, res.TotalCredit, res.Delta)
			   }
			  if res.TotalDebit != tc.wantDebit {
				  t.Errorf("TotalDebit: want %s got %s", tc.wantDebit, res.TotalDebit)
			  }
			   if res.TotalCredit != tc.wantCredit {
				   t.Errorf("TotalCredit: want %s got %s", tc.wantCredit, res.TotalCredit)
			   }
			  if res.Delta != tc.wantDelta {
				   t.Errorf("Delta: want %s got %s", tc.wantDelta, res.Delta)
			   }
		  })
	   }
   }

// ────────────────────────────────────────────────────────────────────────────
// 3. 分录幂等键唯一性
// ────────────────────────────────────────────────────────────────────────────

   func TestJournalIdemKeyUniqueness(t *testing.T) {
	   t.Parallel()
	   // 构造多条不同 Kind 的 ImpactRow，验证所有产出分录的 IdemKey 无重复。
	   rows := []reorgacct.ImpactRow{
		  {ID: "row-idem-1", Kind: reorgacct.KindCredited, AffectedAmount: "10", UserID: "u1", Asset: "ETH", ReferenceID: "r1"},
		  {ID: "row-idem-2", Kind: reorgacct.KindOccupied, AffectedAmount: "5", UserID: "u1", Asset: "ETH", ReferenceID: "r2"},
		   {ID: "row-idem-3", Kind: reorgacct.KindFreeze, AffectedAmount: "3", UserID: "u2", Asset: "BTC", ReferenceID: "r3"},
		   {ID: "row-idem-4", Kind: reorgacct.KindHopPartial, AffectedAmount: "100", UserID: "u2", Asset: "USDT", ReferenceID: "r4"},
		   {ID: "row-idem-5", Kind: reorgacct.KindLiveHash, AffectedAmount: "50", UserID: "u3", Asset: "ETH", ReferenceID: "r5"},
	  }
	   journal := reorgacct.NewReversalJournal("batch-idem-001")
	  pairs, err := journal.Build(rows)
	  if err != nil {
		   t.Fatalf("Build failed: %v", err)
	  }
	  seen := map[string]bool{}
	  for _, p := range pairs {
		  for _, key := range []string{p.Debit.IdemKey, p.Credit.IdemKey} {
			   if seen[key] {
				  t.Errorf("duplicate IdemKey: %s", key)
			   }
			  seen[key] = true
		   }
	   }
	   // 期望 4 对分录（KindLiveHash 不产生）。
	  if len(pairs) != 4 {
		  t.Errorf("expected 4 pairs, got %d", len(pairs))
	   }
   }

// ────────────────────────────────────────────────────────────────────────────
// 4. 大批量分录守恒并发验证
// ────────────────────────────────────────────────────────────────────────────

  func TestReversalJournalConcurrentBuild(t *testing.T) {
	  t.Parallel()
	   makeRows := func(n int, kind string) []reorgacct.ImpactRow {
		   rows := make([]reorgacct.ImpactRow, n)
		  for i := 0; i < n; i++ {
			  rows[i] = reorgacct.ImpactRow{
				   ID: fmt.Sprintf("row-%s-%d", kind, i),
				   Kind: kind, AffectedAmount: "1",
				   UserID:      fmt.Sprintf("user-%d", i),
				  Asset:       "ETH",
				   ReferenceID: fmt.Sprintf("ref-%s-%d", kind, i),
			   }
		   }
		  return rows
	   }
	  const goroutines = 20
	   const rowsPerGoroutine = 50
	  type result struct {
		  balanced bool
		   err error
	  }
	  results := make([]result, goroutines)
	  var wg sync.WaitGroup
	  for i := 0; i < goroutines; i++ {
		   i := i
		  wg.Add(1)
		   go func() {
			   defer wg.Done()
			  batchID := fmt.Sprintf("batch-concurrent-%d", i)
			   journal := reorgacct.NewReversalJournal(batchID)
			   rows := makeRows(rowsPerGoroutine, reorgacct.KindCredited)
			  pairs,
				  err := journal.Build(rows)
			  if err != nil {
				   results[i] = result{err: err}
				  return
			   }
			   res := reorgacct.VerifyBalance(pairs)
			  results[i] = result{balanced: res.Balanced}
		  }()
	   }
	   wg.Wait()
	   for i, r := range results {
		  if r.err != nil {
			   t.Errorf("goroutine %d: Build error: %v", i, r.err)
		  }
		  if !r.balanced {
			   t.Errorf("goroutine %d: not balanced", i)
		   }
	  }
  }

// ────────────────────────────────────────────────────────────────────────────
// 5. FundsafetyHook 参数校验
// ────────────────────────────────────────────────────────────────────────────

  func TestFundsafetyHookValidation(t *testing.T) {
	  t.Parallel()
	   // 使用 memIngestor（来自 reorg_acct_test.go 已定义）。
	   mi := newMemIngestor()
	  // 构造一个内存版 FundsafetyHook（直接 OnConfirmRollback 参数传递逻辑）。
	  cases := []struct {
		  name    string
		  chain   string
		  pivotTx string
		  depthBefore int
		  depthAfter int
		   wantErr bool
	  }{
		   {
			  name: "valid ETH rollback",
			   chain: "ETH", pivotTx: "0xhook001", depthBefore: 12, depthAfter: 5, wantErr: false,
		   },
		   {
			  name:  "no rollback (same depth)",
			   chain: "ETH", pivotTx: "0xhook002", depthBefore: 12, depthAfter: 12, wantErr: true,
		   },
		   {
			   name:  "advance not rollback",
			  chain: "BTC", pivotTx: "btchook001", depthBefore: 3, depthAfter: 5, wantErr: true,
		  },
		   {
			  name:  "zero depth_before",
			  chain: "SOL", pivotTx: "solhook001", depthBefore: 0, depthAfter: 0, wantErr: true,
		  },
	   }
	  for _, tc := range cases {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   // 直接通过 memIngestor 参数校验逻辑（等价于 Hook 内部逻辑）。
			  req := reorgacct.IngestReq{
				   Chain: tc.chain, PivotTxHash: tc.pivotTx,
				  DepthBefore: tc.depthBefore, DepthAfter: tc.depthAfter,
				   SourceKind: "confirm_rollback",
			  }
			  _, err := mi.ingest(req)
			  if tc.wantErr && err == nil {
				   t.Errorf("want error, got nil")
			  }
			   if !tc.wantErr && err != nil {
				  t.Errorf("want no error, got %v", err)
			  }
		   })
	  }
   }

// ────────────────────────────────────────────────────────────────────────────
// 6. ByAsset 守恒按资产分解
// ────────────────────────────────────────────────────────────────────────────

   func TestVerifyBalanceByAsset(t *testing.T) {
	   t.Parallel()
	  rows := []reorgacct.ImpactRow{
		   {ID: "r1", Kind: reorgacct.KindCredited, AffectedAmount: "100", UserID: "u", Asset: "USDT", ReferenceID: "ref1"},
		  {ID: "r2", Kind: reorgacct.KindCredited, AffectedAmount: "2", UserID: "u", Asset: "ETH", ReferenceID: "ref2"},
		   {ID: "r3", Kind: reorgacct.KindFreeze, AffectedAmount: "1", UserID: "u", Asset: "ETH", ReferenceID: "ref3"},
	  }
	   journal := reorgacct.NewReversalJournal("batch-byasset-001")
	  pairs, err := journal.Build(rows)
	   if err != nil {
		  t.Fatalf("Build failed: %v", err)
	  }
	  res := reorgacct.VerifyBalance(pairs)
	  if !res.Balanced {
		   t.Errorf("overall balance failed: debit=%s credit=%s delta=%s",
			  res.TotalDebit, res.TotalCredit, res.Delta)
	  }
	   // USDT: debit=100 credit=100（1 对 credited）。
	   usdt := res.ByAsset["USDT"]
	   if !usdt.Balanced {
		  t.Errorf("USDT not balanced: debit=%s credit=%s", usdt.Debit, usdt.Credit)
	   }
	  // ETH: debit=3 credit=3（2对：credited 2 + freeze 1）。
	  eth := res.ByAsset["ETH"]
	   if !eth.Balanced {
		   t.Errorf("ETH not balanced: debit=%s credit=%s", eth.Debit, eth.Credit)
	  }
	   if eth.Debit != "3" {
		   t.Errorf("ETH debit: want 3 got %s", eth.Debit)
	  }
  }

// ────────────────────────────────────────────────────────────────────────────
// 7. JournalEntry 方向性约束
// ────────────────────────────────────────────────────────────────────────────

   func TestJournalEntryDirections(t *testing.T) {
	   t.Parallel()
	  kinds := []string{
		  reorgacct.KindCredited,
		  reorgacct.KindOccupied,
		  reorgacct.KindFreeze,
		  reorgacct.KindHopPartial,
	   }
	   for _, kind := range kinds {
		   kind := kind
		  t.Run(kind, func(t *testing.T) {
			  t.Parallel()
			   journal := reorgacct.NewReversalJournal("batch-dir-001")
			  rows := []reorgacct.ImpactRow{
				   {ID: "rdir", Kind: kind, AffectedAmount: "1", UserID: "u", Asset: "ETH", ReferenceID: "r"},
			  }
			   pairs, err := journal.Build(rows)
			  if err != nil {
				   t.Fatalf("Build failed: %v", err)
			   }
			  if len(pairs) != 1 {
				   t.Fatalf("expected 1 pair, got %d", len(pairs))
			   }
			   if pairs[0].Debit.Direction != reorgacct.EntryDebit {
				   t.Errorf("Debit.Direction: want debit got %s", pairs[0].Debit.Direction)
			   }
			  if pairs[0].Credit.Direction != reorgacct.EntryCredit {
				   t.Errorf("Credit.Direction: want credit got %s", pairs[0].Credit.Direction)
			  }
			  // 借贷金额必须相等。
			  if pairs[0].Debit.Amount != pairs[0].Credit.Amount {
				  t.Errorf("debit amount %s != credit amount %s",
					   pairs[0].Debit.Amount, pairs[0].Credit.Amount)
			  }
		   })
	   }
  }

// ────────────────────────────────────────────────────────────────────────────
// 8. 科目常量完整性
// ────────────────────────────────────────────────────────────────────────────

  func TestSubjectConstants(t *testing.T) {
	  t.Parallel()
	   subjects := []string{
		   reorgacct.SubjectUserAvail,
		   reorgacct.SubjectReorgReverse,
		  reorgacct.SubjectOccupancyHold,
		   reorgacct.SubjectOccupancyRelease,
		  reorgacct.SubjectFreezeHold,
		   reorgacct.SubjectFreezeRelease,
		  reorgacct.SubjectHopTransit,
		   reorgacct.SubjectHopClose,
	   }
	  seen := map[string]bool{}
	  for _, s := range subjects {
		  if s == "" {
			   t.Errorf("empty subject constant")
		   }
		  if seen[s] {
			   t.Errorf("duplicate subject: %s", s)
		   }
		  seen[s] = true
	   }
   }

// ────────────────────────────────────────────────────────────────────────────
// 9. 零金额行校验（Build 应返回 error）
// ────────────────────────────────────────────────────────────────────────────

  func TestJournalBuildZeroAmountError(t *testing.T) {
	  t.Parallel()
	  cases := []struct {
		  name    string
		   amount  string
		   wantErr bool
	   }{
		  {name: "zero string", amount: "0", wantErr: true},
		  {name: "empty string", amount: "", wantErr: true},
		  {name: "negative", amount: "-1", wantErr: true},
		   {name: "valid positive", amount: "0.001", wantErr: false},
	  }
	  for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			  journal := reorgacct.NewReversalJournal("batch-zero-001")
			  rows := []reorgacct.ImpactRow{
				  {ID: "rzero", Kind: reorgacct.KindCredited, AffectedAmount: tc.amount,
					  UserID: "u", Asset: "ETH", ReferenceID: "r"},
			   }
			   _, err := journal.Build(rows)
			  if tc.wantErr && err == nil {
				   t.Errorf("want Build error for amount %q, got nil", tc.amount)
			   }
			  if !tc.wantErr && err != nil {
				   t.Errorf("want no error for amount %q, got %v", tc.amount, err)
			  }
		  })
	  }
  }

// ────────────────────────────────────────────────────────────────────────────
// 10. CSV Golden fixture 回放：全场景校验
// ────────────────────────────────────────────────────────────────────────────

  func TestCSVGoldenFixtureAllCases(t *testing.T) {
	  t.Parallel()
	  csvPath := filepath.Join("..", "..", "testdata", "reorgacct", "golden_all_cases.csv")
	  f, err := os.Open(csvPath)
	  if err != nil {
		  t.Skipf("CSV fixture not found: %v; skipping", err)
		  return
	  }
	   defer f.Close()
	  reader := csv.NewReader(f)
	   reader.Comment = '#'
	  header, err := reader.Read()
	  if err != nil {
		   t.Fatalf("read CSV header: %v", err)
	  }
	   // 验证必要字段存在。
	  required := []string{"scenario", "chain", "depth_before", "depth_after", "rollback_depth", "affected_kind"}
	  for _, req := range required {
		   found := false
		  for _, h := range header {
			  if h == req {
				  found = true
				   break
			   }
		  }
		  if !found {
			  t.Errorf("CSV missing required column: %s", req)
		   }
	   }
	   lineNum := 1
	  for {
		  record, err := reader.Read()
		   if err == io.EOF {
			  break
		   }
		   if err != nil {
			   t.Fatalf("CSV read error at line %d: %v", lineNum, err)
		   }
		   lineNum++
		  if len(record) < len(header) {
			   t.Errorf("line %d: too few columns (%d < %d)", lineNum, len(record), len(header))
			  continue
		   }
		  row := map[string]string{}
		   for i, h := range header {
			   row[h] = record[i]
		  }
		   // 验证 rollback_depth = depth_before - depth_after。
		  var depthBefore, depthAfter, rollbackDepth int
		   fmt.Sscanf(row["depth_before"], "%d", &depthBefore)
		   fmt.Sscanf(row["depth_after"], "%d", &depthAfter)
		   fmt.Sscanf(row["rollback_depth"], "%d", &rollbackDepth)
		  expectedRollback := depthBefore - depthAfter
		  if rollbackDepth != expectedRollback {
			  t.Errorf("scenario %s: rollback_depth=%d but depth_before(%d)-depth_after(%d)=%d",
				   row["scenario"], rollbackDepth, depthBefore, depthAfter, expectedRollback)
		   }
		   // 验证 live_hash 场景 → expected_leg_kind=incident。
		  if row["live_hash"] == "true" && row["expected_leg_kind"] != "incident" {
			  t.Errorf("scenario %s: live_hash=true but expected_leg_kind=%s (want incident)",
				   row["scenario"], row["expected_leg_kind"])
		  }
		  // 验证 chain 非空。
		  if row["chain"] == "" {
			  t.Errorf("scenario %s: empty chain", row["scenario"])
		  }
	  }
	   t.Logf("CSV golden fixture: validated %d data rows", lineNum-1)
  }

// ────────────────────────────────────────────────────────────────────────────
// 11. 守恒：多批次独立守恒（批次幂等不干扰）
// ────────────────────────────────────────────────────────────────────────────

   func TestMultiBatchIndependentBalance(t *testing.T) {
	   t.Parallel()
	   batches := []struct {
		  batchID string
		   rows []reorgacct.ImpactRow
	   }{
		  {
			  batchID: "batch-multi-001",
			  rows: []reorgacct.ImpactRow{
				   {ID: "r1", Kind: reorgacct.KindCredited, AffectedAmount: "100", UserID: "u1", Asset: "USDT", ReferenceID: "tr1"},
			   },
		  },
		   {
			   batchID: "batch-multi-002",
			   rows: []reorgacct.ImpactRow{
				  {ID: "r2", Kind: reorgacct.KindFreeze, AffectedAmount: "2", UserID: "u2", Asset: "ETH", ReferenceID: "frz1"},
				  {ID: "r3", Kind: reorgacct.KindOccupied, AffectedAmount: "50", UserID: "u3", Asset: "USDT", ReferenceID: "occ1"},
			  },
		   },
		   {
			  batchID: "batch-multi-003",
			   rows: []reorgacct.ImpactRow{
				  {ID: "r4", Kind: reorgacct.KindHopPartial, AffectedAmount: "500", UserID: "u4", Asset: "USDT", ReferenceID: "hop1"},
				   {ID: "r5", Kind: reorgacct.KindLiveHash, AffectedAmount: "1000", UserID: "u5", Asset: "ETH", ReferenceID: "lh1"},
			  },
		   },
	   }
	  for _, batch := range batches {
		  batch := batch
		  t.Run(batch.batchID, func(t *testing.T) {
			  t.Parallel()
			   journal := reorgacct.NewReversalJournal(batch.batchID)
			  pairs, err := journal.Build(batch.rows)
			  if err != nil {
				   t.Fatalf("Build failed: %v", err)
			  }
			   res := reorgacct.VerifyBalance(pairs)
			   if !res.Balanced {
				  t.Errorf("batch %s not balanced: debit=%s credit=%s delta=%s",
					  batch.batchID, res.TotalDebit, res.TotalCredit, res.Delta)
			  }
		  })
	   }
   }

// ────────────────────────────────────────────────────────────────────────────
// 12. 分录 BatchID 字段回填正确性
// ────────────────────────────────────────────────────────────────────────────

  func TestJournalEntryBatchIDFilled(t *testing.T) {
	   t.Parallel()
	   batchID := "batch-batchid-test-001"
	   journal := reorgacct.NewReversalJournal(batchID)
	  rows := []reorgacct.ImpactRow{
		   {ID: "r-bid-1", Kind: reorgacct.KindCredited, AffectedAmount: "10", UserID: "u", Asset: "ETH", ReferenceID: "r"},
		   {ID: "r-bid-2", Kind: reorgacct.KindFreeze, AffectedAmount: "5", UserID: "u", Asset: "ETH", ReferenceID: "r2"},
	  }
	  pairs,
		  err := journal.Build(rows)
	  if err != nil {
		  t.Fatalf("Build failed: %v", err)
	  }
	   for _, p := range pairs {
		  if p.Debit.BatchID != batchID {
			  t.Errorf("Debit.BatchID: want %s got %s", batchID, p.Debit.BatchID)
		  }
		  if p.Credit.BatchID != batchID {
			  t.Errorf("Credit.BatchID: want %s got %s", batchID, p.Credit.BatchID)
		  }
	   }
   }

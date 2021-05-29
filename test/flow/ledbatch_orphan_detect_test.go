package flow

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/ledorphan"
)

// ── 辅助 ──────────────────────────────────────────────────────────────────
// mockBatchObserver 实现 ledorphan.BatchObserver（内存存根）。
type mockBatchObserver struct {
	batches map[string]ledorphan.BatchSnapshot
}

   func (m *mockBatchObserver) GetBatch(_ context.Context, batchID string) (ledorphan.BatchSnapshot, error) {
	  b, ok := m.batches[batchID]
	  if !ok {
		   return ledorphan.BatchSnapshot{}, ledorphan.ErrBatchNotFound
	   }
	  return b, nil
   }

   func (m *mockBatchObserver) ListBatches(_ context.Context, statuses []string, after string, limit int) ([]ledorphan.BatchSnapshot, error) {
	  statusSet := make(map[string]bool)
	  for _, s := range statuses {
		  statusSet[s] = true
	   }
	  result := make([]ledorphan.BatchSnapshot,
		   0)
	  for _, b := range m.batches {
		   if len(statusSet) > 0 && !statusSet[b.Status] {
			  continue
		  }
		  result = append(result, b)
	   }
	   return result, nil
  }

// mockLedgerObserver 实现 ledorphan.LedgerObserver（内存存根）。

type mockLedgerObserver struct {
	// idemToEntry: idem_key → ledger_entry_id（不在此 map 则视为未落账）
	idemToEntry map[string]string
}

  func (m *mockLedgerObserver) LookupHopByIdem(_ context.Context, idemKey string) (string, error) {
	  return m.idemToEntry[idemKey], nil
   }

   func (m *mockLedgerObserver) BatchLookupHopsByIdem(_ context.Context, idemKeys []string) (map[string]string, error) {
	   result := make(map[string]string)
	   for _, k := range idemKeys {
		  if id, ok := m.idemToEntry[k]; ok {
			   result[k] = id
		   }
	   }
	  return result, nil
  }

// makeBatch 构造用 BatchSnapshot。
  func makeBatch(id, status string, items ...ledorphan.BatchItemSnapshot) ledorphan.BatchSnapshot {
	   return ledorphan.BatchSnapshot{
		   ID:     id,
		   Status: status,
		  Actor:     "test-actor",
		  Requested: "100",
		   Items:     items,
		   CreatedAt: time.Now().UTC().Add(-10 * time.Minute),
		   UpdatedAt: time.Now().UTC(),
	  }
  }

// makeItem 构造 BatchItemSnapshot。

   func makeItem(seq int, idem, status, ledgerID string) ledorphan.BatchItemSnapshot {
	  return ledorphan.BatchItemSnapshot{
		   Seq:    seq,
		   UserID: "user-001",
		   Asset:  "USDT",
		  Amount: "10",
		  Direction:      "funding_to_trading",
		  IdempotencyKey: idem,
		   Status:         status,
		   LedgerID: ledgerID,
	   }
   }

// newDetector 构造用 Detector。
  func newDetector(bo *mockBatchObserver, lo *mockLedgerObserver) *ledorphan.Detector {
	   return ledorphan.NewDetector(bo, lo)
   }

// ── 用例 ──────────────────────────────────────────────────────────────────
// TestDetect_NoOrphans 全量已提交，无孤儿。

  func TestDetect_NoOrphans(t *testing.T) {
	  bo := &mockBatchObserver{batches: map[string]ledorphan.BatchSnapshot{
		  "batch-001": makeBatch("batch-001", "submitted",
			  makeItem(1, "idem-1", "submitted", "led-1"),
			  makeItem(2, "idem-2", "submitted", "led-2"),
			  makeItem(3, "idem-3", "skipped", ""),
		  ),
	   }}
	   lo := &mockLedgerObserver{idemToEntry: map[string]string{
		  "idem-1": "led-1",
		   "idem-2": "led-2",
	   }}
	   d := newDetector(bo, lo)
	   ctx := context.Background()
	  rep, err := d.Detect(ctx, "batch-001", 1)
	  if err != nil {
		  t.Fatalf("unexpected error: %v", err)
	  }
	  if rep.TotalOrphans != 0 {
		  t.Errorf("expected 0 orphans, got %d", rep.TotalOrphans)
	   }
	  if rep.BatchID != "batch-001" {
		  t.Errorf("unexpected batch_id: %s", rep.BatchID)
	   }
   }

// TestDetect_HangingOrphan batch 标记 submitted 但 ledger 无记录。

  func TestDetect_HangingOrphan(t *testing.T) {
	   bo := &mockBatchObserver{batches: map[string]ledorphan.BatchSnapshot{
		  "batch-002": makeBatch("batch-002", "submitted",
			  makeItem(1, "idem-1", "submitted", "led-1"), // 正常
			  makeItem(2, "idem-2", "submitted", "led-2"), // hanging：batch 说 submitted 但 ledger 无记录
		   ),
	  }}
	  lo := &mockLedgerObserver{idemToEntry: map[string]string{
		   "idem-1": "led-1",
		  // idem-2 不在 ledger 中
	   }}
	  d := newDetector(bo, lo)
	   ctx := context.Background()
	   rep, err := d.Detect(ctx, "batch-002", 1)
	  if err != nil {
		   t.Fatalf("unexpected error: %v", err)
	  }
	   if rep.TotalOrphans != 1 {
		   t.Errorf("expected 1 orphan, got %d", rep.TotalOrphans)
	  }
	  if rep.HangingCount != 1 {
		   t.Errorf("expected 1 hanging, got %d", rep.HangingCount)
	   }
	  if rep.Items[0].Kind != ledorphan.OrphanHanging {
		  t.Errorf("expected OrphanHanging, got %s", rep.Items[0].Kind)
	  }
	   if rep.Items[0].IdempotencyKey != "idem-2" {
		   t.Errorf("unexpected idem key: %s", rep.Items[0].IdempotencyKey)
	  }
  }

// TestDetect_MissingOrphan ledger 已落账但 batch 状态仍为 pending。

  func TestDetect_MissingOrphan(t *testing.T) {
	  bo := &mockBatchObserver{batches: map[string]ledorphan.BatchSnapshot{
		   "batch-003": makeBatch("batch-003", "submitted",
			  makeItem(1, "idem-1", "submitted", "led-1"), // 正常
			  makeItem(2, "idem-2", "pending", ""), // missing：ledger 有记录但 batch 是 pending
		  ),
	  }}
	  lo := &mockLedgerObserver{idemToEntry: map[string]string{
		  "idem-1": "led-1",
		  "idem-2": "led-x", // ledger 中有 idem-2
	  }}
	  d := newDetector(bo, lo)
	  ctx := context.Background()
	  rep, err := d.Detect(ctx, "batch-003", 1)
	   if err != nil {
		   t.Fatalf("unexpected error: %v", err)
	   }
	  if rep.TotalOrphans != 1 {
		   t.Errorf("expected 1 orphan, got %d", rep.TotalOrphans)
	  }
	  if rep.MissingCount != 1 {
		  t.Errorf("expected 1 missing, got %d", rep.MissingCount)
	  }
	  if rep.Items[0].Kind != ledorphan.OrphanMissing {
		   t.Errorf("expected OrphanMissing, got %s", rep.Items[0].Kind)
	   }
	  if rep.Items[0].LedgerID != "led-x" {
		   t.Errorf("expected ledger_id=led-x, got %s", rep.Items[0].LedgerID)
	   }
   }

// TestDetect_MixedOrphans Hanging + Missing 混合场景。
  func TestDetect_MixedOrphans(t *testing.T) {
	   bo := &mockBatchObserver{batches: map[string]ledorphan.BatchSnapshot{
		   "batch-004": makeBatch("batch-004", "submitted",
			  makeItem(1, "idem-1", "submitted", "led-1"), // 正常
			  makeItem(2, "idem-2", "submitted", "led-2"), // hanging
			  makeItem(3, "idem-3", "pending", ""),        // missing
			  makeItem(4, "idem-4", "skipped", ""), // skipped，不是孤儿
		   ),
	  }}
	   lo := &mockLedgerObserver{idemToEntry: map[string]string{
		   "idem-1": "led-1",
		   // idem-2 不在 ledger
		  "idem-3": "led-3",
	   }}
	  d := newDetector(bo, lo)
	  ctx := context.Background()
	   rep, err := d.Detect(ctx, "batch-004", 1)
	   if err != nil {
		   t.Fatalf("unexpected error: %v", err)
	  }
	   if rep.TotalOrphans != 2 {
		   t.Errorf("expected 2 orphans, got %d", rep.TotalOrphans)
	   }
	   if rep.HangingCount != 1 || rep.MissingCount != 1 {
		  t.Errorf("unexpected hanging=%d missing=%d", rep.HangingCount, rep.MissingCount)
	   }
   }

// TestDetect_EmptyIdemKey idem key 为空的 item 被跳过。
   func TestDetect_EmptyIdemKey(t *testing.T) {
	   bo := &mockBatchObserver{batches: map[string]ledorphan.BatchSnapshot{
		  "batch-005": makeBatch("batch-005", "submitted",
			  makeItem(1, "", "submitted", ""), // idem key 空，不定性为孤儿
		  ),
	   }}
	   lo := &mockLedgerObserver{idemToEntry: map[string]string{}}
	  d := newDetector(bo, lo)
	  ctx := context.Background()
	  rep, err := d.Detect(ctx, "batch-005", 1)
	   if err != nil {
		  t.Fatalf("unexpected error: %v", err)
	  }
	   if rep.TotalOrphans != 0 {
		   t.Errorf("expected 0 orphans for empty idem, got %d", rep.TotalOrphans)
	  }
   }

// TestDetect_NonSubmittedBatch open/halted/canceled 批次不运行孤儿检测。

   func TestDetect_NonSubmittedBatch(t *testing.T) {
	   cases := []string{"open", "halted", "canceled"}
	   bo := &mockBatchObserver{batches: map[string]ledorphan.BatchSnapshot{}}
	   lo := &mockLedgerObserver{idemToEntry: map[string]string{}}
	   for _, status := range cases {
		   bo.batches["batch-x"] = makeBatch("batch-x", status,
			   makeItem(1, "idem-1", "pending", ""),
		  )
		   d := newDetector(bo, lo)
		  _, err := d.Detect(context.Background(), "batch-x", 0)
		   if err == nil {
			   t.Errorf("status=%s: expected error, got nil", status)
		   }
	   }
   }

// TestDetect_BatchNotFound 不存在的批次返回错误。
   func TestDetect_BatchNotFound(t *testing.T) {
	  bo := &mockBatchObserver{batches: map[string]ledorphan.BatchSnapshot{}}
	  lo := &mockLedgerObserver{idemToEntry: map[string]string{}}
	  d := newDetector(bo, lo)
	  _, err := d.Detect(context.Background(), "nonexistent", 0)
	   if err == nil {
		  t.Fatal("expected error for nonexistent batch")
	  }
  }

// TestDetect_Determinism 同一批次多次检测产出一致的哈希。
  func TestDetect_Determinism(t *testing.T) {
	   bo := &mockBatchObserver{batches: map[string]ledorphan.BatchSnapshot{
		  "batch-det": makeBatch("batch-det", "submitted",
			   makeItem(1, "idem-a", "submitted", ""),
			  makeItem(2, "idem-b", "submitted", ""),
			   makeItem(3, "idem-c", "pending", ""),
		  ),
	   }}
	  lo := &mockLedgerObserver{idemToEntry: map[string]string{
		  "idem-c": "led-c",
	  }}
	  d := newDetector(bo, lo)
	   ctx := context.Background()
	   rep1, err := d.Detect(ctx, "batch-det", 1)
	   if err != nil {
		  t.Fatal(err)
	  }
	  rep2, err := d.Detect(ctx, "batch-det", 1)
	   if err != nil {
		  t.Fatal(err)
	   }
	   if rep1.IntentHash != rep2.IntentHash {
		   t.Errorf("intent hash not deterministic: %s != %s", rep1.IntentHash, rep2.IntentHash)
	   }
	  if rep1.LedgerHash != rep2.LedgerHash {
		   t.Errorf("ledger hash not deterministic: %s != %s", rep1.LedgerHash, rep2.LedgerHash)
	   }
	  if rep1.TotalOrphans != rep2.TotalOrphans {
		   t.Errorf("orphan count not deterministic: %d != %d", rep1.TotalOrphans, rep2.TotalOrphans)
	  }
  }

// TestDetect_NotLedgerDiff 确认本包只查询 ledbatch 自身行，不做全账本差分。
// 方式：即使 ledger 中有大量其他 idem 记录，本包只关注批次中的 idem keys。
   func TestDetect_NotLedgerDiff(t *testing.T) {
	  bo := &mockBatchObserver{batches: map[string]ledorphan.BatchSnapshot{
		  "batch-iso": makeBatch("batch-iso", "submitted",
			   makeItem(1, "batch-idem-1", "submitted", "led-1"),
		   ),
	   }}
	   // ledger 中有大量不属于本批次的记录（全账本有其他分录）。
	   lo := &mockLedgerObserver{idemToEntry: map[string]string{
		   "batch-idem-1": "led-1",
		  "other-batch-idem-x": "led-x",
		  "leddiff-idem-y":     "led-y",
		  "invcheck-idem-z": "led-z",
	  }}
	   d := newDetector(bo, lo)
	  ctx := context.Background()
	  rep, err := d.Detect(ctx, "batch-iso", 1)
	   if err != nil {
		   t.Fatal(err)
	   }
	   // 虽然 ledger 有很多记录，本包只关注 batch-iso 中的 item。
	  if rep.TotalOrphans != 0 {
		   t.Errorf("expected 0 orphans (isolation check), got %d", rep.TotalOrphans)
	  }
  }

// TestDetect_LargePartialSubmit 大批次半提交（50 items，前 30 已落账，后 20 未落账）。
  func TestDetect_LargePartialSubmit(t *testing.T) {
	   const total = 50
	  const submitted = 30
	   items := make([]ledorphan.BatchItemSnapshot, total)
	  for i := 0; i < total; i++ {
		  idem := "idem-" + string(rune('a'+i))
		  if i < submitted {
			  items[i] = makeItem(i+1, idem, "submitted", "led-"+string(rune('a'+i)))
		   } else {
			  items[i] = makeItem(i+1, idem, "pending", "")
		   }
	  }
	  bo := &mockBatchObserver{batches: map[string]ledorphan.BatchSnapshot{
		  "batch-large": {ID: "batch-large", Status: "submitted", Items: items, CreatedAt: time.Now().UTC()},
	  }}
	   // ledger 只有前 30 条记录。
	  ledMap := make(map[string]string, submitted)
	  for i := 0; i < submitted; i++ {
		  idem := "idem-" + string(rune('a'+i))
		   ledMap[idem] = "led-" + string(rune('a'+i))
	   }
	  lo := &mockLedgerObserver{idemToEntry: ledMap}
	  d := newDetector(bo, lo)
	  ctx := context.Background()
	   rep, err := d.Detect(ctx, "batch-large", 1)
	   if err != nil {
		  t.Fatal(err)
	   }
	  // 后 20 items 在 batch 是 pending 且 ledger 无记录 → 不是孤儿（仅 pending）。
	   // 前 30 items 在 batch 是 submitted 且 ledger 有记录 → 正常。
	  // 期望孤儿为 0（因为 pending+无ledger 不是孤儿，只有 submitted+无ledger 才是 hanging）。
	   if rep.TotalOrphans != 0 {
		   t.Errorf("expected 0 orphans for clean pending-only case, got %d", rep.TotalOrphans)
	   }
   }

// TestComputeHash_Stability 完整性哈希算法稳定性（不同顺序输入产出同一哈希）。

   func TestComputeHash_Stability(t *testing.T) {
	   intent := []string{"idem-c", "idem-a", "idem-b"}
	   ledger := []string{"idem-a", "idem-b"}
	  seal := []string{"idem-c"}
	  h1 := ledorphan.ComputeHash(intent,
		   ledger, seal)
	  // 改变输入顺序。
	  intent2 := []string{"idem-a", "idem-c", "idem-b"}
	  h2 := ledorphan.ComputeHash(intent2, ledger, seal)
	  if h1 != h2 {
		  t.Errorf("hash not stable across input orderings: %s != %s", h1, h2)
	  }
   }

// TestVerifyHash_Mismatch 哈希不匹配返回 ErrProofMismatch。
   func TestVerifyHash_Mismatch(t *testing.T) {
	  intent := []string{"idem-1", "idem-2"}
	   ledger := []string{"idem-1"}
	   seal := []string{"idem-2"}
	  correct := ledorphan.ComputeHash(intent, ledger, seal)
	   // 传入错误哈希。
	   err := ledorphan.VerifyHash(intent, ledger, seal, "deadbeef0000000000000000000000000000000000000000000000000000dead")
	   if err == nil {
		  t.Fatal("expected ErrProofMismatch for wrong hash")
	   }
	  // 传入正确哈希。
	  if err := ledorphan.VerifyHash(intent, ledger, seal, correct); err != nil {
		  t.Errorf("unexpected error for correct hash: %v", err)
	  }
  }

package flow

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/ledseal"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/snap"
)

// ─────────────────────────────────────────────────────────
// 辅助函数
// ─────────────────────────────────────────────────────────
// makePos 快速构造 snap.Position。

   func makePos(book, asset, avail, frozen string) snap.Position {
	   return snap.Position{
		   Book: book, Asset: asset, Available: avail, Frozen: frozen,
	   }
   }

// makeSnap 构建一个带指定 Position 的快照并存入 st（userID、checksum 已填）。

  func makeSnap(st *snap.Store, userID string, positions []snap.Position, takenAt time.Time) snap.Snapshot {
	   views := make([]ledger.BookView, len(positions))
	   for i, p := range positions {
		  views[i] = ledger.BookView{
			   Book: p.Book, Asset: p.Asset,
			   Available: p.Available, Frozen: p.Frozen,
		   }
	  }
	   snp, err := snap.CaptureViews(userID, views, takenAt)
	  if err != nil {
		   panic("makeSnap: " + err.Error())
	  }
	   ctx := context.Background()
	   snp, err = st.Put(ctx, snp)
	   if err != nil {
		  panic("makeSnap put: " + err.Error())
	  }
	  return snp
   }

// buildSealEnv 组装共享 snap.Store 和 ledseal.Service。
   func buildSealEnv() (*snap.Store, *ledseal.Service) {
	  st := snap.NewMemStore()
	   snapSvc := snap.NewWithStore(nil, nil, st)
	   svc := ledseal.New(snapSvc, ledseal.NewMemStore())
	  return st, svc
  }

// buildSealEnv2 从已有 store 构建 ledseal.Service（辅助）。

   func buildSealEnv2(st *snap.Store) *ledseal.Service {
	   snapSvc := snap.NewWithStore(nil,
		   nil, st)
	  return ledseal.New(snapSvc, ledseal.NewMemStore())
  }

// ─────────────────────────────────────────────────────────
// Fixture 1：两代调度快照，无漂移 → SealRoot 验通，DiffBatch.DriftCount == 0
// ─────────────────────────────────────────────────────────

   func TestLedSeal_NoDrift_SealRootVerifies(t *testing.T) {
	  ctx := context.Background()
	  uid := "user-fixture-1"
	   t0 := time.Date(2022, 6, 14, 10, 0, 0, 0, time.UTC)
	   t1 := t0.Add(1 * time.Hour)
	  // 两代快照数据相同（无漂移）。
	  positions := []snap.Position{
		   makePos("MAIN", "USDT", "1000.00", "0"),
		  makePos("MAIN", "BTC", "0.5", "0"),
		   makePos("TRADE", "USDT", "200.00", "50.00"),
	  }
	  st := snap.NewMemStore()
	  snp0 := makeSnap(st, uid, positions, t0)
	  snp1 := makeSnap(st, uid, positions, t1)
	   svc := buildSealEnv2(st)
	   clock := func() time.Time { return t1 }
	  svc.WithClock(clock)
	  // 创建计划。
	  plan, err := svc.CreatePlan(ctx, ledseal.CreatePlanReq{
		  Name: "test-no-drift", PlanScope: ledseal.ScopeUserShard,
		  ShardKey: uid, IntervalSeconds: 3600, CreatedBy: "test",
	  })
	  if err != nil {
		  t.Fatalf("create plan: %v", err)
	   }
	  // 注册两代切片。
	  sl0, err := svc.RegisterSlice(ctx, plan.ID, snp0.ID, uid, "",
		   snp0.Checksum, snp0.Count,
		  snp0.Fence.Epoch,
		  snp0.Fence.Token, t0)
	   if err != nil {
		   t.Fatalf("register slice 0: %v", err)
	   }
	  sl1, err := svc.RegisterSlice(ctx, plan.ID, snp1.ID, uid, "",
		   snp1.Checksum, snp1.Count,
		  snp1.Fence.Epoch, snp1.Fence.Token, t1)
	   if err != nil {
		  t.Fatalf("register slice 1: %v", err)
	   }
	  // 开启批次并执行差分。
	   batch, err := svc.OpenBatch(ctx, plan.ID, sl0.ID, sl1.ID, sl0.GenSeq, sl1.GenSeq)
	   if err != nil {
		   t.Fatalf("open batch: %v", err)
	  }
	   batch, root, err := svc.RunAndSealBatch(ctx, batch.ID)
	   if err != nil {
		   t.Fatalf("run and seal: %v", err)
	   }
	  // 断言：无漂移
	   if batch.DriftCount != 0 {
		  t.Errorf("want DriftCount=0, got %d", batch.DriftCount)
	  }
	   if batch.State != ledseal.BatchSealed {
		  t.Errorf("want state=sealed, got %s", batch.State)
	   }
	  if root.DriftCount != 0 {
		   t.Errorf("want root.DriftCount=0, got %d", root.DriftCount)
	   }
	   if root.RootHash == "" {
		  t.Error("want non-empty root_hash")
	  }
	   if batch.MaxSeverity != ledseal.SeverityNone {
		   t.Errorf("want severity=none, got %s", batch.MaxSeverity)
	  }
	   // 验证封印根完整性。
	   if err := svc.VerifyRoot(ctx, root.ID); err != nil {
		  t.Errorf("verify root: %v", err)
	   }
	  t.Logf("Fixture1 OK: batch=%s root=%s rootHash=%s", batch.ID, root.ID, root.RootHash)
  }

// ─────────────────────────────────────────────────────────
// Fixture 2：注入漂移行 → 评级告警；可发 block_write 建议（不写 cover）
// ─────────────────────────────────────────────────────────

  func TestLedSeal_InjectedDrift_GradeAlert(t *testing.T) {
	   ctx := context.Background()
	   uid := "user-fixture-2"
	  t0 := time.Date(2022,
		  6, 14, 11, 0, 0, 0, time.UTC)
	   t1 := t0.Add(1 * time.Hour)
	  // 第一代：USDT 100，BTC 0.1
	   pos0 := []snap.Position{
		   makePos("MAIN", "USDT", "100.00", "0"),
		   makePos("MAIN", "BTC", "0.1", "0"),
	  }
	   // 第二代：USDT 被神秘增加了 50000（超阈值 → critical + block_write_hint）
	   pos1 := []snap.Position{
		   makePos("MAIN", "USDT", "50100.00", "0"), // delta=50000 >> threshold
		  makePos("MAIN", "BTC", "0.1", "0"),       // 无变化
	   }
	   st := snap.NewMemStore()
	   snp0 := makeSnap(st, uid, pos0, t0)
	   snp1 := makeSnap(st, uid, pos1, t1)
	  svc := buildSealEnv2(st)
	   svc.WithClock(func() time.Time { return t1 })
	  plan, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{
		  Name: "test-injected-drift", PlanScope: ledseal.ScopeUserShard,
		   ShardKey: uid, IntervalSeconds: 3600, CreatedBy: "test",
	   })
	   sl0, _ := svc.RegisterSlice(ctx, plan.ID, snp0.ID, uid, "", snp0.Checksum, snp0.Count, 0, "", t0)
	  sl1, _ := svc.RegisterSlice(ctx, plan.ID, snp1.ID, uid, "", snp1.Checksum, snp1.Count, 0, "", t1)
	  batch, err := svc.OpenBatch(ctx, plan.ID, sl0.ID, sl1.ID, sl0.GenSeq, sl1.GenSeq)
	  if err != nil {
		  t.Fatalf("open batch: %v", err)
	   }
	  batch, root, err := svc.RunAndSealBatch(ctx, batch.ID)
	   if err != nil {
		  t.Fatalf("run and seal: %v", err)
	   }
	   // 断言：有漂移
	  if batch.DriftCount == 0 {
		  t.Error("want DriftCount > 0")
	   }
	  if batch.State != ledseal.BatchSealed {
		  t.Errorf("want state=sealed, got %s", batch.State)
	   }
	   // 断言：critical 级别（50000 USDT delta 超过阈值 10000）
	  if batch.MaxSeverity != ledseal.SeverityCritical {
		  t.Errorf("want max_severity=critical, got %s", batch.MaxSeverity)
	  }
	   // 检查漂移行：USDT 行应有 block_write_hint
	   rows, err := svc.ListDriftRows(ctx, batch.ID, 100)
	  if err != nil {
		   t.Fatalf("list drift rows: %v", err)
	   }
	   var found bool
	  for _, r := range rows {
		  if strings.ToUpper(r.Asset) == "USDT" {
			  if !r.BlockWriteHint {
				  t.Error("want block_write_hint=true for USDT critical drift")
			   }
			   if r.Severity != ledseal.SeverityCritical {
				  t.Errorf("want severity=critical, got %s", r.Severity)
			   }
			  found = true
		  }
	   }
	  if !found {
		  t.Error("drift row for USDT not found")
	  }
	   // 验根：封印根仍有效
	  if err := svc.VerifyRoot(ctx, root.ID); err != nil {
		  t.Errorf("verify root: %v", err)
	   }
	   // block_write_hint 只是建议；不得直接调用 cover/Apply。
	   // 这里仅验证 hint 字段，不调用任何账本写接口。
	  t.Logf("Fixture2 OK: drift_count=%d max_severity=%s block_hints=%d",
		   batch.DriftCount, batch.MaxSeverity, countHints(rows))
  }

  func countHints(rows []ledseal.DriftRow) int {
	  n := 0
	   for _, r := range rows {
		   if r.BlockWriteHint {
			   n++
		   }
	  }
	   return n
  }

// ─────────────────────────────────────────────────────────
// Fixture 3：Seq 回滚/间隙 → 批次 failed + GapIncident；不得标记 sealed
// ─────────────────────────────────────────────────────────

   func TestLedSeal_SeqGap_BatchFails_NotSealed(t *testing.T) {
	   ctx := context.Background()
	   uid := "user-fixture-3"
	  t0 := time.Date(2022,
		   6, 14, 12, 0, 0, 0, time.UTC)
	   t1 := t0.Add(1 * time.Hour)
	  t2 := t1.Add(1 * time.Hour)
	   positions := []snap.Position{
		  makePos("MAIN", "USDT", "100.00", "0"),
	  }
	  st := snap.NewMemStore()
	  snp0 := makeSnap(st, uid, positions, t0)
	  snp1 := makeSnap(st, uid, positions, t1)
	  snp2 := makeSnap(st,
		  uid, positions, t2)
	   svc := buildSealEnv2(st)
	  svc.WithClock(func() time.Time { return t2 })
	  plan, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{
		   Name: "test-seq-gap", PlanScope: ledseal.ScopeUserShard,
		  ShardKey: uid, IntervalSeconds: 3600, CreatedBy: "test",
	   })
	   // 注册 3 个切片（正常顺序 seq=1,2,3）。
	   sl0, _ := svc.RegisterSlice(ctx, plan.ID, snp0.ID, uid, "", snp0.Checksum, snp0.Count, 0, "", t0)
	   sl1, _ := svc.RegisterSlice(ctx, plan.ID, snp1.ID, uid, "", snp1.Checksum, snp1.Count, 0, "", t1)
	  sl2, _ := svc.RegisterSlice(ctx, plan.ID, snp2.ID, uid, "", snp2.Checksum, snp2.Count, 0, "", t2)
	  // ── 子 A：序号间隙（手动构造 from=sl0, to=sl2，跳过了 sl1）──
	   t.Run("gap", func(t *testing.T) {
		  // 强制篡改 sl2 的 GenSeq 使其 != sl0.GenSeq+1（ gap）
		  // 这里通过直接 OpenBatch 并用不连续的 GenSeq 来触发间隙检测。
		  batch, err := svc.OpenBatch(ctx, plan.ID, sl0.ID, sl2.ID,
			   sl0.GenSeq, sl0.GenSeq+2) // 跳过了 seq+1
		   if err != nil {
			  t.Fatalf("open batch: %v", err)
		   }
		  _, _, err = svc.RunAndSealBatch(ctx, batch.ID)
		  if err == nil {
			   t.Fatal("want error for seq gap, got nil")
		   }
		  // 批次必须标记 failed，不得 sealed
		  b,
			   getErr := svc.GetBatch(ctx, batch.ID)
		   if getErr != nil {
			   t.Fatalf("get batch: %v", getErr)
		  }
		   if b.State != ledseal.BatchFailed {
			  t.Errorf("want state=failed, got %s", b.State)
		  }
		   if b.State == ledseal.BatchSealed {
			   t.Error("batch must NOT be sealed after seq gap")
		  }
	  })
	   // ── 子 B：序号回滚（from_seq > to_seq）──
	   t.Run("rollback", func(t *testing.T) {
		   batch, err := svc.OpenBatch(ctx, plan.ID, sl2.ID, sl1.ID,
			  sl2.GenSeq, sl1.GenSeq) // to < from → rollback
		  if err != nil {
			  t.Fatalf("open batch: %v", err)
		  }
		  _, _, err = svc.RunAndSealBatch(ctx, batch.ID)
		  if err == nil {
			  t.Fatal("want error for seq rollback, got nil")
		   }
		   b, _ := svc.GetBatch(ctx, batch.ID)
		  if b.State != ledseal.BatchFailed {
			  t.Errorf("want state=failed, got %s", b.State)
		  }
	   })
	   _ = sl1
	   t.Logf("Fixture3 OK: seq gap/rollback correctly marks batch as failed")
   }

// ─────────────────────────────────────────────────────────
// Fixture 4：并行与 materialize 读模型——本包不写 materialize 投影表
// ─────────────────────────────────────────────────────────

  func TestLedSeal_ParallelNonMaterialize(t *testing.T) {
	   ctx := context.Background()
	  uid := "user-fixture-4"
	  t0 := time.Date(2022, 6, 14, 13, 0, 0, 0, time.UTC)
	   t1 := t0.Add(1 * time.Hour)
	  positions := []snap.Position{
		   makePos("MAIN", "ETH", "10.0", "0"),
		   makePos("TRADE", "ETH", "5.0", "2.0"),
	  }
	   st := snap.NewMemStore()
	   snp0 := makeSnap(st, uid, positions, t0)
	   snp1 := makeSnap(st, uid, positions, t1)
	  svc := buildSealEnv2(st)
	  svc.WithClock(func() time.Time { return t1 })
	  plan, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{
		   Name: "test-non-materialize", PlanScope: ledseal.ScopeUserShard,
		  ShardKey: uid, IntervalSeconds: 3600, CreatedBy: "test",
	  })
	   sl0, _ := svc.RegisterSlice(ctx, plan.ID, snp0.ID, uid, "", snp0.Checksum, snp0.Count, 0, "", t0)
	   sl1, _ := svc.RegisterSlice(ctx, plan.ID, snp1.ID, uid, "", snp1.Checksum, snp1.Count, 0, "", t1)
	  batch, _ := svc.OpenBatch(ctx, plan.ID, sl0.ID, sl1.ID, sl0.GenSeq, sl1.GenSeq)
	   batch, root, err := svc.RunAndSealBatch(ctx, batch.ID)
	  if err != nil {
		  t.Fatalf("run and seal: %v", err)
	  }
	  // 本次操作不写任何 materialize 投影表。
	  // 仅验证 ledseal 自有数据（DiffBatch、SealRoot、DriftRow）。
	  if batch.State != ledseal.BatchSealed {
		   t.Errorf("want sealed, got %s", batch.State)
	  }
	  if root.ID == "" {
		  t.Error("want non-empty root ID")
	  }
	  // 只读核对：查询漂移行（无漂移）。
	  rows, err := svc.ListDriftRows(ctx, batch.ID, 100)
	  if err != nil {
		  t.Fatalf("list drift rows: %v", err)
	   }
	  if len(rows) != 0 {
		  t.Errorf("want 0 drift rows, got %d", len(rows))
	   }
	   // 验证：snap store 内容未被 ledseal 改写（只读消费）。
	  // 从快照读回并比较原始 checksum。
	   restored0, err := st.Get(ctx, snp0.ID)
	  if err != nil {
		  t.Fatalf("get snap0: %v", err)
	   }
	  if restored0.Checksum != snp0.Checksum {
		  t.Error("ledseal must not mutate snap checksum (read-only)")
	  }
	  // 验证封印根可选读：外部对账台按 batch_id 导出 root。
	   exportedRoot, err := svc.ExportRoot(ctx, ledseal.ExportRootReq{
		   BatchID: batch.ID, ExportedBy: "recon-desk",
	   })
	  if err != nil {
		  t.Fatalf("export root: %v", err)
	  }
	  if exportedRoot.RootHash != root.RootHash {
		   t.Error("exported root hash mismatch")
	   }
	   t.Logf("Fixture4 OK: no materialize writes; root exported for recon desk batch=%s", batch.ID)
  }

// ─────────────────────────────────────────────────────────
// Fixture 5：大型夹具——增量路径结果等于全量路径样本
// ─────────────────────────────────────────────────────────

  func TestLedSeal_LargeFixture_IncrementalMatchesFull(t *testing.T) {
	  ctx := context.Background()
	   uid := "user-fixture-5"
	   t0 := time.Date(2022, 6, 14, 14, 0, 0, 0, time.UTC)
	  t1 := t0.Add(1 * time.Hour)
	  // 生成 200 个唯一位置（每个 book|asset 键唯一），其中 10 个在第二代有变化。
	   // 使用 fmt.Sprintf 确保每个 book 名称唯一，避免差分引擎去重导致不一致。
	   const n = 100
	  assets := []string{"USDT", "BTC"}
	   pos0 := make([]snap.Position, 0, n*len(assets))
	   pos1 := make([]snap.Position, 0, n*len(assets))
	  driftCount := 0
	   for i := 0; i < n; i++ {
		   book := fmt.Sprintf("BOOK%03d", i) // 唯一 book 名称
		   for _, asset := range assets {
			   avail := "100.00"
			  avail1 := "100.00"
			  // 每 10 个 USDT 位置注入一个小漂移（minor 级别）。
			   if asset == "USDT" && i%10 == 0 {
				   avail1 = "100.01" // delta = 0.01，相对率 0.01% → minor
				   driftCount++
			   }
			   pos0 = append(pos0, makePos(book, asset, avail, "0"))
			   pos1 = append(pos1, makePos(book, asset, avail1, "0"))
		  }
	  }
	   st := snap.NewMemStore()
	  snp0 := makeSnap(st, uid, pos0, t0)
	   snp1 := makeSnap(st, uid, pos1, t1)
	   svc := buildSealEnv2(st)
	  svc.WithClock(func() time.Time { return t1 })
	  plan, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{
		  Name: "test-large-fixture", PlanScope: ledseal.ScopeUserShard,
		  ShardKey: uid, IntervalSeconds: 3600, CreatedBy: "test",
	   })
	   sl0, _ := svc.RegisterSlice(ctx, plan.ID, snp0.ID, uid, "", snp0.Checksum, snp0.Count, 0, "", t0)
	  sl1, _ := svc.RegisterSlice(ctx, plan.ID, snp1.ID, uid, "", snp1.Checksum, snp1.Count, 0, "", t1)
	  // ── 增量路径（via RunAndSealBatch）──
	   batchInc, _ := svc.OpenBatch(ctx, plan.ID, sl0.ID, sl1.ID, sl0.GenSeq, sl1.GenSeq)
	  batchInc, rootInc, err := svc.RunAndSealBatch(ctx, batchInc.ID)
	   if err != nil {
		   t.Fatalf("incremental batch: %v", err)
	   }
	   // ── 全量路径（直接 Compare 两个快照，走 snap 包索引去重路径）──
	   snapSvcFull := snap.NewWithStore(nil, nil, st)
	   diffFull, err := snapSvcFull.Diff(ctx, snp0.ID, snp1.ID)
	  if err != nil {
		  t.Fatalf("full diff: %v", err)
	   }
	  // snap.Compare 基于索引（每 book|asset 唯一），incremental 遍历全量 positions。
	  // 唯一 key 时两者结果一致；此处都走唯一 key 路径，所以 Changed 数应等于 driftCount。
	   fullChangedCount := len(diffFull.Changed) + len(diffFull.Added) + len(diffFull.Removed)
	  // 断言：全量路径与预期漂移数一致。
	   if fullChangedCount != driftCount {
		  t.Errorf("full_diff changed=%d, want %d", fullChangedCount, driftCount)
	  }
	   // 断言：增量路径发现的漂移数不少于全量路径（允许多，因为 incremental 遍历全量 positions）。
	   // 在唯一 key 场景下两者应相等。
	  if batchInc.DriftCount != driftCount {
		   t.Errorf("incremental drift_count=%d, want %d", batchInc.DriftCount, driftCount)
	  }
	  // 所有漂移都是 minor（delta=0.01，总量 100 → 0.01% → minor）。
	   rows,
		  _ := svc.ListDriftRows(ctx, batchInc.ID, 500)
	   for _, r := range rows {
		  if r.Severity != ledseal.SeverityMinor {
			  t.Errorf("want minor, got %s for user=%s book=%s asset=%s", r.Severity, r.UserID, r.Book, r.Asset)
		  }
	  }
	  // 封印根完整性。
	   if err := svc.VerifyRoot(ctx, rootInc.ID); err != nil {
		   t.Errorf("verify incremental root: %v", err)
	  }
	   t.Logf("Fixture5 OK: positions=%d incremental_drift=%d full_diff_changed=%d root=%s",
		   len(pos0), batchInc.DriftCount, fullChangedCount, rootInc.RootHash[:16])
  }

// ─────────────────────────────────────────────────────────
// 附加：回放一致性校验
// ─────────────────────────────────────────────────────────

   func TestLedSeal_ReplayCheck_MatchesFingerpint(t *testing.T) {
	  ctx := context.Background()
	   uid := "user-replay"
	  t0 := time.Date(2022,
		   6, 14, 15, 0, 0, 0, time.UTC)
	  t1 := t0.Add(1 * time.Hour)
	  positions := []snap.Position{
		  makePos("MAIN", "USDT", "500.00", "0"),
		  makePos("TRADE", "BTC", "0.25", "0.05"),
	   }
	   st := snap.NewMemStore()
	  snp0 := makeSnap(st,
		  uid, positions, t0)
	   snp1 := makeSnap(st, uid, positions, t1)
	   svc := buildSealEnv2(st)
	  svc.WithClock(func() time.Time { return t1 })
	   plan, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{
		  Name: "test-replay", PlanScope: ledseal.ScopeUserShard,
		   ShardKey: uid, IntervalSeconds: 3600, CreatedBy: "test",
	  })
	  sl0, _ := svc.RegisterSlice(ctx, plan.ID, snp0.ID, uid, "", snp0.Checksum, snp0.Count, 0, "", t0)
	  sl1, _ := svc.RegisterSlice(ctx, plan.ID, snp1.ID, uid, "", snp1.Checksum, snp1.Count, 0, "", t1)
	   batch, _ := svc.OpenBatch(ctx, plan.ID, sl0.ID, sl1.ID, sl0.GenSeq, sl1.GenSeq)
	  _, _, err := svc.RunAndSealBatch(ctx, batch.ID)
	   if err != nil {
		  t.Fatalf("run and seal: %v", err)
	   }
	   // 回放校验器。
	  rc := ledseal.NewReplayChecker(svc)
	   // 校验 from 切片。
	   res0, err := rc.CheckSlice(ctx, sl0.ID)
	   if err != nil {
		   t.Errorf("replay check slice 0: %v", err)
	  }
	  if !res0.Match {
		  t.Errorf("replay check slice 0: fingerprint mismatch from=%s snap=%s",
			   res0.FingerprintFrom, res0.FingerprintSnap)
	  }
	   // 校验 to 切片。
	   res1, err := rc.CheckSlice(ctx, sl1.ID)
	   if err != nil {
		  t.Errorf("replay check slice 1: %v", err)
	   }
	   if !res1.Match {
		   t.Errorf("replay check slice 1: fingerprint mismatch")
	  }
	   // 批次级别回放校验。
	  results, err := rc.CheckBatch(ctx, batch.ID)
	  if err != nil {
		   t.Fatalf("replay check batch: %v", err)
	  }
	   if len(results) != 2 {
		  t.Errorf("want 2 replay check results, got %d", len(results))
	  }
	   for _, r := range results {
		  if !r.Match {
			   t.Errorf("replay mismatch for slice=%s", r.SliceID)
		  }
	  }
	  t.Logf("ReplayCheck OK: both slices fingerprint match")
   }

// ─────────────────────────────────────────────────────────
// 附加：FSM 状态机
// ─────────────────────────────────────────────────────────

   func TestLedSeal_PlanFSM_ActivatePause(t *testing.T) {
	   ctx := context.Background()
	  st := snap.NewMemStore()
	   svc := buildSealEnv2(st)
	   plan, err := svc.CreatePlan(ctx, ledseal.CreatePlanReq{
		  Name: "test-fsm", PlanScope: ledseal.ScopeFullDay,
		  IntervalSeconds: 3600, CreatedBy: "fsm-test",
	   })
	  if err != nil {
		   t.Fatalf("create plan: %v", err)
	  }
	   if plan.State != ledseal.PlanActive {
		  t.Errorf("new plan should be active, got %s", plan.State)
	   }
	   // active → pause
	   plan, err = svc.PausePlan(ctx, plan.ID)
	  if err != nil {
		  t.Fatalf("pause: %v", err)
	  }
	   if plan.State != ledseal.PlanPaused {
		   t.Errorf("want paused, got %s", plan.State)
	   }
	  // 重复 pause 应失败。
	  _, err = svc.PausePlan(ctx, plan.ID)
	  if err == nil {
		  t.Error("want error on double-pause")
	  }
	   // paused → active
	  plan, err = svc.ActivatePlan(ctx, plan.ID)
	  if err != nil {
		   t.Fatalf("activate: %v", err)
	   }
	   if plan.State != ledseal.PlanActive {
		   t.Errorf("want active, got %s", plan.State)
	   }
	   // 重复 activate 应失败。
	  _, err = svc.ActivatePlan(ctx, plan.ID)
	  if err == nil {
		   t.Error("want error on activate when already active")
	   }
	   t.Logf("FSM OK: plan fsm active↔paused transitions verified")
   }

// ─────────────────────────────────────────────────────────
// 附加：批次 FSM — sealed 批次不得再次封印
// ─────────────────────────────────────────────────────────

  func TestLedSeal_BatchFSM_AlreadySealed(t *testing.T) {
	   ctx := context.Background()
	   uid := "user-batch-fsm"
	  t0 := time.Date(2022, 6, 14, 16, 0, 0, 0, time.UTC)
	  t1 := t0.Add(1 * time.Hour)
	  positions := []snap.Position{makePos("MAIN", "USDT", "1.00", "0")}
	  st := snap.NewMemStore()
	  snp0 := makeSnap(st, uid, positions, t0)
	   snp1 := makeSnap(st, uid, positions, t1)
	  svc := buildSealEnv2(st)
	  svc.WithClock(func() time.Time { return t1 })
	  plan, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{
		   Name: "test-batch-fsm", PlanScope: ledseal.ScopeUserShard,
		  ShardKey: uid, IntervalSeconds: 3600, CreatedBy: "test",
	   })
	   sl0, _ := svc.RegisterSlice(ctx, plan.ID, snp0.ID, uid, "", snp0.Checksum, snp0.Count, 0, "", t0)
	  sl1, _ := svc.RegisterSlice(ctx, plan.ID, snp1.ID, uid, "", snp1.Checksum, snp1.Count, 0, "", t1)
	   batch, _ := svc.OpenBatch(ctx, plan.ID, sl0.ID, sl1.ID, sl0.GenSeq, sl1.GenSeq)
	  _, _, err := svc.RunAndSealBatch(ctx, batch.ID)
	  if err != nil {
		  t.Fatalf("first seal: %v", err)
	  }
	  // 再次尝试封印已 sealed 的批次，应失败。
	   _, _, err = svc.RunAndSealBatch(ctx, batch.ID)
	   if err == nil {
		  t.Error("want error on second seal attempt")
	  }
	  t.Logf("BatchFSM OK: sealed batch correctly rejects re-seal")
   }

// ─────────────────────────────────────────────────────────
// 附加：漂移行人工确认
// ─────────────────────────────────────────────────────────

  func TestLedSeal_ConfirmDrift(t *testing.T) {
	  ctx := context.Background()
	  uid := "user-confirm"
	   t0 := time.Date(2022, 6, 14, 17, 0, 0, 0, time.UTC)
	  t1 := t0.Add(1 * time.Hour)
	  pos0 := []snap.Position{makePos("MAIN", "USDT", "100.00", "0")}
	  pos1 := []snap.Position{makePos("MAIN", "USDT", "50000.00", "0")} // 注入漂移
	   st := snap.NewMemStore()
	  snp0 := makeSnap(st,
		   uid, pos0, t0)
	   snp1 := makeSnap(st, uid, pos1, t1)
	  svc := buildSealEnv2(st)
	  svc.WithClock(func() time.Time { return t1 })
	   plan, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{
		  Name: "test-confirm-drift", PlanScope: ledseal.ScopeUserShard,
		   ShardKey: uid, IntervalSeconds: 3600, CreatedBy: "test",
	   })
	   sl0, _ := svc.RegisterSlice(ctx, plan.ID, snp0.ID, uid, "", snp0.Checksum, snp0.Count, 0, "", t0)
	  sl1, _ := svc.RegisterSlice(ctx, plan.ID, snp1.ID, uid, "", snp1.Checksum, snp1.Count, 0, "", t1)
	   batch, _ := svc.OpenBatch(ctx, plan.ID, sl0.ID, sl1.ID, sl0.GenSeq, sl1.GenSeq)
	   batch, _, _ = svc.RunAndSealBatch(ctx, batch.ID)
	   rows, _ := svc.ListDriftRows(ctx, batch.ID, 10)
	   if len(rows) == 0 {
		   t.Fatal("want at least 1 drift row")
	  }
	   driftID := rows[0].ID
	  // 运营台确认漂移。
	  err := svc.ConfirmDrift(ctx, ledseal.ConfirmDriftReq{
		   DriftID: driftID, Actor: "ops-alice", Note: "已核实，系统迁移引起",
	   })
	   if err != nil {
		   t.Fatalf("confirm drift: %v", err)
	   }
	  // 验证确认信息已写入。
	  confirmed, err := svc.Store().GetDriftRow(ctx, driftID)
	   if err != nil {
		  t.Fatalf("get drift row: %v", err)
	   }
	   if confirmed.ConfirmedBy != "ops-alice" {
		  t.Errorf("want ConfirmedBy=ops-alice, got %s", confirmed.ConfirmedBy)
	  }
	  if confirmed.ConfirmedAt == nil {
		  t.Error("want non-nil ConfirmedAt")
	   }
	   t.Logf("ConfirmDrift OK: drift confirmed by ops-alice")
  }

// ─────────────────────────────────────────────────────────
// 附加：persist.Fence 在 SnapSlice 中的用法验证
// ─────────────────────────────────────────────────────────

  func TestLedSeal_FenceInSlice(t *testing.T) {
	  ctx := context.Background()
	   uid := "user-fence"
	   t0 := time.Date(2022, 6, 14, 18, 0, 0, 0, time.UTC)
	  positions := []snap.Position{makePos("MAIN", "USDT", "100.00", "0")}
	   st := snap.NewMemStore()
	  snp0 := makeSnap(st, uid, positions, t0)
	   svc := buildSealEnv2(st)
	   plan, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{
		  Name: "test-fence-slice", PlanScope: ledseal.ScopeUserShard,
		   ShardKey: uid, IntervalSeconds: 3600, CreatedBy: "test",
	  })
	   // 登记带围栏信息的切片。
	   fence := persist.Fence{Epoch: 42, Token: "test-token-abc123"}
	   sl, err := svc.RegisterSlice(ctx, plan.ID, snp0.ID, uid, "", snp0.Checksum, snp0.Count,
		   fence.Epoch, fence.Token, t0)
	   if err != nil {
		  t.Fatalf("register slice with fence: %v", err)
	  }
	   if sl.FenceEpoch != 42 {
		   t.Errorf("want FenceEpoch=42, got %d", sl.FenceEpoch)
	   }
	   if sl.FenceToken != "test-token-abc123" {
		  t.Errorf("want FenceToken=test-token-abc123, got %s", sl.FenceToken)
	   }
	   t.Logf("FenceSlice OK: slice fence epoch=%d token=%s", sl.FenceEpoch, sl.FenceToken[:8])
  }

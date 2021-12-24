package tracesnapgate_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/investigate"
	"account-assets.kucoin.net/funding/internal/tracesnapgate"
)

// ─── Provider 桩 ──────────────────────────────────────────────────────
// stubProvider 用静态 TraceProvider。

type stubProvider struct {
	mu    sync.Mutex
	hops  []investigate.Hop
	books []tracesnapgate.SnapPosition
}

  func (p *stubProvider) TraceHops(_ context.Context, _, _ string) ([]investigate.Hop, error) {
	   p.mu.Lock()
	  defer p.mu.Unlock()
	  cp := make([]investigate.Hop, len(p.hops))
	  copy(cp, p.hops)
	  return cp, nil
  }

  func (p *stubProvider) TraceBooks(_ context.Context, _, _ string) ([]tracesnapgate.SnapPosition, error) {
	   p.mu.Lock()
	   defer p.mu.Unlock()
	  cp := make([]tracesnapgate.SnapPosition, len(p.books))
	   copy(cp, p.books)
	  return cp, nil
  }

  func (p *stubProvider) setHops(hops []investigate.Hop) {
	   p.mu.Lock()
	  defer p.mu.Unlock()
	   p.hops = hops
  }

  func (p *stubProvider) setBooks(books []tracesnapgate.SnapPosition) {
	  p.mu.Lock()
	   defer p.mu.Unlock()
	  p.books = books
   }

// ─── 构建辅助 ─────────────────────────────────────────────────────────────

   func newTestEngine(prov tracesnapgate.TraceProvider, cfg tracesnapgate.Config) *tracesnapgate.Engine {
	  store := tracesnapgate.NewMemStore()
	  return tracesnapgate.NewEngine(store, prov, cfg)
  }

   func defaultCfg() tracesnapgate.Config {
	  return tracesnapgate.Config{
		  SealTTL:         10 * time.Minute,
		   ReplayBatchSize: 50,
	  }
   }

  func sampleHops() []investigate.Hop {
	  return []investigate.Hop{
		   {ID: "tr-001", FromBook: "funding", ToBook: "trading", Asset: "BTC", Amount: "0.1"},
		  {ID: "tr-002", FromBook: "trading", ToBook: "margin", Asset: "BTC", Amount: "0.05"},
	  }
  }

  func sampleBooks() []tracesnapgate.SnapPosition {
	   return []tracesnapgate.SnapPosition{
		  {Book: "funding", Subject: "available", Asset: "BTC", Available: "0.85", Frozen: "0"},
		  {Book: "trading", Subject: "available", Asset: "BTC", Available: "0.05", Frozen: "0"},
		  {Book: "margin", Subject: "available", Asset: "BTC", Available: "0.05", Frozen: "0"},
	  }
  }

// ─── CreateSnapshot ───────────────────────────────────────────────────

  func TestCreateSnapshot(t *testing.T) {
	   ctx := context.Background()
	   prov := &stubProvider{hops: sampleHops(), books: sampleBooks()}
	  eng := newTestEngine(prov, defaultCfg())
	   tests := []struct {
		   name string
		   req     tracesnapgate.CreateRequest
		   wantErr error
	   }{
		   {
			  name: "正常创建",
			   req: tracesnapgate.CreateRequest{UserID: "u1", Asset: "BTC", Actor: "ops-1"},
			  wantErr: nil,
		   },
		  {
			   name:    "缺 UserID",
			  req:     tracesnapgate.CreateRequest{UserID: "", Asset: "BTC", Actor: "ops-1"},
			   wantErr: tracesnapgate.ErrInvalidUserID,
		   },
		   {
			  name:    "缺 Asset",
			  req:     tracesnapgate.CreateRequest{UserID: "u1", Asset: "", Actor: "ops-1"},
			  wantErr: tracesnapgate.ErrInvalidAsset,
		   },
		  {
			   name:    "资产大小写归一化",
			   req:     tracesnapgate.CreateRequest{UserID: "u1", Asset: "usdt", Actor: "ops-1"},
			  wantErr: nil,
		   },
	  }
	  for _, tt := range tests {
		  t.Run(tt.name, func(t *testing.T) {
			   s, err := eng.CreateSnapshot(ctx, tt.req)
			   if tt.wantErr != nil {
				   if !errors.Is(err, tt.wantErr) {
					  t.Fatalf("期望错误 %v，实际 %v", tt.wantErr, err)
				  }
				   return
			  }
			  if err != nil {
				  t.Fatalf("意外错误: %v", err)
			  }
			   if s.ID == "" {
				  t.Error("快照 ID 不应为空")
			  }
			   if s.SnapshotGen == "" {
				  t.Error("snapshot_gen 不应为空")
			   }
			  if s.Status != tracesnapgate.StatusOpen {
				  t.Errorf("期望 open 状态，实际 %s", s.Status)
			  }
			  if strings.ToUpper(tt.req.Asset) != s.Asset {
				  t.Errorf("资产未大写归一化: %s", s.Asset)
			  }
		  })
	   }
   }

// TestCreateSnapshotIdemKey 幂等键防重。
   func TestCreateSnapshotIdemKey(t *testing.T) {
	   ctx := context.Background()
	  prov := &stubProvider{hops: sampleHops(), books: sampleBooks()}
	  eng := newTestEngine(prov, defaultCfg())
	   req := tracesnapgate.CreateRequest{UserID: "u2", Asset: "ETH", Actor: "ops-2", IdemKey: "idem-eth-u2"}
	   s1,
		   err := eng.CreateSnapshot(ctx, req)
	  if err != nil {
		   t.Fatalf("首次创建失败: %v", err)
	  }
	   s2, err := eng.CreateSnapshot(ctx, req)
	  if err != nil {
		  t.Fatalf("幂等重建失败: %v", err)
	   }
	   if s1.ID != s2.ID {
		  t.Errorf("幂等键应返回相同 ID: %s != %s", s1.ID, s2.ID)
	  }
  }

// ─── Seal ─────────────────────────────────────────────────────────────

  func TestSeal(t *testing.T) {
	   ctx := context.Background()
	   prov := &stubProvider{hops: sampleHops(),
		   books: sampleBooks()}
	   eng := newTestEngine(prov, defaultCfg())
	   // 创建快照。
	   snap, err := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{UserID: "u3", Asset: "BTC", Actor: "ops-3"})
	  if err != nil {
		  t.Fatalf("创建快照失败: %v", err)
	   }
	  // 正常密封。
	  sealed, err := eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: snap.ID, Actor: "ops-3"})
	  if err != nil {
		  t.Fatalf("密封失败: %v", err)
	  }
	  if sealed.Status != tracesnapgate.StatusSealed {
		  t.Errorf("期望 sealed 状态，实际 %s", sealed.Status)
	  }
	   if sealed.TransfersRoot == "" {
		  t.Error("TransfersRoot 不应为空")
	  }
	  if sealed.BooksRoot == "" {
		   t.Error("BooksRoot 不应为空")
	  }
	  if sealed.AsOf.IsZero() {
		  t.Error("AsOf 不应为零值")
	  }
	   // 重复密封：ErrSealedAlready。
	   _, err = eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: snap.ID, Actor: "ops-3"})
	  if !errors.Is(err, tracesnapgate.ErrSealedAlready) {
		  t.Errorf("重复密封期望 ErrSealedAlready，实际 %v", err)
	   }
  }

// TestSealTTLExpired 超 TTL 后 Seal 被拒绝。

  func TestSealTTLExpired(t *testing.T) {
	  ctx := context.Background()
	   prov := &stubProvider{hops: sampleHops(), books: sampleBooks()}
	   cfg := tracesnapgate.Config{SealTTL: 1 * time.Second}
	  store := tracesnapgate.NewMemStore()
	   eng := tracesnapgate.NewEngine(store, prov, cfg)
	  // 时钟注入：创建时刻 T0。
	  t0 := time.Now().UTC()
	   eng.WithClock(func() time.Time { return t0 })
	  snap, err := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{UserID: "u4", Asset: "BTC", Actor: "ops"})
	  if err != nil {
		   t.Fatalf("创建快照失败: %v", err)
	   }
	  // 推进时钟超 TTL。
	  eng.WithClock(func() time.Time { return t0.Add(2 * time.Second) })
	  _, err = eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: snap.ID, Actor: "ops"})
	  if !errors.Is(err, tracesnapgate.ErrNotOpen) {
		   t.Errorf("超 TTL 期望 ErrNotOpen，实际 %v", err)
	   }
  }

// TestSealSupersedesOld 新快照密封后旧快照被取代。

  func TestSealSupersedesOld(t *testing.T) {
	   ctx := context.Background()
	   prov := &stubProvider{hops: sampleHops(), books: sampleBooks()}
	   eng := newTestEngine(prov, defaultCfg())
	  // 创建并密封旧快照 S1。
	   s1, _ := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{UserID: "u5", Asset: "ETH", Actor: "ops"})
	   _, err := eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: s1.ID, Actor: "ops"})
	   if err != nil {
		  t.Fatalf("密封 S1 失败: %v", err)
	   }
	  // 创建并密封新快照 S2（同用户/同资产）。
	  s2, _ := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{UserID: "u5", Asset: "ETH", Actor: "ops"})
	  _, err = eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: s2.ID, Actor: "ops"})
	   if err != nil {
		  t.Fatalf("密封 S2 失败: %v", err)
	  }
	   // S1 应已变为 superseded。
	   updated, err := eng.GetSnapshot(ctx, s1.ID)
	  if err != nil {
		   t.Fatalf("读取 S1 失败: %v", err)
	   }
	  if updated.Status != tracesnapgate.StatusSuperseded {
		  t.Errorf("S1 期望 superseded，实际 %s", updated.Status)
	   }
	   if updated.SupersededBy != s2.ID {
		  t.Errorf("SupersededBy 期望 %s，实际 %s", s2.ID, updated.SupersededBy)
	  }
   }

// ─── Replay ──────────────────────────────────────────────────────────
// TestReplayConsistent 活账与密封一致时重放返回一致。
   func TestReplayConsistent(t *testing.T) {
	   ctx := context.Background()
	   prov := &stubProvider{hops: sampleHops(), books: sampleBooks()}
	  eng := newTestEngine(prov, defaultCfg())
	   snap, _ := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{UserID: "u6", Asset: "BTC", Actor: "ops"})
	   sealed, _ := eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: snap.ID, Actor: "ops"})
	  // 活账未变；多次 Replay 应一致。
	  for i := 0; i < 3; i++ {
		  res, err := eng.Replay(ctx, sealed.ID)
		  if err != nil {
			  t.Fatalf("第 %d 次 Replay 意外失败: %v", i+1, err)
		   }
		   if !res.Consistent {
			  t.Errorf("第 %d 次 Replay 期望一致", i+1)
		   }
		   if res.TransfersRoot != sealed.TransfersRoot {
			  t.Errorf("第 %d 次 TransfersRoot 不一致: %s != %s", i+1, res.TransfersRoot, sealed.TransfersRoot)
		  }
		  if res.BooksRoot != sealed.BooksRoot {
			  t.Errorf("第 %d 次 BooksRoot 不一致: %s != %s", i+1, res.BooksRoot, sealed.BooksRoot)
		   }
	  }
  }

// TestReplayDrift 密封后活账变化时 Replay 返回漂移错误和证明。

   func TestReplayDrift(t *testing.T) {
	   ctx := context.Background()
	   prov := &stubProvider{hops: sampleHops(), books: sampleBooks()}
	  eng := newTestEngine(prov, defaultCfg())
	   snap, _ := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{UserID: "u7", Asset: "BTC", Actor: "ops"})
	  sealed, _ := eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: snap.ID, Actor: "ops"})
	  // 密封后活账增加新 hop（活账漂移）。
	   newHops := append(sampleHops(), investigate.Hop{
		  ID: "tr-999", FromBook: "margin", ToBook: "futures", Asset: "BTC", Amount: "0.01",
	   })
	   prov.setHops(newHops)
	   _, err := eng.Replay(ctx, sealed.ID)
	  if err == nil {
		  t.Fatal("期望漂移错误，实际 nil")
	  }
	   de, isDrift := tracesnapgate.IsDriftError(err)
	   if !isDrift {
		   t.Fatalf("期望 DriftError，实际 %T: %v", err, err)
	   }
	   if de.ProofID == "" {
		   t.Error("漂移证明 ID 不应为空")
	  }
	   // 验证证明已落库。
	   proof, perr := eng.GetProof(ctx, de.ProofID)
	  if perr != nil {
		  t.Fatalf("读取漂移证明失败: %v", perr)
	   }
	   if proof.SnapshotID != sealed.ID {
		   t.Errorf("证明 SnapshotID 不匹配: %s != %s", proof.SnapshotID, sealed.ID)
	  }
	   if proof.SealedTransfersRoot != sealed.TransfersRoot {
		  t.Errorf("证明 SealedTransfersRoot 不匹配")
	   }
   }

// TestReplayDriftIdempotent 重复 Replay 相同漂移不重复创建证明。

   func TestReplayDriftIdempotent(t *testing.T) {
	  ctx := context.Background()
	   prov := &stubProvider{hops: sampleHops(), books: sampleBooks()}
	  eng := newTestEngine(prov, defaultCfg())
	  snap, _ := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{UserID: "u8", Asset: "ETH", Actor: "ops"})
	   sealed, _ := eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: snap.ID, Actor: "ops"})
	  // 活账变化。
	  prov.setBooks([]tracesnapgate.SnapPosition{
		  {Book: "funding", Subject: "available", Asset: "ETH", Available: "1.5", Frozen: "0"},
	   })
	  var firstProofID string
	   for i := 0; i < 3; i++ {
		  _, err := eng.Replay(ctx, sealed.ID)
		  if err == nil {
			  t.Fatalf("第 %d 次期望漂移错误", i+1)
		  }
		   de, ok := tracesnapgate.IsDriftError(err)
		  if !ok {
			   t.Fatalf("第 %d 次期望 DriftError", i+1)
		  }
		   if i == 0 {
			   firstProofID = de.ProofID
		   } else {
			   // 相同 hash → 应返回已存在的证明 ID（幂等）。
			   if de.ProofID != firstProofID {
				  t.Errorf("第 %d 次漂移证明 ID 不一致: %s != %s（应幂等）", i+1, de.ProofID, firstProofID)
			  }
		   }
	   }
  }

// TestReplayNotSealed 未密封快照 Replay 被拒绝。

  func TestReplayNotSealed(t *testing.T) {
	   ctx := context.Background()
	  prov := &stubProvider{hops: sampleHops(), books: sampleBooks()}
	  eng := newTestEngine(prov, defaultCfg())
	  snap, _ := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{UserID: "u9", Asset: "BTC", Actor: "ops"})
	  _, err := eng.Replay(ctx, snap.ID)
	  if !errors.Is(err, tracesnapgate.ErrNotSealed) {
		   t.Errorf("未密封期望 ErrNotSealed，实际 %v", err)
	  }
  }

// ─── Export ──────────────────────────────────────────────────────────
// TestExportNormal 正常法医导出。

  func TestExportNormal(t *testing.T) {
	   ctx := context.Background()
	   prov := &stubProvider{hops: sampleHops(), books: sampleBooks()}
	   eng := newTestEngine(prov, defaultCfg())
	   snap, _ := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{UserID: "u10", Asset: "BTC", Actor: "ops"})
	  sealed, _ := eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: snap.ID, Actor: "ops"})
	  bundle, err := eng.Export(ctx, tracesnapgate.ExportRequest{SnapshotID: sealed.ID, Actor: "ops"})
	  if err != nil {
		  t.Fatalf("法医导出失败: %v", err)
	  }
	  if bundle.Snapshot.Status != tracesnapgate.StatusExported {
		   t.Errorf("期望 exported 状态，实际 %s", bundle.Snapshot.Status)
	  }
	  if bundle.Snapshot.SnapshotGen == "" {
		  t.Error("导出 bundle 中 snapshot_gen 不应为空")
	   }
	   if bundle.ExportedAt.IsZero() {
		   t.Error("ExportedAt 不应为零值")
	   }
  }

// TestExportFailsWithoutSnapshotGen 验收 2：缺 snapshot_gen 必须拒绝（fail-closed）。

   func TestExportFailsWithoutSnapshotGen(t *testing.T) {
	  ctx := context.Background()
	  prov := &stubProvider{hops: sampleHops(), books: sampleBooks()}
	   store := tracesnapgate.NewMemStore()
	  eng := tracesnapgate.NewEngine(store, prov, defaultCfg())
	  // 手动注入缺少 snapshot_gen 的已密封快照。
	   now := time.Now().UTC()
	   sealedAt := now
	   noGenSnap := tracesnapgate.TraceSnapshot{
		  ID:          "no-gen-snap",
		  SnapshotGen: "",
		  UserID:      "u-nogen",
		   Asset:       "ETH",
		  Status:        tracesnapgate.StatusSealed,
		  TransfersRoot: "deadbeef",
		  BooksRoot:     "cafebabe",
		   AsOf:          now,
		  SealedAt:      &sealedAt,
		  CreatedAt: now,
	  }
	   if err := store.PutSnapshot(ctx, noGenSnap); err != nil {
		  t.Fatalf("注入快照失败: %v", err)
	  }
	   _, err := eng.Export(ctx, tracesnapgate.ExportRequest{SnapshotID: "no-gen-snap", Actor: "ops"})
	  if !errors.Is(err, tracesnapgate.ErrNoSnapshot) {
		  t.Errorf("缺 snapshot_gen 期望 ErrNoSnapshot，实际 %v", err)
	  }
  }

// TestExportSuperseded superseded 快照导出被拒绝。
  func TestExportSuperseded(t *testing.T) {
	  ctx := context.Background()
	   prov := &stubProvider{hops: sampleHops(), books: sampleBooks()}
	   eng := newTestEngine(prov, defaultCfg())
	  s1, _ := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{UserID: "u11", Asset: "BTC", Actor: "ops"})
	   eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: s1.ID, Actor: "ops"}) // nolint
	   s2, _ := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{UserID: "u11", Asset: "BTC", Actor: "ops"})
	  eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: s2.ID, Actor: "ops"}) // nolint
	   // S1 应已被取代，导出应失败。
	  _, err := eng.Export(ctx, tracesnapgate.ExportRequest{SnapshotID: s1.ID, Actor: "ops"})
	   if !errors.Is(err, tracesnapgate.ErrSuperseded) {
		  t.Errorf("superseded 快照导出期望 ErrSuperseded，实际 %v", err)
	   }
   }

// ─── BindCase ────────────────────────────────────────────────────────
// TestBindCase 案件绑定正常路径。

  func TestBindCase(t *testing.T) {
	  ctx := context.Background()
	   prov := &stubProvider{hops: sampleHops(),
		  books: sampleBooks()}
	   eng := newTestEngine(prov, defaultCfg())
	   snap, _ := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{UserID: "u12", Asset: "BTC", Actor: "ops"})
	  sealed, _ := eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: snap.ID, Actor: "ops"})
	   bind, err := eng.BindCase(ctx, tracesnapgate.BindCaseRequest{
		   CaseID: "case-001", SnapshotID: sealed.ID, BoundBy: "ops",
	  })
	   if err != nil {
		  t.Fatalf("绑定案件失败: %v", err)
	  }
	  if bind.CaseID != "case-001" {
		  t.Errorf("CaseID 不匹配: %s", bind.CaseID)
	   }
	  if bind.SnapshotGen != sealed.SnapshotGen {
		  t.Errorf("SnapshotGen 不匹配: %s != %s", bind.SnapshotGen, sealed.SnapshotGen)
	   }
  }

// TestBindCaseDuplicate 重复绑定被拒绝。

  func TestBindCaseDuplicate(t *testing.T) {
	   ctx := context.Background()
	  prov := &stubProvider{hops: sampleHops(), books: sampleBooks()}
	   eng := newTestEngine(prov, defaultCfg())
	  snap, _ := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{UserID: "u13", Asset: "ETH", Actor: "ops"})
	  sealed, _ := eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: snap.ID, Actor: "ops"})
	   // 首次绑定。
	   _, err := eng.BindCase(ctx, tracesnapgate.BindCaseRequest{
		   CaseID: "case-002", SnapshotID: sealed.ID, BoundBy: "ops",
	   })
	   if err != nil {
		   t.Fatalf("首次绑定失败: %v", err)
	   }
	   // 同案 ID 绑不同快照应拒绝。
	   snap2, _ := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{UserID: "u13", Asset: "ETH", Actor: "ops"})
	  sealed2, _ := eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: snap2.ID, Actor: "ops"})
	   _,
		  err = eng.BindCase(ctx, tracesnapgate.BindCaseRequest{
		   CaseID: "case-002", SnapshotID: sealed2.ID, BoundBy: "ops",
	  })
	  if !errors.Is(err, tracesnapgate.ErrCaseBound) {
		   t.Errorf("重复绑定不同快照期望 ErrCaseBound，实际 %v", err)
	  }
  }

// TestBindCaseIdempotent 相同快照重复绑定幂等。

   func TestBindCaseIdempotent(t *testing.T) {
	  ctx := context.Background()
	   prov := &stubProvider{hops: sampleHops(), books: sampleBooks()}
	  eng := newTestEngine(prov, defaultCfg())
	  snap, _ := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{UserID: "u14", Asset: "BTC", Actor: "ops"})
	  sealed, _ := eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: snap.ID, Actor: "ops"})
	  b1, _ := eng.BindCase(ctx, tracesnapgate.BindCaseRequest{
		  CaseID: "case-003", SnapshotID: sealed.ID, BoundBy: "ops",
	  })
	  b2, err := eng.BindCase(ctx, tracesnapgate.BindCaseRequest{
		   CaseID: "case-003", SnapshotID: sealed.ID, BoundBy: "ops",
	   })
	   if err != nil {
		  t.Fatalf("幂等绑定失败: %v", err)
	   }
	   if b1.ID != b2.ID {
		  t.Errorf("幂等绑定应返回相同 ID: %s != %s", b1.ID, b2.ID)
	  }
   }

// ─── 并发安全 ─────────────────────────────────────────────────────────
// TestConcurrentCreateSnapshot 并发创建快照不死锁、不竞争。
  func TestConcurrentCreateSnapshot(t *testing.T) {
	   ctx := context.Background()
	  prov := &stubProvider{hops: sampleHops(), books: sampleBooks()}
	  eng := newTestEngine(prov, defaultCfg())
	  const goroutines = 20
	   var wg sync.WaitGroup
	   errs := make(chan error, goroutines)
	   for i := 0; i < goroutines; i++ {
		  wg.Add(1)
		   go func(i int) {
			  defer wg.Done()
			   _, err := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{
				  UserID: "concurrent-user",
				  Asset: "BTC",
				   Actor: "ops",
			   })
			  if err != nil {
				   errs <- err
			  }
		  }(i)
	   }
	  wg.Wait()
	  close(errs)
	   for err := range errs {
		  t.Errorf("并发创建出现错误: %v", err)
	  }
   }

// ─── 摘要根不变量 ─────────────────────────────────────────────────────
// TestTransfersRootOrderIndependent 摘要根不受 hop 输入顺序影响。

  func TestTransfersRootOrderIndependent(t *testing.T) {
	   hops1 := []tracesnapgate.HopRecord{
		   {TransferID: "t1", FromBook: "f", ToBook: "g", Asset: "BTC", AmountHex: "aabb"},
		   {TransferID: "t2", FromBook: "g", ToBook: "h", Asset: "BTC", AmountHex: "ccdd"},
	   }
	   hops2 := []tracesnapgate.HopRecord{
		  {TransferID: "t2", FromBook: "g", ToBook: "h", Asset: "BTC", AmountHex: "ccdd"},
		   {TransferID: "t1", FromBook: "f", ToBook: "g", Asset: "BTC", AmountHex: "aabb"},
	  }
	   r1 := tracesnapgate.ComputeTransfersRoot(hops1)
	  r2 := tracesnapgate.ComputeTransfersRoot(hops2)
	   if r1 != r2 {
		  t.Errorf("摘要根应与顺序无关: %s != %s", r1, r2)
	   }
   }

// TestBooksRootOrderIndependent book 摘要根不受输入顺序影响。
  func TestBooksRootOrderIndependent(t *testing.T) {
	  books1 := []tracesnapgate.BookRecord{
		  {Book: "funding", Subject: "avail", Asset: "ETH", Available: "1.0", Frozen: "0"},
		  {Book: "trading", Subject: "avail", Asset: "ETH", Available: "0.5", Frozen: "0"},
	   }
	  books2 := []tracesnapgate.BookRecord{
		   {Book: "trading", Subject: "avail", Asset: "ETH", Available: "0.5", Frozen: "0"},
		   {Book: "funding", Subject: "avail", Asset: "ETH", Available: "1.0", Frozen: "0"},
	  }
	  r1 := tracesnapgate.ComputeBooksRoot(books1)
	  r2 := tracesnapgate.ComputeBooksRoot(books2)
	  if r1 != r2 {
		   t.Errorf("book 摘要根应与顺序无关: %s != %s", r1, r2)
	   }
  }

// TestEmptyRootNotEqual 空列表摘要根与非空列表不同。

  func TestEmptyRootNotEqual(t *testing.T) {
	  empty := tracesnapgate.ComputeTransfersRoot(nil)
	   nonEmpty := tracesnapgate.ComputeTransfersRoot([]tracesnapgate.HopRecord{
		   {TransferID: "t1", FromBook: "f", ToBook: "g", Asset: "BTC", AmountHex: "aabb"},
	  })
	   if empty == nonEmpty {
		   t.Error("空列表与非空列表摘要根不应相同")
	  }
   }

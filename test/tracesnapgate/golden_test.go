package tracesnapgate_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/investigate"
	"account-assets.kucoin.net/funding/internal/tracesnapgate"
)

// ─── 辅助：加载 testdata golden JSON ─────────────────────────────────────────

  func loadGolden(t *testing.T, name string) []byte {
	  t.Helper()
	   path := filepath.Join("..", "..", "testdata", "tracesnapgate", name)
	  data, err := os.ReadFile(path)
	  if err != nil {
		   t.Skipf("golden 文件不存在，跳过: %v", err)
	   }
	   return data
   }

// ─── 验收 1：Seal 后活账变更 → Replay 报漂移 ──────────────────────────────

   func TestGoldenSealThenDrift(t *testing.T) {
	   _ = loadGolden(t, "seal_then_drift.json") // 确认文件存在。
	  ctx := context.Background()
	   prov := &stubProvider{}
	   // 初始 hops（密封时）。
	   prov.setHops([]investigate.Hop{
		  {ID: "tr-aaa-001", FromBook: "funding", ToBook: "trading", Asset: "BTC", Amount: "0.1"},
	   })
	  prov.setBooks([]tracesnapgate.SnapPosition{
		  {Book: "funding", Subject: "available", Asset: "BTC", Available: "0.9", Frozen: "0"},
		  {Book: "trading", Subject: "available", Asset: "BTC", Available: "0.1", Frozen: "0"},
	  })
	  eng := newTestEngine(prov, defaultCfg())
	  snap, err := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{
		  UserID: "user-001", Asset: "BTC", Actor: "ops-golden",
	   })
	   if err != nil {
		  t.Fatalf("[golden-1] 创建快照失败: %v", err)
	   }
	   sealed, err := eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: snap.ID, Actor: "ops-golden"})
	  if err != nil {
		   t.Fatalf("[golden-1] 密封失败: %v", err)
	   }
	  // 密封后活账增加新 hop（漂移场景）。
	   prov.setHops([]investigate.Hop{
		   {ID: "tr-aaa-001", FromBook: "funding", ToBook: "trading", Asset: "BTC", Amount: "0.1"},
		   {ID: "tr-bbb-002", FromBook: "trading", ToBook: "margin", Asset: "BTC", Amount: "0.05"},
	   })
	  prov.setBooks([]tracesnapgate.SnapPosition{
		  {Book: "funding", Subject: "available", Asset: "BTC", Available: "0.9", Frozen: "0"},
		   {Book: "trading", Subject: "available", Asset: "BTC", Available: "0", Frozen: "0"},
		  {Book: "margin", Subject: "available", Asset: "BTC", Available: "0.1", Frozen: "0"},
	   })
	   // Replay 必须返回漂移错误，不得返回活账视图作为法证结论。
	   res, err := eng.Replay(ctx, sealed.ID)
	   if err == nil {
		   t.Fatal("[golden-1] 期望漂移错误，Replay 返回 nil error")
	   }
	   if res.Consistent {
		   t.Error("[golden-1] 漂移场景下 Consistent 不应为 true")
	   }
	   de, isDrift := tracesnapgate.IsDriftError(err)
	  if !isDrift {
		  t.Fatalf("[golden-1] 期望 DriftError，实际 %T", err)
	   }
	  if de.ProofID == "" {
		  t.Error("[golden-1] 漂移证明 ID 不应为空")
	  }
	   // 验证证明中密封根与快照一致。
	   proof, perr := eng.GetProof(ctx, de.ProofID)
	   if perr != nil {
		  t.Fatalf("[golden-1] 读取证明失败: %v", perr)
	  }
	   if proof.SealedTransfersRoot != sealed.TransfersRoot {
		  t.Errorf("[golden-1] SealedTransfersRoot 不一致")
	  }
	  if proof.SealedBooksRoot != sealed.BooksRoot {
		  t.Errorf("[golden-1] SealedBooksRoot 不一致")
	  }
	   t.Logf("[golden-1] 漂移证明 ID=%s type=%s", proof.ID, proof.DriftType)
  }

// ─── 验收 2：缺 snapshot_gen 导出拒绝 ─────────────────────────────────────

   func TestGoldenExportWithoutSnapshotGen(t *testing.T) {
	  _ = loadGolden(t, "export_without_snapshot_gen.json")
	   ctx := context.Background()
	   prov := &stubProvider{}
	   store := tracesnapgate.NewMemStore()
	   eng := tracesnapgate.NewEngine(store, prov, defaultCfg())
	   // 注入缺 snapshot_gen 的密封快照。
	  now := time.Now().UTC()
	   sealedAt := now
	  noGen := tracesnapgate.TraceSnapshot{
		   ID:          "no-gen-golden",
		  SnapshotGen: "",
		   UserID:      "user-002",
		   Asset:  "ETH",
		   Status: tracesnapgate.StatusSealed,
		  TransfersRoot: "aabbccddeeff0011",
		  BooksRoot:     "1122334455667788",
		  AsOf:          now,
		   SealedAt: &sealedAt,
		   CreatedAt: now,
	  }
	  if err := store.PutSnapshot(ctx, noGen); err != nil {
		  t.Fatalf("[golden-2] 注入快照失败: %v", err)
	  }
	   _, err := eng.Export(ctx, tracesnapgate.ExportRequest{SnapshotID: "no-gen-golden", Actor: "ops"})
	  if !errors.Is(err, tracesnapgate.ErrNoSnapshot) {
		  t.Errorf("[golden-2] 缺 snapshot_gen 期望 ErrNoSnapshot，实际 %v", err)
	   }
	   t.Log("[golden-2] fail-closed 验证通过：缺 snapshot_gen 已拒绝导出")
   }

// ─── 验收 3：多次 Replay 摘要幂等 ─────────────────────────────────────────

   func TestGoldenReplayIdempotent(t *testing.T) {
	   _ = loadGolden(t, "replay_idempotent.json")
	  ctx := context.Background()
	   prov := &stubProvider{}
	   prov.setHops([]investigate.Hop{
		   {ID: "tr-usdt-001", FromBook: "funding", ToBook: "earn", Asset: "USDT", Amount: "100"},
	  })
	   prov.setBooks([]tracesnapgate.SnapPosition{
		   {Book: "funding", Subject: "available", Asset: "USDT", Available: "900", Frozen: "0"},
		  {Book: "earn", Subject: "available", Asset: "USDT", Available: "100", Frozen: "0"},
	  })
	   eng := newTestEngine(prov, defaultCfg())
	   snap, _ := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{
		   UserID: "user-003", Asset: "USDT", Actor: "ops",
	  })
	   sealed, err := eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: snap.ID, Actor: "ops"})
	   if err != nil {
		   t.Fatalf("[golden-3] 密封失败: %v", err)
	   }
	   // 活账不变；3 次 Replay 摘要必须完全一致。
	  var prevTRoot, prevBRoot string
	   for i := 0; i < 3; i++ {
		  res, rerr := eng.Replay(ctx, sealed.ID)
		   if rerr != nil {
			   t.Fatalf("[golden-3] 第 %d 次 Replay 失败: %v", i+1, rerr)
		  }
		   if !res.Consistent {
			   t.Errorf("[golden-3] 第 %d 次 Replay 不一致", i+1)
		  }
		   if i == 0 {
			  prevTRoot = res.TransfersRoot
			  prevBRoot = res.BooksRoot
		   } else {
			   if res.TransfersRoot != prevTRoot {
				  t.Errorf("[golden-3] 第 %d 次 TransfersRoot 变化（非幂等）", i+1)
			  }
			   if res.BooksRoot != prevBRoot {
				   t.Errorf("[golden-3] 第 %d 次 BooksRoot 变化（非幂等）", i+1)
			  }
		   }
	  }
	   t.Logf("[golden-3] Replay 幂等验证通过：TransfersRoot=%s", prevTRoot)
  }

// ─── 验收 4：案件绑定导出携带同一 snapshot_gen ──────────────────────────────

  func TestGoldenCaseBindAndExport(t *testing.T) {
	   _ = loadGolden(t, "case_bind_and_export.json")
	  ctx := context.Background()
	   prov := &stubProvider{hops: sampleHops(), books: sampleBooks()}
	   eng := newTestEngine(prov, defaultCfg())
	  snap, _ := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{
		   UserID: "user-004", Asset: "BTC", Actor: "ops",
	  })
	  sealed, err := eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: snap.ID, Actor: "ops"})
	   if err != nil {
		  t.Fatalf("[golden-4] 密封失败: %v", err)
	   }
	  bind, err := eng.BindCase(ctx, tracesnapgate.BindCaseRequest{
		  CaseID: "case-c1", SnapshotID: sealed.ID, BoundBy: "ops",
	   })
	   if err != nil {
		   t.Fatalf("[golden-4] 绑定案件失败: %v", err)
	   }
	  if bind.SnapshotGen != sealed.SnapshotGen {
		  t.Errorf("[golden-4] 绑定 SnapshotGen 不匹配: %s != %s", bind.SnapshotGen, sealed.SnapshotGen)
	   }
	   bundle, err := eng.Export(ctx, tracesnapgate.ExportRequest{SnapshotID: sealed.ID, Actor: "ops"})
	   if err != nil {
		  t.Fatalf("[golden-4] 导出失败: %v", err)
	  }
	  if bundle.Snapshot.SnapshotGen != sealed.SnapshotGen {
		   t.Errorf("[golden-4] 导出 snapshot_gen 不匹配")
	   }
	  if bundle.Bind == nil {
		  t.Error("[golden-4] 导出 bundle 应包含 bind 记录")
	  } else if bundle.Bind.SnapshotGen != sealed.SnapshotGen {
		  t.Errorf("[golden-4] 导出 bundle bind.SnapshotGen 不匹配")
	   }
	   t.Logf("[golden-4] 案件绑定导出验证通过：snapshot_gen=%s", sealed.SnapshotGen)
   }

// ─── 验收 5：新 Seal 取代旧快照 ──────────────────────────────────────────

   func TestGoldenSupersede(t *testing.T) {
	  _ = loadGolden(t, "supersede_on_reseal.json")
	  ctx := context.Background()
	   prov := &stubProvider{hops: sampleHops(), books: sampleBooks()}
	   eng := newTestEngine(prov, defaultCfg())
	   // Seal 旧快照。
	   sOld, _ := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{
		   UserID: "user-005", Asset: "ETH", Actor: "ops",
	   })
	  _, err := eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: sOld.ID, Actor: "ops"})
	   if err != nil {
		   t.Fatalf("[golden-5] 旧快照密封失败: %v", err)
	   }
	   // Seal 新快照（同用户/同资产）。
	  sNew, _ := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{
		   UserID: "user-005", Asset: "ETH", Actor: "ops",
	   })
	  sNewSealed, err := eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: sNew.ID, Actor: "ops"})
	  if err != nil {
		  t.Fatalf("[golden-5] 新快照密封失败: %v", err)
	  }
	  // 读取旧快照，验证已被取代。
	  oldUpdated, err := eng.GetSnapshot(ctx, sOld.ID)
	   if err != nil {
		   t.Fatalf("[golden-5] 读取旧快照失败: %v", err)
	  }
	   if oldUpdated.Status != tracesnapgate.StatusSuperseded {
		   t.Errorf("[golden-5] 旧快照期望 superseded，实际 %s", oldUpdated.Status)
	  }
	  if oldUpdated.SupersededBy != sNewSealed.ID {
		  t.Errorf("[golden-5] SupersededBy 不匹配: %s != %s", oldUpdated.SupersededBy, sNewSealed.ID)
	  }
	  // 旧快照导出应被拒绝。
	   _, exportErr := eng.Export(ctx, tracesnapgate.ExportRequest{SnapshotID: sOld.ID, Actor: "ops"})
	   if !errors.Is(exportErr, tracesnapgate.ErrSuperseded) {
		   t.Errorf("[golden-5] 旧快照导出期望 ErrSuperseded，实际 %v", exportErr)
	  }
	   t.Logf("[golden-5] supersede 验证通过：旧=%s → 新=%s", sOld.ID, sNewSealed.ID)
   }

// ─── JSON 序列化往返 ───────────────────────────────────────────────────────
// TestSnapshotJSONRoundtrip TraceSnapshot 的 JSON 序列化/反序列化无损。

  func TestSnapshotJSONRoundtrip(t *testing.T) {
	  ctx := context.Background()
	  prov := &stubProvider{hops: sampleHops(),
		   books: sampleBooks()}
	  eng := newTestEngine(prov, defaultCfg())
	   snap, _ := eng.CreateSnapshot(ctx, tracesnapgate.CreateRequest{
		  UserID: "u-json", Asset: "BTC", Actor: "ops",
	   })
	   sealed, err := eng.Seal(ctx, tracesnapgate.SealRequest{SnapshotID: snap.ID, Actor: "ops"})
	  if err != nil {
		  t.Fatalf("密封失败: %v", err)
	   }
	  data, err := json.Marshal(sealed)
	   if err != nil {
		   t.Fatalf("JSON 序列化失败: %v", err)
	  }
	  var restored tracesnapgate.TraceSnapshot
	   if err := json.Unmarshal(data, &restored); err != nil {
		  t.Fatalf("JSON 反序列化失败: %v", err)
	  }
	  if restored.ID != sealed.ID {
		   t.Errorf("ID 不一致: %s != %s", restored.ID, sealed.ID)
	  }
	  if restored.TransfersRoot != sealed.TransfersRoot {
		   t.Errorf("TransfersRoot 不一致")
	  }
	   if restored.BooksRoot != sealed.BooksRoot {
		  t.Errorf("BooksRoot 不一致")
	  }
	  if restored.SnapshotGen != sealed.SnapshotGen {
		  t.Errorf("SnapshotGen 不一致")
	   }
   }

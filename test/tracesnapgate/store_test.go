package tracesnapgate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/tracesnapgate"
)

// ─── Snapshot 操作 ───────────────────────────────────────────────────

   func TestMemStoreSnapshot(t *testing.T) {
	   ctx := context.Background()
	   store := tracesnapgate.NewMemStore()
	  now := time.Now().UTC()
	  tests := []struct {
		   name string
		   snap tracesnapgate.TraceSnapshot
		  getErr error
	  }{
		  {
			  name: "正常写入读取",
			   snap: tracesnapgate.TraceSnapshot{
				   ID: "snap-1", SnapshotGen: "gen-1", UserID: "u1", Asset: "BTC",
				   Status: tracesnapgate.StatusOpen, CreatedAt: now,
			  },
		   },
		   {
			   name: "带 idem_key 写入",
			  snap: tracesnapgate.TraceSnapshot{
				  ID: "snap-2", SnapshotGen: "gen-2", UserID: "u2", Asset: "ETH",
				   Status: tracesnapgate.StatusSealed, IdemKey: "idem-snap-2", CreatedAt: now,
			  },
		   },
	   }
	  for _, tt := range tests {
		  t.Run(tt.name, func(t *testing.T) {
			   if err := store.PutSnapshot(ctx, tt.snap); err != nil {
				  t.Fatalf("PutSnapshot 失败: %v", err)
			  }
			  got, err := store.GetSnapshot(ctx, tt.snap.ID)
			  if tt.getErr != nil {
				   if !errors.Is(err, tt.getErr) {
					  t.Errorf("期望 %v，实际 %v", tt.getErr, err)
				   }
				  return
			  }
			   if err != nil {
				   t.Fatalf("GetSnapshot 失败: %v", err)
			   }
			   if got.ID != tt.snap.ID {
				   t.Errorf("ID 不匹配: %s != %s", got.ID, tt.snap.ID)
			  }
		   })
	  }
	   // 读不存在的快照应返回 ErrNotFound。
	   _, err := store.GetSnapshot(ctx, "non-existent")
	  if !errors.Is(err, tracesnapgate.ErrNotFound) {
		   t.Errorf("读不存在的快照期望 ErrNotFound，实际 %v", err)
	   }
   }

  func TestMemStoreSnapshotsByUser(t *testing.T) {
	  ctx := context.Background()
	  store := tracesnapgate.NewMemStore()
	  t0 := time.Now().UTC()
	  for i := 0; i < 5; i++ {
		  store.PutSnapshot(ctx, tracesnapgate.TraceSnapshot{ // nolint
			   ID:     "snap-user-" + string(rune('a'+i)),
			   UserID: "user-list",
			   Asset: "BTC",
			   Status:    tracesnapgate.StatusOpen,
			   CreatedAt: t0.Add(time.Duration(i) * time.Second),
		   })
	  }
	   snaps, err := store.SnapshotsByUser(ctx, "user-list", 3)
	  if err != nil {
		  t.Fatalf("SnapshotsByUser 失败: %v", err)
	   }
		  UserID: "u-latest",
		   Asset:    "BTC",
		  Status:   tracesnapgate.StatusSealed,
		   SealedAt: &sealedAt,
		   AsOf:      t0,
		   CreatedAt: t0,
	   })
	  t1 := t0.Add(time.Second)
	   sealedAt1 := t1
	   store.PutSnapshot(ctx, tracesnapgate.TraceSnapshot{ // nolint
		  ID:     "snap-new-s",
		   UserID: "u-latest",
		   Asset: "BTC",
		  Status: tracesnapgate.StatusSealed,
		   SealedAt: &sealedAt1,
		   AsOf:     t1,
		  CreatedAt: t1,
	   })
	   latest, ok, err := store.LatestSealedByUserAsset(ctx, "u-latest", "BTC")
	  if err != nil {
		  t.Fatalf("LatestSealedByUserAsset 失败: %v", err)
	   }
	   if !ok {
		  t.Fatal("期望找到最新密封快照")
	  }
	  if latest.ID != "snap-new-s" {
		  t.Errorf("期望最新快照 ID snap-new-s，实际 %s", latest.ID)
	   }
   }

// ─── Proof 操作 ─────────────────────────────────────────────────────────

  func TestMemStoreProof(t *testing.T) {
	  ctx := context.Background()
	  store := tracesnapgate.NewMemStore()
	   now := time.Now().UTC()
	   proof := tracesnapgate.TraceDriftProof{
		  ID:          "proof-1",
		   SnapshotID:  "snap-x",
		   SnapshotGen: "gen-x",
		   UserID: "u-proof",
		  Asset:  "BTC",
		  DriftType: tracesnapgate.DriftTransfers,
		   ProofHash:  "deadbeef0011223344556677889900aa",
		   IdemKey:    "idem-proof-1",
		  DetectedAt: now,
	  }
	   if err := store.PutProof(ctx, proof); err != nil {
		  t.Fatalf("PutProof 失败: %v", err)
	   }
	   got, err := store.GetProof(ctx, "proof-1")
	   if err != nil {
		  t.Fatalf("GetProof 失败: %v", err)
	   }
	  if got.ID != proof.ID {
		   t.Errorf("ID 不匹配: %s != %s", got.ID, proof.ID)
	  }
	  // 按 hash 查找。
	   byHash, ok, err := store.ProofByHash(ctx, proof.ProofHash)
	   if err != nil || !ok {
		   t.Fatalf("ProofByHash 失败: %v (ok=%v)", err, ok)
	  }
	  if byHash.ID != proof.ID {
		  t.Errorf("按 hash 查找 ID 不匹配")
	   }
	   // 按快照 ID 查找。
	   proofs, err := store.ProofsBySnapshot(ctx, "snap-x")
	   if err != nil || len(proofs) != 1 {
		  t.Errorf("ProofsBySnapshot 期望 1 条，实际 %d (err=%v)", len(proofs), err)
	  }
   }

// ─── Bind 操作 ─────────────────────────────────────────────────────────

  func TestMemStoreBind(t *testing.T) {
	  ctx := context.Background()
	   store := tracesnapgate.NewMemStore()
	   now := time.Now().UTC()
	  bind := tracesnapgate.InvestigateCaseBind{
		  ID:     "bind-1",
		  CaseID: "case-x",
		  SnapshotID:  "snap-y",
		   SnapshotGen: "gen-y",
		  UserID: "u-bind",
		  Asset:  "ETH",
		   BoundAt: now,
		   BoundBy: "ops",
	  }
	   if err := store.PutBind(ctx, bind); err != nil {
		  t.Fatalf("PutBind 失败: %v", err)
	   }
	  got, err := store.GetBind(ctx, "bind-1")
	   if err != nil {
		   t.Fatalf("GetBind 失败: %v", err)
	  }
	  if got.CaseID != bind.CaseID {
		   t.Errorf("CaseID 不匹配: %s != %s", got.CaseID, bind.CaseID)
	   }
	  // 按 case_id 查找。
	   bCase, ok, err := store.BindByCaseID(ctx, "case-x")
	   if err != nil || !ok {
		   t.Fatalf("BindByCaseID 失败: %v (ok=%v)", err, ok)
	  }
	  if bCase.ID != bind.ID {
		  t.Errorf("BindByCaseID ID 不匹配")
	  }
	  // 按快照 ID 查找。
	  binds, err := store.BindsBySnapshot(ctx, "snap-y")
	   if err != nil || len(binds) != 1 {
		  t.Errorf("BindsBySnapshot 期望 1 条，实际 %d (err=%v)", len(binds), err)
	   }
  }

// TestMemStoreStoreFull 超容量时写入被拒绝。

   func TestMemStoreStoreFull(t *testing.T) {
	   ctx := context.Background()
	   store := tracesnapgate.NewMemStoreN(2) // 容量 2。
	  now := time.Now().UTC()
	  for i := 0; i < 2; i++ {
		   err := store.PutSnapshot(ctx, tracesnapgate.TraceSnapshot{
			   ID:        "snap-full-" + string(rune('a'+i)),
			   UserID:    "u-full",
			  Asset:     "BTC",
			   Status:    tracesnapgate.StatusOpen,
			   CreatedAt: now,
		  })
		   if err != nil {
			  t.Fatalf("写入第 %d 条失败: %v", i, err)
		   }
	   }
	  // 第三条应被拒绝。
	  err := store.PutSnapshot(ctx, tracesnapgate.TraceSnapshot{
		   ID: "snap-full-overflow",
		  UserID: "u-full",
		  Asset:     "BTC",
		   Status:    tracesnapgate.StatusOpen,
		   CreatedAt: now,
	   })
	   if !errors.Is(err, tracesnapgate.ErrStoreFull) {
		   t.Errorf("超容量写入期望 ErrStoreFull，实际 %v", err)
	   }
  }

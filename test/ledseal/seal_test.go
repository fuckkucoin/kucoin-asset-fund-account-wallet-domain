package ledseal_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/snap"
	"account-assets.kucoin.net/funding/internal/ledseal"
)

func TestSealRoot_BuildAndVerify_TableDriven(t *testing.T) {
	type tc struct {
		name		string
		driftRows	[]ledseal.DriftRow
		wantEmpty	bool	// drift_count == 0
	}
	now := time.Date(2022, 6, 14, 12, 0, 0, 0, time.UTC)
	cases := []tc{
		{
			name:		"empty_drift_list",
			driftRows:	[]ledseal.DriftRow{},
			wantEmpty:	true,
		},
		{
			name:	"single_minor_drift",
			driftRows: []ledseal.DriftRow{{
				ID:	"d1", BatchID: "b1", PlanID: "p1",
				UserID:	"u1", Book: "MAIN", Asset: "USDT",
				AvailBefore:	"100", AvailAfter: "100.01",
				FrozenBefore:	"0", FrozenAfter: "0",
				Severity:	ledseal.SeverityMinor, Kind: ledseal.KindMutation,
				CreatedAt:	now,
			}},
		},
		{
			name:	"single_critical_drift",
			driftRows: []ledseal.DriftRow{{
				ID:	"d2", BatchID: "b2", PlanID: "p1",
				UserID:	"u2", Book: "MAIN", Asset: "USDT",
				AvailBefore:	"100", AvailAfter: "50100",
				FrozenBefore:	"0", FrozenAfter: "0",
				Severity:	ledseal.SeverityCritical, Kind: ledseal.KindMutation, BlockWriteHint: true,
				CreatedAt:	now,
			}},
		},
		{
			name:	"multiple_mixed_drifts",
			driftRows: []ledseal.DriftRow{
				{ID: "d3a", BatchID: "b3", PlanID: "p1", UserID: "u3", Book: "MAIN", Asset: "USDT",
					AvailBefore:	"100", AvailAfter: "100.01", Severity: ledseal.SeverityMinor, CreatedAt: now},
				{ID: "d3b", BatchID: "b3", PlanID: "p1", UserID: "u3", Book: "TRADE", Asset: "BTC",
					AvailBefore:	"1", AvailAfter: "50100", Severity: ledseal.SeverityCritical, BlockWriteHint: true, CreatedAt: now},
				{ID: "d3c", BatchID: "b3", PlanID: "p1", UserID: "u3", Book: "EARN", Asset: "ETH",
					AvailBefore:	"0", AvailAfter: "5", Severity: ledseal.SeverityCritical, Kind: ledseal.KindAppearance, CreatedAt: now},
			},
		},
		{
			name:	"appearance_and_disappearance",
			driftRows: []ledseal.DriftRow{
				{ID: "d4a", BatchID: "b4", PlanID: "p1", UserID: "u4", Book: "MAIN", Asset: "NEW",
					AvailBefore:	"0", AvailAfter: "1000", Severity: ledseal.SeverityCritical, Kind: ledseal.KindAppearance, CreatedAt: now},
				{ID: "d4b", BatchID: "b4", PlanID: "p1", UserID: "u4", Book: "OLD", Asset: "USDT",
					AvailBefore:	"5000", AvailAfter: "0", Severity: ledseal.SeverityCritical, Kind: ledseal.KindDisappearance, CreatedAt: now},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := ledseal.NewMemStore()
			ctx := context.Background()
			batchID := "batch-" + tc.name
			// 写漂移行到 store
			for _, r := range tc.driftRows {
				r.BatchID = batchID
				if err := st.PutDriftRow(ctx, r); err != nil {
					t.Fatalf("put drift row: %v", err)
				}
			}
			batch := ledseal.DiffBatch{
				ID:	batchID, PlanID: "p1",
				FromGenSeq:	1, ToGenSeq: 2,
				State:		ledseal.BatchRunning,
				DriftCount:	len(tc.driftRows),
				MaxSeverity:	ledseal.MaxSeverity(tc.driftRows),
			}
			root, updatedBatch, err := ledseal.BuildSealRoot(ctx, st, batch, func() time.Time { return now })
			if err != nil {
				t.Fatalf("BuildSealRoot: %v", err)
			}
			// 验证基本字段
			if root.RootHash == "" {
				t.Error("root_hash must not be empty")
			}
			if root.BatchID != batchID {
				t.Errorf("want batch_id=%s, got %s", batchID, root.BatchID)
			}
			if root.DriftCount != len(tc.driftRows) {
				t.Errorf("want drift_count=%d, got %d", len(tc.driftRows), root.DriftCount)
			}
			if updatedBatch.State != ledseal.BatchSealed {
				t.Errorf("want sealed, got %s", updatedBatch.State)
			}
			// 验证封印根完整性
			if err := ledseal.VerifySealRoot(ctx, st, root.ID); err != nil {
				t.Errorf("VerifySealRoot: %v", err)
			}
			// 幂等性：重建 root 哈希必须相同
			rebuilt,
				err := ledseal.BuildRoot(batch, tc.driftRows, now)
			if err != nil {
				t.Fatalf("buildRoot for idempotency: %v", err)
			}
			if rebuilt.RootHash != root.RootHash {
				t.Errorf("root_hash not idempotent: first=%s rebuilt=%s", root.RootHash, rebuilt.RootHash)
			}
		})
	}
}

// TestSealRoot_HashOrder_Deterministic 验证漂移行顺序不影响 root_hash（排序后哈希）。

func TestSealRoot_HashOrder_Deterministic(t *testing.T) {
	now := time.Date(2022, 6, 14, 12, 0, 0, 0, time.UTC)
	rows1 := []ledseal.DriftRow{
		{ID: "b", BatchID: "bx", UserID: "u", Book: "MAIN", Asset: "USDT",
			AvailBefore:	"100", AvailAfter: "200", Severity: ledseal.SeverityCritical, CreatedAt: now},
		{ID: "a", BatchID: "bx", UserID: "u", Book: "TRADE", Asset: "BTC",
			AvailBefore:	"1", AvailAfter: "2", Severity: ledseal.SeverityMinor, CreatedAt: now},
	}
	rows2 := []ledseal.DriftRow{rows1[1], rows1[0]}	// 顺序颠倒
	batch := ledseal.DiffBatch{ID: "bx", PlanID: "p1", FromGenSeq: 1, ToGenSeq: 2, DriftCount: 2}
	root1, err1 := ledseal.BuildRoot(batch, rows1, now)
	root2, err2 := ledseal.BuildRoot(batch, rows2, now)
	if err1 != nil || err2 != nil {
		t.Fatalf("buildRoot errors: %v %v", err1, err2)
	}
	if root1.RootHash != root2.RootHash {
		t.Errorf("root_hash must be deterministic regardless of row order: %s vs %s", root1.RootHash, root2.RootHash)
	}
}

// TestSealRoot_TamperDetection 验证修改漂移行内容后 VerifySealRoot 失败。
func TestSealRoot_TamperDetection(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2022, 6, 14, 12, 0, 0, 0, time.UTC)
	st := ledseal.NewMemStore()
	batchID := "tamper-batch"
	origRow := ledseal.DriftRow{
		ID:	"dr-tamper", BatchID: batchID, PlanID: "p1",
		UserID:	"u1", Book: "MAIN", Asset: "USDT",
		AvailBefore:	"100", AvailAfter: "200",
		Severity:	ledseal.SeverityMajor, CreatedAt: now,
	}
	_ = st.PutDriftRow(ctx, origRow)
	batch := ledseal.DiffBatch{
		ID:	batchID, PlanID: "p1", State: ledseal.BatchRunning,
		FromGenSeq:	1, ToGenSeq: 2, DriftCount: 1,
		MaxSeverity:	ledseal.SeverityMajor,
	}
	root, _, err := ledseal.BuildSealRoot(ctx, st, batch, func() time.Time { return now })
	if err != nil {
		t.Fatalf("BuildSealRoot: %v", err)
	}
	// 篡改：直接修改 store 中的漂移行金额
	tampered := origRow
	tampered.AvailAfter = "999999"	// 篡改金额
	_ = st.PutDriftRow(ctx, tampered)
	// VerifySealRoot 应失败
	if err := ledseal.VerifySealRoot(ctx, st, root.ID); err == nil {
		t.Error("want error after tamper, got nil")
	}
}

// TestSealRoot_AlreadySealed_RejectsReseal 验证 sealed 批次拒绝二次封印。

func TestSealRoot_AlreadySealed_RejectsReseal(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2022, 6, 14, 12, 0, 0, 0, time.UTC)
	st := ledseal.NewMemStore()
	batch := ledseal.DiffBatch{
		ID:	"sealed-batch", PlanID: "p1", State: ledseal.BatchRunning,
		FromGenSeq:	1,
		ToGenSeq:	2, DriftCount: 0,
		MaxSeverity:	ledseal.SeverityNone,
	}
	_ = st.PutBatch(ctx, batch)
	// 第一次封印应成功
	_, updatedBatch, err := ledseal.BuildSealRoot(ctx, st, batch, func() time.Time { return now })
	if err != nil {
		t.Fatalf("first seal: %v", err)
	}
	// 第二次封印应失败
	_,
		_, err = ledseal.BuildSealRoot(ctx, st, updatedBatch, func() time.Time { return now })
	if err == nil {
		t.Error("want error on second seal, got nil")
	}
}

// TestBuildSealRoot_NilStore 验证 nil store 返回 ErrUnavailable。
func TestBuildSealRoot_NilStore(t *testing.T) {
	ctx := context.Background()
	batch := ledseal.DiffBatch{State: ledseal.BatchRunning}
	_, _, err := ledseal.BuildSealRoot(ctx, nil, batch, nil)
	if err != ledseal.ErrUnavailable {
		t.Errorf("want ErrUnavailable, got %v", err)
	}
}

// TestSealRoot_EmptyDrifts_ValidHash 验证空漂移批次也能生成有效 root_hash。

func TestSealRoot_EmptyDrifts_ValidHash(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2022, 6, 14, 12, 0, 0, 0, time.UTC)
	st := ledseal.NewMemStore()
	batch := ledseal.DiffBatch{
		ID:	"empty-batch", PlanID: "p1", State: ledseal.BatchRunning,
		FromGenSeq:	1,
		ToGenSeq:	2, DriftCount: 0,
		MaxSeverity:	ledseal.SeverityNone,
	}
	_ = st.PutBatch(ctx, batch)
	root, updatedBatch, err := ledseal.BuildSealRoot(ctx, st, batch, func() time.Time { return now })
	if err != nil {
		t.Fatalf("BuildSealRoot empty: %v", err)
	}
	if root.RootHash == "" {
		t.Error("empty drift root_hash must not be empty")
	}
	if updatedBatch.State != ledseal.BatchSealed {
		t.Errorf("want sealed, got %s", updatedBatch.State)
	}
	if err := ledseal.VerifySealRoot(ctx, st, root.ID); err != nil {
		t.Errorf("verify empty drift root: %v", err)
	}
	// 确保不同空批次 root_hash 不同（含 batch_id 区分）
	batch2 := ledseal.DiffBatch{ID: "empty-batch-2", PlanID: "p1", State: ledseal.BatchRunning}
	root2,
		_, _ := ledseal.BuildSealRoot(ctx, st, batch2, func() time.Time { return now })
	if root.RootHash == root2.RootHash {
		t.Error("different empty batches should have different root hashes")
	}
}

// TestSealSvc_SnapPair 通过 Service.RunAndSealBatch 端到端验证封印根。
func TestSealSvc_SnapPair(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2022, 6, 14, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	st := snap.NewMemStore()
	svc := newTestSvc(st)
	svc.WithClock(func() time.Time { return t1 })
	snp0 := buildTestSnap(t, st, "u-seal", [][]string{{"MAIN", "USDT", "1000", "0"}}, t0)
	snp1 := buildTestSnap(t, st, "u-seal", [][]string{{"MAIN", "USDT", "1000", "0"}}, t1)
	plan, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{Name: "seal-test", PlanScope: ledseal.ScopeUserShard, ShardKey: "u-seal", IntervalSeconds: 3600})
	sl0, _ := svc.RegisterSlice(ctx, plan.ID, snp0.ID, "u-seal", "", snp0.Checksum, snp0.Count, 0, "", t0)
	sl1, _ := svc.RegisterSlice(ctx, plan.ID, snp1.ID, "u-seal", "", snp1.Checksum, snp1.Count, 0, "", t1)
	batch, _ := svc.OpenBatch(ctx, plan.ID, sl0.ID, sl1.ID, sl0.GenSeq, sl1.GenSeq)
	_, root, err := svc.RunAndSealBatch(ctx, batch.ID)
	if err != nil {
		t.Fatalf("run and seal: %v", err)
	}
	if err := svc.VerifyRoot(ctx, root.ID); err != nil {
		t.Errorf("verify root: %v", err)
	}
}

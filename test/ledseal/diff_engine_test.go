package ledseal_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/snap"
	"account-assets.kucoin.net/funding/internal/ledseal"
)

// ─────────────────────────────────────────────────────────
// 用快照构建辅助
// ─────────────────────────────────────────────────────────
// buildTestSnap 把 book/asset/avail/frozen 四元组构建成 snap.Snapshot 并存入 st。

func buildTestSnap(t *testing.T, st *snap.Store, userID string, rows [][]string, takenAt time.Time) snap.Snapshot {
	t.Helper()
	views := make([]ledger.BookView,
		len(rows))
	for i, r := range rows {
		views[i] = ledger.BookView{
			Book:	r[0], Asset: r[1], Available: r[2], Frozen: r[3],
		}
	}
	snp, err := snap.CaptureViews(userID, views, takenAt)
	if err != nil {
		t.Fatalf("CaptureViews: %v", err)
	}
	snp, err = st.Put(context.Background(), snp)
	if err != nil {
		t.Fatalf("Put snap: %v", err)
	}
	return snp
}

// newTestSvc 构建带 snap.Store 共享的 Service。
func newTestSvc(st *snap.Store) *ledseal.Service {
	snapSvc := snap.NewWithStore(nil, nil, st)
	return ledseal.New(snapSvc, ledseal.NewMemStore())
}

// ─────────────────────────────────────────────────────────
// computeDrift 内核（通过 RunAndSealBatch 间接）
// ─────────────────────────────────────────────────────────

type diffCase struct {
	name		string
	uid		string
	gen0		[][]string	// [book, asset, avail, frozen]
	gen1		[][]string
	wantDrifts	int
	wantSeverity	ledseal.DriftSeverity
	wantKinds	map[string]ledseal.DriftKind	// book|asset → kind
	wantSealed	bool
	seqFromTo	[2]int64	// [fromSeq, toSeq]; 0 = auto (1,2)
	wantErr		bool
}

func TestDiffEngine_TableDriven(t *testing.T) {
	t0 := time.Date(2022, 6, 14, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	cases := []diffCase{
		// ── 无漂移场景 ──────────────────────────────────────────────────────────
		{
			name:		"no_drift_single_position",
			uid:		"u-nd-01",
			gen0:		[][]string{{"MAIN", "USDT", "1000", "0"}},
			gen1:		[][]string{{"MAIN", "USDT", "1000", "0"}},
			wantDrifts:	0, wantSeverity: ledseal.SeverityNone, wantSealed: true,
		},
		{
			name:	"no_drift_multiple_positions",
			uid:	"u-nd-02",
			gen0: [][]string{
				{"MAIN", "USDT", "1000", "0"},
				{"MAIN", "BTC", "0.5", "0"},
				{"TRADE", "USDT", "200", "50"},
			},
			gen1: [][]string{
				{"MAIN", "USDT", "1000", "0"},
				{"MAIN", "BTC", "0.5", "0"},
				{"TRADE", "USDT", "200", "50"},
			},
			wantDrifts:	0, wantSeverity: ledseal.SeverityNone, wantSealed: true,
		},
		{
			name:		"no_drift_empty_positions",
			uid:		"u-nd-03",
			gen0:		[][]string{},
			gen1:		[][]string{},
			wantDrifts:	0, wantSeverity: ledseal.SeverityNone, wantSealed: true,
		},
		// ── mutation 漂移场景 ─────────────────────────────────────────────────
		{
			name:		"mutation_avail_increase_minor",
			uid:		"u-mut-01",
			gen0:		[][]string{{"MAIN", "USDT", "100", "0"}},
			gen1:		[][]string{{"MAIN", "USDT", "100.01", "0"}},
			wantDrifts:	1, wantSeverity: ledseal.SeverityMinor,
			wantKinds:	map[string]ledseal.DriftKind{"MAIN|USDT": ledseal.KindMutation},
			wantSealed:	true,
		},
		{
			name:		"mutation_avail_decrease_minor",
			uid:		"u-mut-02",
			gen0:		[][]string{{"MAIN", "USDT", "100.01", "0"}},
			gen1:		[][]string{{"MAIN", "USDT", "100", "0"}},
			wantDrifts:	1, wantSeverity: ledseal.SeverityMinor,
			wantKinds:	map[string]ledseal.DriftKind{"MAIN|USDT": ledseal.KindMutation},
			wantSealed:	true,
		},
		{
			name:		"mutation_avail_major",
			uid:		"u-mut-03",
			gen0:		[][]string{{"MAIN", "USDT", "10000", "0"}},
			gen1:		[][]string{{"MAIN", "USDT", "10005", "0"}},
			wantDrifts:	1, wantSeverity: ledseal.SeverityMajor,
			wantKinds:	map[string]ledseal.DriftKind{"MAIN|USDT": ledseal.KindMutation},
			wantSealed:	true,
		},
		{
			name:		"mutation_avail_critical",
			uid:		"u-mut-04",
			gen0:		[][]string{{"MAIN", "USDT", "100", "0"}},
			gen1:		[][]string{{"MAIN", "USDT", "50100", "0"}},
			wantDrifts:	1, wantSeverity: ledseal.SeverityCritical,
			wantKinds:	map[string]ledseal.DriftKind{"MAIN|USDT": ledseal.KindMutation},
			wantSealed:	true,
		},
		{
			name:		"mutation_frozen_only",
			uid:		"u-mut-05",
			gen0:		[][]string{{"MAIN", "USDT", "1000", "0"}},
			gen1:		[][]string{{"MAIN", "USDT", "1000", "5000"}},
			wantDrifts:	1, wantSeverity: ledseal.SeverityCritical,
			wantKinds:	map[string]ledseal.DriftKind{"MAIN|USDT": ledseal.KindMutation},
			wantSealed:	true,
		},
		{
			name:	"mutation_multiple_drifts",
			uid:	"u-mut-06",
			gen0: [][]string{
				{"MAIN", "USDT", "1000", "0"},
				{"TRADE", "BTC", "1.0", "0"},
				{"EARN", "USDT", "500", "0"},
			},
			gen1: [][]string{
				{"MAIN", "USDT", "50100", "0"},	// critical
				{"TRADE", "BTC", "1.0", "0"},	// no change
				{"EARN", "USDT", "505", "0"},	// major
			},
			wantDrifts:	2, wantSeverity: ledseal.SeverityCritical,
			wantKinds: map[string]ledseal.DriftKind{
				"MAIN|USDT":	ledseal.KindMutation,
				"EARN|USDT":	ledseal.KindMutation,
			},
			wantSealed:	true,
		},
		// ── appearance 新增场景 ───────────────────────────────────────────────
		{
			name:	"appearance_new_book_asset",
			uid:	"u-app-01",
			gen0:	[][]string{{"MAIN", "USDT", "1000", "0"}},
			gen1: [][]string{
				{"MAIN", "USDT", "1000", "0"},
				{"EARN", "BTC", "2.0", "0"},	// 新增
			},
			wantDrifts:	1, wantSealed: true,
			wantKinds:	map[string]ledseal.DriftKind{"EARN|BTC": ledseal.KindAppearance},
		},
		{
			name:		"appearance_from_empty",
			uid:		"u-app-02",
			gen0:		[][]string{},
			gen1:		[][]string{{"MAIN", "USDT", "500", "0"}},
			wantDrifts:	1,
			wantSealed:	true,
			wantKinds:	map[string]ledseal.DriftKind{"MAIN|USDT": ledseal.KindAppearance},
		},
		// ── disappearance 消失场景 ────────────────────────────────────────────
		{
			name:	"disappearance_existing_position_gone",
			uid:	"u-dis-01",
			gen0: [][]string{
				{"MAIN", "USDT", "1000", "0"},
				{"TRADE", "ETH", "5.0", "1.0"},
			},
			gen1:		[][]string{{"MAIN", "USDT", "1000", "0"}},
			wantDrifts:	1, wantSealed: true,
			wantKinds:	map[string]ledseal.DriftKind{"TRADE|ETH": ledseal.KindDisappearance},
		},
		{
			name:		"disappearance_to_empty",
			uid:		"u-dis-02",
			gen0:		[][]string{{"MAIN", "USDT", "500", "0"}},
			gen1:		[][]string{},
			wantDrifts:	1, wantSealed: true,
			wantKinds:	map[string]ledseal.DriftKind{"MAIN|USDT": ledseal.KindDisappearance},
		},
		{
			name:	"disappearance_zero_position_not_drift",
			uid:	"u-dis-03",
			gen0: [][]string{
				{"MAIN", "USDT", "1000", "0"},
				{"EARN", "BTC", "0", "0"},	// 零 position 消失不算漂移
			},
			gen1:		[][]string{{"MAIN", "USDT", "1000", "0"}},
			wantDrifts:	0, wantSealed: true,
		},
		// ── 序号间隙 / 回滚（通过直接注入 GenSeq 的切片）────────────────
		// 注意：seqFromTo 字段在此组用于标记为需要特殊处理，实际由后续单独覆盖
	}
	// 普通 diff 用例（无 seqFromTo 异常）
	normalCases := cases
	for _, tc := range normalCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			st := snap.NewMemStore()
			svc := newTestSvc(st)
			svc.WithClock(func() time.Time { return t1 })
			plan, err := svc.CreatePlan(ctx, ledseal.CreatePlanReq{
				Name:			tc.name + "-plan",
				PlanScope:		ledseal.ScopeUserShard,
				ShardKey:		tc.uid,
				IntervalSeconds:	3600,
				CreatedBy:		"test",
			})
			if err != nil {
				t.Fatalf("create plan: %v", err)
			}
			snp0 := buildTestSnap(t, st, tc.uid, tc.gen0, t0)
			snp1 := buildTestSnap(t, st, tc.uid, tc.gen1, t1)
			sl0, err := svc.RegisterSlice(ctx, plan.ID, snp0.ID, tc.uid, "",
				snp0.Checksum, snp0.Count, 0, "", t0)
			if err != nil {
				t.Fatalf("register slice 0: %v", err)
			}
			sl1, err := svc.RegisterSlice(ctx, plan.ID, snp1.ID, tc.uid, "",
				snp1.Checksum, snp1.Count, 0, "", t1)
			if err != nil {
				t.Fatalf("register slice 1: %v", err)
			}
			// 确定 fromSeq / toSeq
			fromSeq := sl0.GenSeq
			toSeq := sl1.GenSeq
			if tc.seqFromTo[1] != 0 {
				fromSeq = tc.seqFromTo[0]
				toSeq = tc.seqFromTo[1]
			}
			batch, err := svc.OpenBatch(ctx, plan.ID, sl0.ID, sl1.ID, fromSeq, toSeq)
			if err != nil {
				t.Fatalf("open batch: %v", err)
			}
			batch, _, runErr := svc.RunAndSealBatch(ctx, batch.ID)
			if tc.wantErr {
				if runErr == nil {
					t.Errorf("%s: want error, got nil", tc.name)
				}
			} else {
				if runErr != nil {
					t.Errorf("%s: unexpected error: %v", tc.name, runErr)
				}
			}
			// 验证批次状态
			if tc.wantSealed {
				if batch.State != ledseal.BatchSealed {
					t.Errorf("%s: want sealed, got %s", tc.name, batch.State)
				}
			} else {
				if batch.State == ledseal.BatchSealed {
					t.Errorf("%s: must NOT be sealed", tc.name)
				}
				if batch.State != ledseal.BatchFailed {
					t.Errorf("%s: want failed, got %s", tc.name, batch.State)
				}
			}
			// 验证漂移数
			if batch.DriftCount != tc.wantDrifts {
				t.Errorf("%s: want %d drifts, got %d", tc.name, tc.wantDrifts, batch.DriftCount)
			}
			// 验证最大严重度
			if tc.wantSeverity != "" && batch.MaxSeverity != tc.wantSeverity {
				t.Errorf("%s: want severity %s, got %s", tc.name, tc.wantSeverity, batch.MaxSeverity)
			}
			// 验证漂移行 kind
			if len(tc.wantKinds) > 0 {
				rows,
					_ := svc.ListDriftRows(ctx, batch.ID, 100)
				for _, r := range rows {
					key := r.Book + "|" + r.Asset
					if want, ok := tc.wantKinds[key]; ok {
						if r.Kind != want {
							t.Errorf("%s: drift %s: want kind %s, got %s", tc.name, key, want, r.Kind)
						}
					}
				}
			}
		})
	}
}

// TestDiffEngine_SeqGap_DirectSliceInjection 通过直接注入 GenSeq 间隙和回滚。
// RunBatch 的 seq 连续性检查基于 fromSlice.GenSeq 和 toSlice.GenSeq，
// 此直接写入目标 GenSeq 的切片以触发间隙/回滚路径。

func TestDiffEngine_SeqGap_DirectSliceInjection(t *testing.T) {
	type seqTC struct {
		name		string
		fromSeq		int64
		toSeq		int64
		wantErr		error
		wantState	ledseal.BatchState
	}
	cases := []seqTC{
		{name: "gap_by_2", fromSeq: 1, toSeq: 3, wantErr: ledseal.ErrSliceGap, wantState: ledseal.BatchFailed},
		{name: "gap_by_5", fromSeq: 1, toSeq: 6, wantErr: ledseal.ErrSliceGap, wantState: ledseal.BatchFailed},
		{name: "rollback", fromSeq: 5, toSeq: 3, wantErr: ledseal.ErrSliceRollback, wantState: ledseal.BatchFailed},
		{name: "equal_seqs", fromSeq: 3, toSeq: 3, wantErr: ledseal.ErrSliceRollback, wantState: ledseal.BatchFailed},
		{name: "consecutive", fromSeq: 1, toSeq: 2, wantErr: nil, wantState: ledseal.BatchSealed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			t0 := time.Date(2022, 6, 14, 10, 0, 0, 0, time.UTC)
			t1 := t0.Add(time.Hour)
			st := snap.NewMemStore()
			svc := newTestSvc(st)
			svc.WithClock(func() time.Time { return t1 })
			// 构建两个快照（内容相同，不产生漂移）
			snp0 := buildTestSnap(t, st, "u-seq", [][]string{{"MAIN", "USDT", "100", "0"}}, t0)
			snp1 := buildTestSnap(t, st, "u-seq", [][]string{{"MAIN", "USDT", "100", "0"}}, t1)
			plan, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{
				Name:	"seq-inject-" + tc.name, PlanScope: ledseal.ScopeUserShard, ShardKey: "u-seq", IntervalSeconds: 3600,
			})
			// 直接注入带目标 GenSeq 的切片（绕过 RegisterSlice 的自增逻辑）
			sl0 := ledseal.SnapSlice{
				ID:	"sl-from-" + tc.name, PlanID: plan.ID, SnapID: snp0.ID,
				UserID:	"u-seq", GenSeq: tc.fromSeq, Checksum: snp0.Checksum,
				PositionCount:	snp0.Count, State: "ready", TakenAt: t0, CreatedAt: t0,
			}
			sl1 := ledseal.SnapSlice{
				ID:	"sl-to-" + tc.name, PlanID: plan.ID, SnapID: snp1.ID,
				UserID:	"u-seq", GenSeq: tc.toSeq, Checksum: snp1.Checksum,
				PositionCount:	snp1.Count, State: "ready", TakenAt: t1, CreatedAt: t1,
			}
			_ = svc.store.PutSlice(ctx, sl0)
			_ = svc.store.PutSlice(ctx, sl1)
			batch, _ := svc.OpenBatch(ctx, plan.ID, sl0.ID, sl1.ID, tc.fromSeq, tc.toSeq)
			_, _, err := svc.RunAndSealBatch(ctx, batch.ID)
			if tc.wantErr != nil {
				if err == nil {
					t.Errorf("want error %v, got nil", tc.wantErr)
				}
			} else {
				if err != nil {
					t.Errorf("want nil, got %v", err)
				}
			}
			b, _ := svc.GetBatch(ctx, batch.ID)
			if b.State != tc.wantState {
				t.Errorf("want state=%s, got %s", tc.wantState, b.State)
			}
			if tc.wantState == ledseal.BatchFailed && b.State == ledseal.BatchSealed {
				t.Error("failed batch must NOT be sealed")
			}
		})
	}
}
func TestDiffEngine_BlockWriteHint(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2022, 6, 14, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	st := snap.NewMemStore()
	svc := newTestSvc(st)
	svc.WithClock(func() time.Time { return t1 })
	snp0 := buildTestSnap(t, st, "u-bwh", [][]string{{"MAIN", "USDT", "100", "0"}}, t0)
	snp1 := buildTestSnap(t, st, "u-bwh", [][]string{{"MAIN", "USDT", "50100", "0"}}, t1)
	plan, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{Name: "bwh-test", PlanScope: ledseal.ScopeUserShard, ShardKey: "u-bwh", IntervalSeconds: 3600})
	sl0, _ := svc.RegisterSlice(ctx, plan.ID, snp0.ID, "u-bwh", "", snp0.Checksum, snp0.Count, 0, "", t0)
	sl1, _ := svc.RegisterSlice(ctx, plan.ID, snp1.ID, "u-bwh", "", snp1.Checksum, snp1.Count, 0, "", t1)
	batch, _ := svc.OpenBatch(ctx, plan.ID, sl0.ID, sl1.ID, sl0.GenSeq, sl1.GenSeq)
	_, _, _ = svc.RunAndSealBatch(ctx, batch.ID)
	rows, _ := svc.ListDriftRows(ctx, batch.ID, 10)
	if len(rows) == 0 {
		t.Fatal("want drift rows")
	}
	for _, r := range rows {
		if r.Severity == ledseal.SeverityCritical && !r.BlockWriteHint {
			t.Errorf("critical drift must have block_write_hint=true, got false for %s|%s", r.Book, r.Asset)
		}
		if r.Severity != ledseal.SeverityCritical && r.BlockWriteHint {
			t.Errorf("non-critical drift must NOT have block_write_hint=true, got true for %s|%s", r.Book, r.Asset)
		}
	}
}

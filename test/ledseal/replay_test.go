package ledseal_test

import (
	"context"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/snap"
	"account-assets.kucoin.net/funding/internal/ledseal"
)

func TestReplayChecker_TableDriven(t *testing.T) {
	t0 := time.Date(2022, 6, 14, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	type tc struct {
		name		string
		uid		string
		positions	[][]string
		wantMatch	bool
		wantErrNil	bool
	}
	cases := []tc{
		{
			name:		"match_single_position",
			uid:		"u-replay-01",
			positions:	[][]string{{"MAIN", "USDT", "1000", "0"}},
			wantMatch:	true, wantErrNil: true,
		},
		{
			name:	"match_multiple_positions",
			uid:	"u-replay-02",
			positions: [][]string{
				{"MAIN", "USDT", "1000", "0"},
				{"TRADE", "BTC", "0.5", "0.1"},
				{"EARN", "ETH", "10.0", "0"},
			},
			wantMatch:	true, wantErrNil: true,
		},
		{
			name:		"match_empty_positions",
			uid:		"u-replay-03",
			positions:	[][]string{},
			wantMatch:	true, wantErrNil: true,
		},
		{
			name:	"match_frozen_nonzero",
			uid:	"u-replay-04",
			positions: [][]string{
				{"MAIN", "USDT", "500.00", "100.00"},
				{"MAIN", "BTC", "0.25", "0.05"},
			},
			wantMatch:	true, wantErrNil: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			st := snap.NewMemStore()
			svc := newTestSvc(st)
			svc.WithClock(func() time.Time { return t1 })
			plan, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{
				Name:			"replay-" + tc.name,
				PlanScope:		ledseal.ScopeUserShard,
				ShardKey:		tc.uid,
				IntervalSeconds:	3600,
			})
			snp := buildTestSnap(t, st, tc.uid, tc.positions, t0)
			sl, _ := svc.RegisterSlice(ctx, plan.ID, snp.ID, tc.uid, "",
				snp.Checksum, snp.Count, 0, "", t0)
			rc := ledseal.NewReplayChecker(svc)
			res, err := rc.CheckSlice(ctx, sl.ID)
			if tc.wantErrNil {
				if err != nil {
					t.Errorf("want nil error, got %v", err)
				}
			}
			if res.Match != tc.wantMatch {
				t.Errorf("want Match=%v, got %v (from=%s snap=%s)",
					tc.wantMatch, res.Match, res.FingerprintFrom, res.FingerprintSnap)
			}
		})
	}
}

// TestReplayChecker_BatchCheck 验证批次级别的回放校验。

func TestReplayChecker_BatchCheck(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2022, 6, 14, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	st := snap.NewMemStore()
	svc := newTestSvc(st)
	svc.WithClock(func() time.Time { return t1 })
	plan, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{
		Name:	"replay-batch", PlanScope: ledseal.ScopeUserShard, ShardKey: "u-rb", IntervalSeconds: 3600,
	})
	snp0 := buildTestSnap(t, st, "u-rb", [][]string{{"MAIN", "USDT", "1000", "0"}}, t0)
	snp1 := buildTestSnap(t, st, "u-rb", [][]string{{"MAIN", "USDT", "1000", "0"}}, t1)
	sl0, _ := svc.RegisterSlice(ctx, plan.ID, snp0.ID, "u-rb", "", snp0.Checksum, snp0.Count, 0, "", t0)
	sl1, _ := svc.RegisterSlice(ctx, plan.ID, snp1.ID, "u-rb", "", snp1.Checksum, snp1.Count, 0, "", t1)
	batch, _ := svc.OpenBatch(ctx, plan.ID, sl0.ID, sl1.ID, sl0.GenSeq, sl1.GenSeq)
	_,
		_, _ = svc.RunAndSealBatch(ctx, batch.ID)
	rc := ledseal.NewReplayChecker(svc)
	results, err := rc.CheckBatch(ctx, batch.ID)
	if err != nil {
		t.Fatalf("CheckBatch: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("want 2 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Match {
			t.Errorf("batch replay mismatch for slice=%s", r.SliceID)
		}
	}
}

// TestReplayChecker_NonexistentSlice 验证不存在的 slice 返回错误。
func TestReplayChecker_NonexistentSlice(t *testing.T) {
	ctx := context.Background()
	st := snap.NewMemStore()
	svc := newTestSvc(st)
	rc := ledseal.NewReplayChecker(svc)
	_, err := rc.CheckSlice(ctx, "nonexistent-slice-id")
	if err == nil {
		t.Error("want error for nonexistent slice, got nil")
	}
}

// TestComputeFingerprint_Deterministic 验证指纹计算对相同 Positions 输出相同结果。

func TestComputeFingerprint_Deterministic(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2022, 6, 14, 10, 0, 0, 0, time.UTC)
	st := snap.NewMemStore()
	rows := [][]string{
		{"MAIN", "USDT", "1000", "0"},
		{"TRADE", "BTC", "0.5", "0.1"},
	}
	snp1 := buildTestSnap(t, st, "u-fp", rows, t0)
	snp2 := buildTestSnap(t, st, "u-fp", rows, t0)
	// 从 store 取回
	s1, _ := snap.NewWithStore(nil, nil, st).Get(ctx, snp1.ID)
	s2, _ := snap.NewWithStore(nil, nil, st).Get(ctx, snp2.ID)
	fp1 := ledseal.ComputeFingerprint(s1)
	fp2 := ledseal.ComputeFingerprint(s2)
	if fp1 != fp2 {
		t.Errorf("fingerprints differ for same positions: %s vs %s", fp1, fp2)
	}
	if fp1 == "" || fp2 == "" {
		t.Error("fingerprint must not be empty")
	}
}

// TestComputeFingerprint_DifferentPositions 验证不同 Positions 产生不同指纹。
func TestComputeFingerprint_DifferentPositions(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2022, 6, 14, 10, 0, 0, 0, time.UTC)
	st := snap.NewMemStore()
	snp1 := buildTestSnap(t, st, "u-fp-diff", [][]string{{"MAIN", "USDT", "100", "0"}}, t0)
	snp2 := buildTestSnap(t, st, "u-fp-diff", [][]string{{"MAIN", "USDT", "200", "0"}}, t0)
	s1, _ := snap.NewWithStore(nil, nil, st).Get(ctx, snp1.ID)
	s2, _ := snap.NewWithStore(nil, nil, st).Get(ctx, snp2.ID)
	fp1 := ledseal.ComputeFingerprint(s1)
	fp2 := ledseal.ComputeFingerprint(s2)
	if fp1 == fp2 {
		t.Error("different positions should produce different fingerprints")
	}
}

// TestReplayChecker_NilSvc 验证 nil service 返回 ErrUnavailable。
func TestReplayChecker_NilSvc(t *testing.T) {
	rc := &ledseal.ReplayChecker{svc: nil}
	_, err := rc.CheckSlice(context.Background(), "any")
	if err != ledseal.ErrUnavailable {
		t.Errorf("want ErrUnavailable, got %v", err)
	}
	// CheckBatch 同样
	_, err = rc.CheckBatch(context.Background(), "any")
	if err != ledseal.ErrUnavailable {
		t.Errorf("CheckBatch nil svc: want ErrUnavailable, got %v", err)
	}
}

// TestReplayChecker_FingerprintMatchesChecksum 验证 computeFingerprint 与 snap.Checksum 一致。

func TestReplayChecker_FingerprintMatchesChecksum(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2022, 6, 14, 10, 0, 0, 0, time.UTC)
	st := snap.NewMemStore()
	positions := [][]string{
		{"MAIN", "USDT", "500.00", "0"},
		{"TRADE", "BTC", "0.25", "0.05"},
		{"EARN", "ETH", "3.0", "0"},
		{"SAVING", "USDT", "1000", "0"},
	}
	snp := buildTestSnap(t, st, "u-ck", positions, t0)
	snapSvc := snap.NewWithStore(nil,
		nil, st)
	loaded, err := snapSvc.Get(ctx, snp.ID)
	if err != nil {
		t.Fatalf("load snap: %v", err)
	}
	fp := ledseal.ComputeFingerprint(loaded)
	if !strings.EqualFold(fp, loaded.Checksum) {
		t.Errorf("fingerprint=%s checksum=%s: must match", fp, loaded.Checksum)
	}
}

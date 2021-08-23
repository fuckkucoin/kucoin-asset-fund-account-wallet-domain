package ledseal_test

import (
	"context"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/snap"
	"account-assets.kucoin.net/funding/internal/ledseal"
)

// ─────────────────────────────────────────────────────────
// CreatePlan 参数验证
// ─────────────────────────────────────────────────────────

func TestService_CreatePlan_Validation(t *testing.T) {
	ctx := context.Background()
	st := snap.NewMemStore()
	svc := newTestSvc(st)
	type tc struct {
		name	string
		req	ledseal.CreatePlanReq
		wantErr	bool
		errIs	error
	}
	cases := []tc{
		{
			name:		"valid_user_shard",
			req:		ledseal.CreatePlanReq{Name: "plan-ok", PlanScope: ledseal.ScopeUserShard, ShardKey: "u1", IntervalSeconds: 3600},
			wantErr:	false,
		},
		{
			name:		"valid_full_day",
			req:		ledseal.CreatePlanReq{Name: "plan-full", PlanScope: ledseal.ScopeFullDay, IntervalSeconds: 86400},
			wantErr:	false,
		},
		{
			name:		"empty_name",
			req:		ledseal.CreatePlanReq{Name: "", PlanScope: ledseal.ScopeUserShard},
			wantErr:	true, errIs: ledseal.ErrParam,
		},
		{
			name:		"whitespace_name",
			req:		ledseal.CreatePlanReq{Name: "   ", PlanScope: ledseal.ScopeUserShard},
			wantErr:	true, errIs: ledseal.ErrParam,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreatePlan(ctx, tc.req)
			if tc.wantErr {
				if err == nil {
					t.Errorf("want error, got nil")
				}
				if tc.errIs != nil && !strings.Contains(err.Error(), tc.errIs.Error()) {
					t.Errorf("want error containing %q, got %q", tc.errIs, err)
				}
			} else {
				if err != nil {
					t.Errorf("want nil, got %v", err)
				}
			}
		})
	}
}

// TestService_CreatePlan_DuplicateName 验证计划名称唯一性。

func TestService_CreatePlan_DuplicateName(t *testing.T) {
	ctx := context.Background()
	st := snap.NewMemStore()
	svc := newTestSvc(st)
	req := ledseal.CreatePlanReq{Name: "dup-plan", PlanScope: ledseal.ScopeUserShard, ShardKey: "u1", IntervalSeconds: 3600}
	if _, err := svc.CreatePlan(ctx, req); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := svc.CreatePlan(ctx, req)
	if err != ledseal.ErrPlanDuplicate {
		t.Errorf("want ErrPlanDuplicate, got %v", err)
	}
}

// TestService_PlanActivatePause_FSM 完整 Plan FSM 路径。

func TestService_PlanActivatePause_FSM(t *testing.T) {
	ctx := context.Background()
	st := snap.NewMemStore()
	svc := newTestSvc(st)
	plan, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{
		Name:	"fsm-plan", PlanScope: ledseal.ScopeUserShard, ShardKey: "u1", IntervalSeconds: 3600,
	})
	if plan.State != ledseal.PlanActive {
		t.Errorf("new plan: want active, got %s", plan.State)
	}
	// 不能 activate 已 active 的计划
	if _, err := svc.ActivatePlan(ctx, plan.ID); err == nil {
		t.Error("want error activating active plan")
	}
	// pause
	plan, err := svc.PausePlan(ctx, plan.ID)
	if err != nil || plan.State != ledseal.PlanPaused {
		t.Errorf("pause: want paused, got %s err=%v", plan.State, err)
	}
	// 不能 pause 已 paused 的计划
	if _, err := svc.PausePlan(ctx, plan.ID); err == nil {
		t.Error("want error pausing paused plan")
	}
	// activate
	plan, err = svc.ActivatePlan(ctx, plan.ID)
	if err != nil || plan.State != ledseal.PlanActive {
		t.Errorf("activate: want active, got %s err=%v", plan.State, err)
	}
}

// TestService_RegisterSlice_AutoSeq 验证 gen_seq 单调递增。
func TestService_RegisterSlice_AutoSeq(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2022, 6, 14, 10, 0, 0, 0, time.UTC)
	st := snap.NewMemStore()
	svc := newTestSvc(st)
	svc.WithClock(func() time.Time { return t0 })
	plan, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{
		Name:	"seq-plan", PlanScope: ledseal.ScopeUserShard, ShardKey: "u1", IntervalSeconds: 3600,
	})
	// 连续注册 5 个切片，验证 gen_seq 从 1 开始递增
	for i := 0; i < 5; i++ {
		snp := buildTestSnap(t, st, "u1", [][]string{{"MAIN", "USDT", "100", "0"}}, t0.Add(time.Duration(i)*time.Hour))
		sl, err := svc.RegisterSlice(ctx, plan.ID, snp.ID, "u1", "",
			snp.Checksum, snp.Count, 0, "", t0.Add(time.Duration(i)*time.Hour))
		if err != nil {
			t.Fatalf("register slice %d: %v", i, err)
		}
		expectedSeq := int64(i + 1)
		if sl.GenSeq != expectedSeq {
			t.Errorf("slice %d: want gen_seq=%d, got %d", i, expectedSeq, sl.GenSeq)
		}
	}
	// LatestSlice 应返回 gen_seq=5
	latest, _ := svc.LatestSlice(ctx, plan.ID)
	if latest.GenSeq != 5 {
		t.Errorf("LatestSlice: want 5, got %d", latest.GenSeq)
	}
}

// TestService_OpenBatch_DefaultState 验证 OpenBatch 初始状态为 running。
func TestService_OpenBatch_DefaultState(t *testing.T) {
	ctx := context.Background()
	st := snap.NewMemStore()
	svc := newTestSvc(st)
	plan, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{Name: "ob-plan", PlanScope: ledseal.ScopeUserShard, ShardKey: "u1", IntervalSeconds: 3600})
	batch, err := svc.OpenBatch(ctx, plan.ID, "sl1", "sl2", 1, 2)
	if err != nil {
		t.Fatalf("OpenBatch: %v", err)
	}
	if batch.State != ledseal.BatchRunning {
		t.Errorf("want running, got %s", batch.State)
	}
	if batch.FromGenSeq != 1 || batch.ToGenSeq != 2 {
		t.Errorf("want from=1 to=2, got from=%d to=%d", batch.FromGenSeq, batch.ToGenSeq)
	}
}

// TestService_ConfirmDrift_Validation 验证 ConfirmDrift 参数校验。

func TestService_ConfirmDrift_Validation(t *testing.T) {
	ctx := context.Background()
	st := snap.NewMemStore()
	svc := newTestSvc(st)
	// empty drift_id
	if err := svc.ConfirmDrift(ctx, ledseal.ConfirmDriftReq{DriftID: "", Actor: "ops"}); err == nil {
		t.Error("want error for empty drift_id")
	}
	// empty actor
	if err := svc.ConfirmDrift(ctx, ledseal.ConfirmDriftReq{DriftID: "d1", Actor: ""}); err == nil {
		t.Error("want error for empty actor")
	}
	// non-existent drift
	if err := svc.ConfirmDrift(ctx, ledseal.ConfirmDriftReq{DriftID: "nonexistent", Actor: "ops"}); err == nil {
		t.Error("want error for non-existent drift")
	}
}

// TestService_ExportRoot 完整的导出路径验证。
func TestService_ExportRoot(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2022, 6, 14, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	st := snap.NewMemStore()
	svc := newTestSvc(st)
	svc.WithClock(func() time.Time { return t1 })
	plan, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{Name: "export-test", PlanScope: ledseal.ScopeUserShard, ShardKey: "ue", IntervalSeconds: 3600})
	snp0 := buildTestSnap(t, st, "ue", [][]string{{"MAIN", "USDT", "100", "0"}}, t0)
	snp1 := buildTestSnap(t, st, "ue", [][]string{{"MAIN", "USDT", "100", "0"}}, t1)
	sl0, _ := svc.RegisterSlice(ctx, plan.ID, snp0.ID, "ue", "", snp0.Checksum, snp0.Count, 0, "", t0)
	sl1, _ := svc.RegisterSlice(ctx, plan.ID, snp1.ID, "ue", "", snp1.Checksum, snp1.Count, 0, "", t1)
	batch, _ := svc.OpenBatch(ctx, plan.ID, sl0.ID, sl1.ID, sl0.GenSeq, sl1.GenSeq)
	_, root, _ := svc.RunAndSealBatch(ctx, batch.ID)
	// 导出前
	exported, err := svc.ExportRoot(ctx, ledseal.ExportRootReq{BatchID: batch.ID, ExportedBy: "recon-bot"})
	if err != nil {
		t.Fatalf("ExportRoot: %v", err)
	}
	if exported.ExportedBy != "recon-bot" {
		t.Errorf("want recon-bot, got %s", exported.ExportedBy)
	}
	if exported.RootHash != root.RootHash {
		t.Error("root hash changed after export")
	}
	// ExportRoot 不存在的 batch
	_, err = svc.ExportRoot(ctx, ledseal.ExportRootReq{BatchID: "nonexistent", ExportedBy: "bot"})
	if err == nil {
		t.Error("want error for nonexistent batch")
	}
}

// TestService_NilSvc 验证 nil service 方法返回 ErrUnavailable。
func TestService_NilSvc(t *testing.T) {
	var svc *ledseal.Service
	ctx := context.Background()
	if _, err := svc.CreatePlan(ctx, ledseal.CreatePlanReq{}); err != ledseal.ErrUnavailable {
		t.Errorf("CreatePlan nil: want ErrUnavailable, got %v", err)
	}
	if _, err := svc.ListPlans(ctx, 10); err != ledseal.ErrUnavailable {
		t.Errorf("ListPlans nil: want ErrUnavailable, got %v", err)
	}
	if _, err := svc.RegisterSlice(ctx, "", "", "", "", "", 0, 0, "", time.Now()); err != ledseal.ErrUnavailable {
		t.Errorf("RegisterSlice nil: want ErrUnavailable, got %v", err)
	}
	if _, err := svc.OpenBatch(ctx, "", "", "", 0, 0); err != ledseal.ErrUnavailable {
		t.Errorf("OpenBatch nil: want ErrUnavailable, got %v", err)
	}
	if _, _, err := svc.RunAndSealBatch(ctx, ""); err != ledseal.ErrUnavailable {
		t.Errorf("RunAndSealBatch nil: want ErrUnavailable, got %v", err)
	}
	if _, err := svc.ListDriftRows(ctx, "", 10); err != ledseal.ErrUnavailable {
		t.Errorf("ListDriftRows nil: want ErrUnavailable, got %v", err)
	}
	if err := svc.ConfirmDrift(ctx, ledseal.ConfirmDriftReq{}); err != ledseal.ErrUnavailable {
		t.Errorf("ConfirmDrift nil: want ErrUnavailable, got %v", err)
	}
	if err := svc.VerifyRoot(ctx, ""); err != ledseal.ErrUnavailable {
		t.Errorf("VerifyRoot nil: want ErrUnavailable, got %v", err)
	}
}

// TestService_MultiPlan_Isolation 验证多计划数据隔离。
func TestService_MultiPlan_Isolation(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2022,
		6, 14, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	st := snap.NewMemStore()
	svc := newTestSvc(st)
	svc.WithClock(func() time.Time { return t1 })
	// 创建两个计划
	p1, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{Name: "plan-A", PlanScope: ledseal.ScopeUserShard, ShardKey: "uA", IntervalSeconds: 3600})
	p2, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{Name: "plan-B", PlanScope: ledseal.ScopeUserShard, ShardKey: "uB", IntervalSeconds: 3600})
	// 各自注册切片
	snpA0 := buildTestSnap(t, st, "uA", [][]string{{"MAIN", "USDT", "100", "0"}}, t0)
	snpA1 := buildTestSnap(t, st, "uA", [][]string{{"MAIN", "USDT", "100", "0"}}, t1)
	snpB0 := buildTestSnap(t, st, "uB", [][]string{{"MAIN", "USDT", "200", "0"}}, t0)
	snpB1 := buildTestSnap(t, st, "uB", [][]string{{"MAIN", "USDT", "50200", "0"}}, t1)	// critical drift
	slA0, _ := svc.RegisterSlice(ctx, p1.ID, snpA0.ID, "uA", "", snpA0.Checksum, snpA0.Count, 0, "", t0)
	slA1, _ := svc.RegisterSlice(ctx, p1.ID, snpA1.ID, "uA", "", snpA1.Checksum, snpA1.Count, 0, "", t1)
	slB0, _ := svc.RegisterSlice(ctx, p2.ID, snpB0.ID, "uB", "", snpB0.Checksum, snpB0.Count, 0, "", t0)
	slB1, _ := svc.RegisterSlice(ctx, p2.ID, snpB1.ID, "uB", "", snpB1.Checksum, snpB1.Count, 0, "", t1)
	batchA, _ := svc.OpenBatch(ctx, p1.ID, slA0.ID, slA1.ID, slA0.GenSeq, slA1.GenSeq)
	batchB, _ := svc.OpenBatch(ctx, p2.ID, slB0.ID, slB1.ID, slB0.GenSeq, slB1.GenSeq)
	batchA, _, err := svc.RunAndSealBatch(ctx, batchA.ID)
	if err != nil {
		t.Fatalf("batch A: %v", err)
	}
	batchB, _, err = svc.RunAndSealBatch(ctx, batchB.ID)
	if err != nil {
		t.Fatalf("batch B: %v", err)
	}
	// Plan A 无漂移
	if batchA.DriftCount != 0 {
		t.Errorf("plan A: want 0 drifts, got %d", batchA.DriftCount)
	}
	// Plan B 有 critical 漂移
	if batchB.DriftCount == 0 {
		t.Error("plan B: want drifts > 0")
	}
	if batchB.MaxSeverity != ledseal.SeverityCritical {
		t.Errorf("plan B: want critical, got %s", batchB.MaxSeverity)
	}
	// 切片列表隔离
	slicesA, _ := svc.ListSlices(ctx, p1.ID, 10)
	slicesB, _ := svc.ListSlices(ctx, p2.ID, 10)
	if len(slicesA) != 2 {
		t.Errorf("plan A slices: want 2, got %d", len(slicesA))
	}
	if len(slicesB) != 2 {
		t.Errorf("plan B slices: want 2, got %d", len(slicesB))
	}
	// 批次列表隔离
	batchesA, _ := svc.ListBatches(ctx, p1.ID, "", 10)
	batchesB, _ := svc.ListBatches(ctx, p2.ID, "", 10)
	if len(batchesA) != 1 {
		t.Errorf("plan A batches: want 1, got %d", len(batchesA))
	}
	if len(batchesB) != 1 {
		t.Errorf("plan B batches: want 1, got %d", len(batchesB))
	}
}

// TestService_GetPlanByID_NotFound 验证不存在的计划返回 ErrPlanNotFound。

func TestService_GetPlanByID_NotFound(t *testing.T) {
	ctx := context.Background()
	st := snap.NewMemStore()
	svc := newTestSvc(st)
	_, err := svc.GetPlan(ctx, "nonexistent")
	if err != ledseal.ErrPlanNotFound {
		t.Errorf("want ErrPlanNotFound, got %v", err)
	}
}

// TestService_DefaultIntervalAndRetain 验证默认值填充。

func TestService_DefaultIntervalAndRetain(t *testing.T) {
	ctx := context.Background()
	st := snap.NewMemStore()
	svc := newTestSvc(st)
	plan, _ := svc.CreatePlan(ctx, ledseal.CreatePlanReq{Name: "default-test", PlanScope: ledseal.ScopeFullDay})
	if plan.IntervalSeconds != 3600 {
		t.Errorf("default interval: want 3600, got %d", plan.IntervalSeconds)
	}
	if plan.RetainCount != 48 {
		t.Errorf("default retain: want 48, got %d", plan.RetainCount)
	}
}

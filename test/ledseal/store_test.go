package ledseal_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/ledseal"
)

var testNow = time.Date(2022, 6, 14, 10, 0, 0, 0, time.UTC)

// ─────────────────────────────────────────────────────────
// SnapPlan CRUD
// ─────────────────────────────────────────────────────────

func TestMemStore_PlanCRUD(t *testing.T) {
	ctx := context.Background()
	st := ledseal.NewMemStore()
	// GetPlan 不存在应返回 ErrPlanNotFound
	if _, err := st.GetPlan(ctx, "nonexistent"); err != ledseal.ErrPlanNotFound {
		t.Errorf("want ErrPlanNotFound, got %v", err)
	}
	// GetPlanByName 不存在应返回 ErrPlanNotFound
	if _, err := st.GetPlanByName(ctx, "nonexistent"); err != ledseal.ErrPlanNotFound {
		t.Errorf("want ErrPlanNotFound by name, got %v", err)
	}
	// PutPlan 写入
	plan := ledseal.SnapPlan{
		ID:	"p1", Name: "plan-one", PlanScope: ledseal.ScopeUserShard,
		ShardKey:	"u1", IntervalSeconds: 3600, RetainCount: 48,
		State:	ledseal.PlanActive, CreatedAt: testNow, UpdatedAt: testNow,
	}
	if err := st.PutPlan(ctx, plan); err != nil {
		t.Fatalf("PutPlan: %v", err)
	}
	// GetPlan 应返回
	got, err := st.GetPlan(ctx, "p1")
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if got.Name != "plan-one" {
		t.Errorf("want name=plan-one, got %s", got.Name)
	}
	// GetPlanByName
	byName, err := st.GetPlanByName(ctx, "plan-one")
	if err != nil {
		t.Fatalf("GetPlanByName: %v", err)
	}
	if byName.ID != "p1" {
		t.Errorf("want id=p1, got %s", byName.ID)
	}
	// ListPlans
	plans, err := st.ListPlans(ctx, 10)
	if err != nil || len(plans) != 1 {
		t.Errorf("ListPlans: want 1, got %d err=%v", len(plans), err)
	}
	// ListActivePlans（to=now+1s，next_run_at 为 nil，应返回）
	after := testNow.Add(time.Hour)
	active, _ := st.ListActivePlans(ctx, after)
	if len(active) != 1 {
		t.Errorf("ListActivePlans: want 1, got %d", len(active))
	}
	// 暂停计划后不应出现在 active 列表
	plan.State = ledseal.PlanPaused
	_ = st.PutPlan(ctx, plan)
	active2, _ := st.ListActivePlans(ctx, after)
	if len(active2) != 0 {
		t.Errorf("ListActivePlans after pause: want 0, got %d", len(active2))
	}
	// next_run_at 超出 before 时间不应返回
	plan.State = ledseal.PlanActive
	futureRun := after.Add(2 * time.Hour)
	plan.NextRunAt = &futureRun
	_ = st.PutPlan(ctx, plan)
	active3, _ := st.ListActivePlans(ctx, after)
	if len(active3) != 0 {
		t.Errorf("ListActivePlans with future next_run: want 0, got %d", len(active3))
	}
}

// ─────────────────────────────────────────────────────────
// SnapSlice CRUD
// ─────────────────────────────────────────────────────────

func TestMemStore_SliceCRUD(t *testing.T) {
	ctx := context.Background()
	st := ledseal.NewMemStore()
	// GetSlice 不存在
	if _, err := st.GetSlice(ctx, "nonexistent"); err != ledseal.ErrSliceNotFound {
		t.Errorf("want ErrSliceNotFound, got %v", err)
	}
	// LatestSlice 无切片时
	if _, err := st.LatestSlice(ctx, "p1"); err != ledseal.ErrSliceNotFound {
		t.Errorf("want ErrSliceNotFound for empty plan, got %v", err)
	}
	slices := []ledseal.SnapSlice{
		{ID: "s1", PlanID: "p1", SnapID: "snap1", GenSeq: 1, State: "ready", TakenAt: testNow, CreatedAt: testNow},
		{ID: "s2", PlanID: "p1", SnapID: "snap2", GenSeq: 2, State: "ready", TakenAt: testNow.Add(time.Hour), CreatedAt: testNow.Add(time.Hour)},
		{ID: "s3", PlanID: "p1", SnapID: "snap3", GenSeq: 3, State: "ready", TakenAt: testNow.Add(2 * time.Hour), CreatedAt: testNow.Add(2 * time.Hour)},
		{ID: "s4", PlanID: "p2", SnapID: "snap4", GenSeq: 1, State: "ready", TakenAt: testNow, CreatedAt: testNow},
	}
	for _, s := range slices {
		if err := st.PutSlice(ctx, s); err != nil {
			t.Fatalf("PutSlice %s: %v", s.ID, err)
		}
	}
	// GetSlice
	got, _ := st.GetSlice(ctx, "s2")
	if got.GenSeq != 2 {
		t.Errorf("want GenSeq=2, got %d", got.GenSeq)
	}
	// LatestSlice（最大 gen_seq）
	latest, _ := st.LatestSlice(ctx, "p1")
	if latest.GenSeq != 3 {
		t.Errorf("LatestSlice: want GenSeq=3, got %d", latest.GenSeq)
	}
	// ListSlices 按 plan_id 过滤，按 gen_seq 升序
	list, _ := st.ListSlices(ctx, "p1", 10)
	if len(list) != 3 {
		t.Errorf("ListSlices p1: want 3, got %d", len(list))
	}
	for i := 1; i < len(list); i++ {
		if list[i].GenSeq <= list[i-1].GenSeq {
			t.Errorf("ListSlices not sorted by gen_seq: [%d] %d <= [%d] %d",
				i, list[i].GenSeq, i-1, list[i-1].GenSeq)
		}
	}
	// ListSlices 另一个 plan
	list2, _ := st.ListSlices(ctx, "p2", 10)
	if len(list2) != 1 {
		t.Errorf("ListSlices p2: want 1, got %d", len(list2))
	}
}

// ─────────────────────────────────────────────────────────
// DiffBatch CRUD
// ─────────────────────────────────────────────────────────

func TestMemStore_BatchCRUD(t *testing.T) {
	ctx := context.Background()
	st := ledseal.NewMemStore()
	if _, err := st.GetBatch(ctx, "nonexistent"); err != ledseal.ErrBatchNotFound {
		t.Errorf("want ErrBatchNotFound, got %v", err)
	}
	batches := []ledseal.DiffBatch{
		{ID: "b1", PlanID: "p1", State: ledseal.BatchRunning, FromGenSeq: 1, ToGenSeq: 2, CreatedAt: testNow, UpdatedAt: testNow},
		{ID: "b2", PlanID: "p1", State: ledseal.BatchSealed, FromGenSeq: 3, ToGenSeq: 4, CreatedAt: testNow.Add(time.Hour), UpdatedAt: testNow.Add(time.Hour)},
		{ID: "b3", PlanID: "p1", State: ledseal.BatchFailed, FromGenSeq: 5, ToGenSeq: 7, ErrMsg: "seq gap", CreatedAt: testNow.Add(2 * time.Hour), UpdatedAt: testNow.Add(2 * time.Hour)},
		{ID: "b4", PlanID: "p2", State: ledseal.BatchSealed, FromGenSeq: 1, ToGenSeq: 2, CreatedAt: testNow, UpdatedAt: testNow},
	}
	for _, b := range batches {
		_ = st.PutBatch(ctx, b)
	}
	// GetBatch
	got, _ := st.GetBatch(ctx, "b3")
	if got.ErrMsg != "seq gap" {
		t.Errorf("want ErrMsg='seq gap', got %q", got.ErrMsg)
	}
	// ListBatches 按 plan_id 和 state 过滤
	all, _ := st.ListBatches(ctx, "p1", "", 10)
	if len(all) != 3 {
		t.Errorf("ListBatches p1 all: want 3, got %d", len(all))
	}
	sealed, _ := st.ListBatches(ctx, "p1", ledseal.BatchSealed, 10)
	if len(sealed) != 1 {
		t.Errorf("ListBatches p1 sealed: want 1, got %d", len(sealed))
	}
	failed, _ := st.ListBatches(ctx, "p1", ledseal.BatchFailed, 10)
	if len(failed) != 1 {
		t.Errorf("ListBatches p1 failed: want 1, got %d", len(failed))
	}
	running, _ := st.ListBatches(ctx, "p1", ledseal.BatchRunning, 10)
	if len(running) != 1 {
		t.Errorf("ListBatches p1 running: want 1, got %d", len(running))
	}
	// 更新批次状态
	batches[0].State = ledseal.BatchSealed
	_ = st.PutBatch(ctx, batches[0])
	updated, _ := st.GetBatch(ctx, "b1")
	if updated.State != ledseal.BatchSealed {
		t.Errorf("after update: want sealed, got %s", updated.State)
	}
}

// ─────────────────────────────────────────────────────────
// DriftRow CRUD
// ─────────────────────────────────────────────────────────

func TestMemStore_DriftRowCRUD(t *testing.T) {
	ctx := context.Background()
	st := ledseal.NewMemStore()
	if _, err := st.GetDriftRow(ctx, "nonexistent"); err != ledseal.ErrDriftNotFound {
		t.Errorf("want ErrDriftNotFound, got %v", err)
	}
	rows := []ledseal.DriftRow{
		{ID: "d1", BatchID: "b1", PlanID: "p1", UserID: "u1", Book: "MAIN", Asset: "USDT",
			AvailBefore:	"100", AvailAfter: "200", Severity: ledseal.SeverityMajor, Kind: ledseal.KindMutation, CreatedAt: testNow},
		{ID: "d2", BatchID: "b1", PlanID: "p1", UserID: "u1", Book: "TRADE", Asset: "BTC",
			AvailBefore:	"0", AvailAfter: "50100", Severity: ledseal.SeverityCritical, Kind: ledseal.KindAppearance, BlockWriteHint: true, CreatedAt: testNow.Add(time.Minute)},
		{ID: "d3", BatchID: "b2", PlanID: "p1", UserID: "u2", Book: "MAIN", Asset: "ETH",
			AvailBefore:	"5", AvailAfter: "0", Severity: ledseal.SeverityCritical, Kind: ledseal.KindDisappearance, CreatedAt: testNow.Add(2 * time.Minute)},
	}
	for _, r := range rows {
		_ = st.PutDriftRow(ctx, r)
	}
	// GetDriftRow
	got, _ := st.GetDriftRow(ctx, "d2")
	if !got.BlockWriteHint {
		t.Error("want BlockWriteHint=true")
	}
	// ListDriftRows 按 batch_id 过滤
	b1rows, _ := st.ListDriftRows(ctx, "b1", 10)
	if len(b1rows) != 2 {
		t.Errorf("ListDriftRows b1: want 2, got %d", len(b1rows))
	}
	b2rows, _ := st.ListDriftRows(ctx, "b2", 10)
	if len(b2rows) != 1 {
		t.Errorf("ListDriftRows b2: want 1, got %d", len(b2rows))
	}
	// ConfirmDriftRow
	at := testNow.Add(time.Hour)
	if err := st.ConfirmDriftRow(ctx, "d1", "ops-alice", "已核实", at); err != nil {
		t.Fatalf("ConfirmDriftRow: %v", err)
	}
	confirmed, _ := st.GetDriftRow(ctx, "d1")
	if confirmed.ConfirmedBy != "ops-alice" {
		t.Errorf("want ConfirmedBy=ops-alice, got %s", confirmed.ConfirmedBy)
	}
	if confirmed.ConfirmedAt == nil {
		t.Error("want non-nil ConfirmedAt")
	}
	// ConfirmDriftRow 不存在的行
	if err := st.ConfirmDriftRow(ctx, "nonexistent", "actor", "", at); err != ledseal.ErrDriftNotFound {
		t.Errorf("want ErrDriftNotFound, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────
// SealRoot CRUD
// ─────────────────────────────────────────────────────────

func TestMemStore_SealRootCRUD(t *testing.T) {
	ctx := context.Background()
	st := ledseal.NewMemStore()
	if _, err := st.GetSealRoot(ctx, "nonexistent"); err != ledseal.ErrSealRootNotFound {
		t.Errorf("want ErrSealRootNotFound, got %v", err)
	}
	if _, err := st.GetSealRootByBatch(ctx, "nonexistent"); err != ledseal.ErrSealRootNotFound {
		t.Errorf("want ErrSealRootNotFound by batch, got %v", err)
	}
	root := ledseal.SealRoot{
		ID:	"r1", BatchID: "b1", PlanID: "p1",
		FromGenSeq:	1, ToGenSeq: 2,
		RootHash:	"abc123hash", DriftCount: 3,
		PositionsHash:	"posHash",
		MetaJSON:	`{"plan_id":"p1"}`,
		CreatedAt:	testNow,
	}
	_ = st.PutSealRoot(ctx, root)
	// GetSealRoot
	got, _ := st.GetSealRoot(ctx, "r1")
	if got.RootHash != "abc123hash" {
		t.Errorf("want hash=abc123hash, got %s", got.RootHash)
	}
	// GetSealRootByBatch
	byBatch, _ := st.GetSealRootByBatch(ctx, "b1")
	if byBatch.ID != "r1" {
		t.Errorf("want id=r1, got %s", byBatch.ID)
	}
	// MarkExported
	at := testNow.Add(time.Hour)
	if err := st.MarkExported(ctx, "r1", "recon-bot", at); err != nil {
		t.Fatalf("MarkExported: %v", err)
	}
	exported, _ := st.GetSealRoot(ctx, "r1")
	if exported.ExportedBy != "recon-bot" {
		t.Errorf("want ExportedBy=recon-bot, got %s", exported.ExportedBy)
	}
	if exported.ExportedAt == nil {
		t.Error("want non-nil ExportedAt")
	}
	// MarkExported 不存在
	if err := st.MarkExported(ctx, "nonexistent", "bot", at); err != ledseal.ErrSealRootNotFound {
		t.Errorf("want ErrSealRootNotFound, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────
// ListPlans limit 验证
// ─────────────────────────────────────────────────────────

func TestMemStore_ListPlans_LimitRespected(t *testing.T) {
	ctx := context.Background()
	st := ledseal.NewMemStore()
	for i := 0; i < 20; i++ {
		p := ledseal.SnapPlan{
			ID:	"p" + string(rune('a'+i)), Name: "plan-" + string(rune('a'+i)),
			State:	ledseal.PlanActive, CreatedAt: testNow.Add(time.Duration(i) * time.Minute),
		}
		_ = st.PutPlan(ctx, p)
	}
	// limit=5 应返回 5 个
	list, _ := st.ListPlans(ctx, 5)
	if len(list) != 5 {
		t.Errorf("want 5 plans with limit=5, got %d", len(list))
	}
	// limit=0 默认 50，全部返回
	all, _ := st.ListPlans(ctx, 0)
	if len(all) != 20 {
		t.Errorf("want 20 plans with limit=0, got %d", len(all))
	}
}

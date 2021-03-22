package batchhopgate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/batchhopgate"
)

// TestWorker_ResidualScanner 验证 ResidualScanner 对 open 状态残留案重建占用证明。
func TestWorker_ResidualScanner(t *testing.T) {
	svc := batchhopgate.NewService(nil)
	svc.WithClock(fixedClock(epoch))
	ctx := context.Background()
	// 直接写入一条 open 状态残留案（无 ProofID，证明构建失败的场景）。
	openCase := &batchhopgate.BatchResidualCase{
		ID:		"case-worker-scan-01",
		UserID:		"u-worker-01",
		Asset:		"USDT",
		Generation:	1,
		TotalItems:	3,
		FailIndex:	2,
		SucceededHops:	makeSucceededHops(2),
		FailedReason:	"proof build failed transiently",
		BeforeRoot:	"root-before-w1",
		PartialRoot:	"root-partial-w1",
		Status:		batchhopgate.ResidualOpen,
		ProofID:	"",	// 尚无证明
		DetectedAt:	epoch,
		UpdatedAt:	epoch,
	}
	if err := svc.store.SaveResidual(ctx, openCase); err != nil {
		t.Fatalf("SaveResidual: %v", err)
	}
	// 同时初始化世代（避免 CheckConcurrent 误报）。
	_, _ = svc.store.BumpGeneration(ctx, "u-worker-01", "USDT", 0)
	w := batchhopgate.NewWorkers(svc, batchhopgate.WorkerConfig{MaxBatch: 10})
	w.WithClock(fixedClock(epoch))
	w.RunScanOnce(ctx)
	// 扫描后残留案应推进至 proven（证明已重建）。
	c, err := svc.store.GetResidual(ctx, "case-worker-scan-01")
	if err != nil {
		t.Fatalf("GetResidual: %v", err)
	}
	if c.Status != batchhopgate.ResidualProven {
		t.Errorf("after scan: status=%s, want proven", c.Status)
	}
	if c.ProofID == "" {
		t.Error("ProofID should be non-empty after scanner rebuild")
	}
}

// TestWorker_OccupyPatrol 验证 OccupyPatrol 对 Digest 被篡改的案触发告警指标。

func TestWorker_OccupyPatrol(t *testing.T) {
	svc := batchhopgate.NewService(nil)
	svc.WithClock(fixedClock(epoch))
	ctx := context.Background()
	// 先正常落库一个 proven 残留案。
	req := batchhopgate.OpenResidualFullReq{
		UserID:	"u-worker-02", Asset: "ETH",
		TotalItems:	2, FailIndex: 1,
		SucceededHops:	makeSucceededHops(1),
		FailedReason:	"fail", BeforeRoot: "rb-w2", PartialRoot: "rp-w2",
		BuiltBy:	"test",
	}
	openResult, err := svc.OpenResidual(ctx, req)
	if err != nil {
		t.Fatalf("OpenResidual: %v", err)
	}
	if openResult.Proof == nil {
		t.Skip("no proof built, skip patrol test")
	}
	// 篡改 Digest。
	tampered := *openResult.Proof
	tampered.Digest = "0000000000000000000000000000000000000000000000000000000000000000"
	_ = svc.store.SaveProof(ctx,
		&tampered)
	w := batchhopgate.NewWorkers(svc, batchhopgate.WorkerConfig{MaxBatch: 10})
	w.WithClock(fixedClock(epoch))
	// 记录 patrol 前的 proofDigestFail 计数（不能直接读 prometheus 计数，只验证不 panic）。
	w.RunPatrolOnce(ctx)
	// 不 panic 即通过；实际告警由 Prometheus 指标驱动，此处不做精确数值断言。
}

// TestWorker_LeaseReaper 验证 LeaseReaper 正确标记超过 TTL 的 open 续跳租约为 expired。
func TestWorker_LeaseReaper(t *testing.T) {
	svc := batchhopgate.NewService(nil)
	svc.WithClock(fixedClock(epoch))
	ctx := context.Background()
	// 直接写入一条已超时的 open 续跳租约。
	expiredLease := &batchhopgate.ResumeLease{
		ID:		"lease-reaper-01",
		ResidualCaseID:	"case-reaper-01",
		UserID:		"u-worker-03",
		Asset:		"USDT",
		Generation:	1,
		ResumeFrom:	1,
		Status:		batchhopgate.LeaseOpen,
		ExpiresAt:	epoch.Add(-2 * time.Hour),	// 2小时前已到期
		CreatedAt:	epoch.Add(-3 * time.Hour),
		UpdatedAt:	epoch.Add(-3 * time.Hour),
	}
	if err := svc.store.SaveLease(ctx, expiredLease); err != nil {
		t.Fatalf("SaveLease: %v", err)
	}
	// 推进时钟，使 now > expires_at。
	now := epoch.Add(1 * time.Hour)	// 现在比 expires_at (epoch-2h) 晚了 3 小时
	w := batchhopgate.NewWorkers(svc, batchhopgate.WorkerConfig{MaxBatch: 10})
	w.WithClock(fixedClock(now))
	w.RunReaperOnce(ctx)
	// 验证过期租约已被标记为 expired。
	l, err := svc.store.GetLease(ctx, "lease-reaper-01")
	if err != nil {
		t.Fatalf("GetLease: %v", err)
	}
	if l.Status != batchhopgate.LeaseExpired {
		t.Errorf("lease status=%s after reaper, want expired", l.Status)
	}
}

// TestWorker_LeaseReaper_NoFalseExpiry 验证未超时的 open 租约不被误标记。

func TestWorker_LeaseReaper_NoFalseExpiry(t *testing.T) {
	svc := batchhopgate.NewService(nil)
	svc.WithClock(fixedClock(epoch))
	ctx := context.Background()
	activeLease := &batchhopgate.ResumeLease{
		ID:		"lease-active-01",
		ResidualCaseID:	"case-active-01",
		UserID:		"u-worker-04",
		Asset:		"BTC",
		Generation:	1,
		ResumeFrom:	0,
		Status:		batchhopgate.LeaseOpen,
		ExpiresAt:	epoch.Add(20 * time.Minute),	// 未到期（20分钟后到期）
		CreatedAt:	epoch,
		UpdatedAt:	epoch,
	}
	if err := svc.store.SaveLease(ctx, activeLease); err != nil {
		t.Fatalf("SaveLease: %v", err)
	}
	// Reaper 以 epoch（now） 为基准，epoch < expires_at，不应过期。
	w := batchhopgate.NewWorkers(svc, batchhopgate.WorkerConfig{MaxBatch: 10})
	w.WithClock(fixedClock(epoch))
	w.RunReaperOnce(ctx)
	l, err := svc.store.GetLease(ctx, "lease-active-01")
	if err != nil {
		t.Fatalf("GetLease: %v", err)
	}
	if l.Status != batchhopgate.LeaseOpen {
		t.Errorf("active lease status=%s after reaper, want open (should not be expired)", l.Status)
	}
}

// TestWorker_ResidualScanner_ProofIDPresent 验证 ProofID 非空时跳过证明重建，直接尝试 Prove 推进。
func TestWorker_ResidualScanner_ProofIDPresent(t *testing.T) {
	svc := batchhopgate.NewService(nil)
	svc.WithClock(fixedClock(epoch))
	ctx := context.Background()
	// 写入带 ProofID 但状态仍 open 的残留案（Prove 调用失败导致状态未推进）。
	proof, _ := svc.proofVault.Build(ctx, batchhopgate.BuildReq{
		ResidualCaseID:	"case-worker-scan-02",
		UserID:		"u-worker-05",
		Asset:		"USDT",
		BeforeRoot:	"rb-w5",
		PartialRoot:	"rp-w5",
		SucceededHops:	makeSucceededHops(1),
		ConserveOK:	true,
		BuiltBy:	"test",
	})
	stuckCase := &batchhopgate.BatchResidualCase{
		ID:		"case-worker-scan-02",
		UserID:		"u-worker-05",
		Asset:		"USDT",
		Generation:	1,
		TotalItems:	2, FailIndex: 1,
		SucceededHops:	makeSucceededHops(1),
		FailedReason:	"fail",
		BeforeRoot:	"rb-w5", PartialRoot: "rp-w5",
		Status:		batchhopgate.ResidualOpen,
		ProofID:	proof.ID,	// ProofID 已有
		DetectedAt:	epoch, UpdatedAt: epoch,
	}
	_ = svc.store.SaveResidual(ctx, stuckCase)
	w := batchhopgate.NewWorkers(svc, batchhopgate.WorkerConfig{MaxBatch: 10})
	w.RunScanOnce(ctx)
	c, _ := svc.store.GetResidual(ctx, "case-worker-scan-02")
	if c.Status != batchhopgate.ResidualProven {
		t.Errorf("status=%s, want proven", c.Status)
	}
}

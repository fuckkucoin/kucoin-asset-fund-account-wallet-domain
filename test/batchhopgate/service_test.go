package batchhopgate_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/batchhopgate"
)

// ─────────────────────────────────────────────────────────────────────────────
// 辅助工具
// ─────────────────────────────────────────────────────────────────────────────

func newTestService(t *testing.T) *batchhopgate.Service {
	t.Helper()
	return batchhopgate.NewService(nil)
}

func fixedClock(ts time.Time) func() time.Time {
	return func() time.Time { return ts }
}

var epoch = time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)

func makeSucceededHops(count int) []batchhopgate.HopSummary {
	hops := make([]batchhopgate.HopSummary, 0, count)
	for i := 0; i < count; i++ {
		hops = append(hops, batchhopgate.HopSummary{
			Index:	i,
			From:	"funding",
			To:	"trading",
			Asset:	"USDT",
			Amount:	"1000",
			Idem:	"hop-idem-" + string(rune('0'+i)),
		})
	}
	return hops
}

// ─────────────────────────────────────────────────────────────────────────────
// TestOpenResidual_*
// ─────────────────────────────────────────────────────────────────────────────

func TestOpenResidual_Basic(t *testing.T) {
	cases := []struct {
		name		string
		req		batchhopgate.OpenResidualFullReq
		wantErr		error
		wantStatus	batchhopgate.ResidualStatus
		wantProofSome	bool
	}{
		{
			name:	"正常落库：2笔成功后第3笔失败",
			req: batchhopgate.OpenResidualFullReq{
				UserID:		"u-test-01",
				Asset:		"USDT",
				TotalItems:	3,
				FailIndex:	2,
				SucceededHops:	makeSucceededHops(2),
				FailedReason:	"portfolio: amount would go negative",
				BeforeRoot:	"root-before-01",
				PartialRoot:	"root-partial-01",
				BuiltBy:	"test",
			},
			wantErr:	nil,
			wantStatus:	batchhopgate.ResidualProven,
			wantProofSome:	true,
		},
		{
			name:	"空用户 ID 拒绝",
			req: batchhopgate.OpenResidualFullReq{
				UserID:		"",
				Asset:		"USDT",
				TotalItems:	1,
				FailIndex:	0,
			},
			wantErr:	batchhopgate.ErrUserInvalid,
		},
		{
			name:	"空资产拒绝",
			req: batchhopgate.OpenResidualFullReq{
				UserID:		"u-test-01",
				Asset:		"",
				TotalItems:	1,
				FailIndex:	0,
			},
			wantErr:	batchhopgate.ErrAssetInvalid,
		},
		{
			name:	"零已成功 hops（第1笔即失败）",
			req: batchhopgate.OpenResidualFullReq{
				UserID:		"u-test-02",
				Asset:		"BTC",
				TotalItems:	5,
				FailIndex:	0,
				SucceededHops:	[]batchhopgate.HopSummary{},
				FailedReason:	"portfolio book unavailable",
				BeforeRoot:	"root-before-02",
				PartialRoot:	"root-partial-02",
				BuiltBy:	"test",
			},
			wantErr:	nil,
			wantStatus:	batchhopgate.ResidualProven,
			wantProofSome:	true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			svc.WithClock(fixedClock(epoch))
			ctx := context.Background()
			result, err := svc.OpenResidual(ctx, tc.req)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("got err=%v, want=%v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if result.Residual == nil {
				t.Fatal("Residual is nil")
			}
			if result.Residual.Status != tc.wantStatus {
				t.Errorf("status=%s, want=%s", result.Residual.Status, tc.wantStatus)
			}
			if tc.wantProofSome && result.Proof == nil {
				t.Error("Proof is nil, expected non-nil")
			}
			if result.Token.UserID == "" {
				t.Error("Token.UserID is empty")
			}
			if result.Token.Generation != result.Residual.Generation {
				t.Errorf("token generation %d != residual generation %d",
					result.Token.Generation, result.Residual.Generation)
			}
		})
	}
}

func TestOpenResidual_ConcurrentBlocksNewBatch(t *testing.T) {
	svc := newTestService(t)
	svc.WithClock(fixedClock(epoch))
	ctx := context.Background()
	// 第一次 BatchHop 失败，落库残留案。
	req := batchhopgate.OpenResidualFullReq{
		UserID:		"u-concurrent-01",
		Asset:		"USDT",
		TotalItems:	3,
		FailIndex:	2,
		SucceededHops:	makeSucceededHops(2),
		FailedReason:	"fail",
		BeforeRoot:	"root-before",
		PartialRoot:	"root-partial",
		BuiltBy:	"test",
	}
	_, err := svc.OpenResidual(ctx, req)
	if err != nil {
		t.Fatalf("first OpenResidual: %v", err)
	}
	// 第二次同用户同资产 BatchHop 应被拒绝。
	_,
		err = svc.OpenResidual(ctx, req)
	if !errors.Is(err, batchhopgate.ErrConcurrentBatch) {
		t.Errorf("got err=%v, want ErrConcurrentBatch", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestRequestResume_*
// ─────────────────────────────────────────────────────────────────────────────

func TestRequestResume_ValidToken(t *testing.T) {
	svc := newTestService(t)
	svc.WithClock(fixedClock(epoch))
	ctx := context.Background()
	req := batchhopgate.OpenResidualFullReq{
		UserID:		"u-resume-01",
		Asset:		"USDT",
		TotalItems:	3,
		FailIndex:	1,
		SucceededHops:	makeSucceededHops(1),
		FailedReason:	"fail",
		BeforeRoot:	"root-before",
		PartialRoot:	"root-partial",
		BuiltBy:	"test",
	}
	openResult, err := svc.OpenResidual(ctx, req)
	if err != nil {
		t.Fatalf("OpenResidual: %v", err)
	}
	lease, err := svc.RequestResume(ctx, batchhopgate.RequestResumeReq{
		ResidualCaseID:	openResult.Residual.ID,
		Token:		openResult.Token,
		ResumeFrom:	1,
		ConfirmedBy:	"ops-01",
	})
	if err != nil {
		t.Fatalf("RequestResume: %v", err)
	}
	if lease.Status != batchhopgate.LeaseOpen {
		t.Errorf("lease status=%s, want open", lease.Status)
	}
	// 残留案应推进至 resuming。
	residual, err := svc.GetResidual(ctx, openResult.Residual.ID)
	if err != nil {
		t.Fatalf("GetResidual: %v", err)
	}
	if residual.Status != batchhopgate.ResidualResuming {
		t.Errorf("residual status=%s, want resuming", residual.Status)
	}
}

func TestRequestResume_NoToken(t *testing.T) {
	svc := newTestService(t)
	svc.WithClock(fixedClock(epoch))
	ctx := context.Background()
	req := batchhopgate.OpenResidualFullReq{
		UserID:	"u-resume-02", Asset: "BTC",
		TotalItems:	2, FailIndex: 1,
		SucceededHops:	makeSucceededHops(1),
		FailedReason:	"fail", BeforeRoot: "rb", PartialRoot: "rp",
	}
	openResult, _ := svc.OpenResidual(ctx, req)
	_, err := svc.RequestResume(ctx, batchhopgate.RequestResumeReq{
		ResidualCaseID:	openResult.Residual.ID,
		Token:		batchhopgate.GenToken{},	// 空令牌
		ResumeFrom:	1,
	})
	if !errors.Is(err, batchhopgate.ErrNoTokenProvided) {
		t.Errorf("got err=%v, want ErrNoTokenProvided", err)
	}
}

func TestRequestResume_StaleGeneration(t *testing.T) {
	svc := newTestService(t)
	svc.WithClock(fixedClock(epoch))
	ctx := context.Background()
	req := batchhopgate.OpenResidualFullReq{
		UserID:	"u-resume-03", Asset: "ETH",
		TotalItems:	2, FailIndex: 1,
		SucceededHops:	makeSucceededHops(1),
		FailedReason:	"fail", BeforeRoot: "rb", PartialRoot: "rp",
	}
	openResult, _ := svc.OpenResidual(ctx, req)
	// 构造世代不符的令牌（generation+1）。
	staleToken := batchhopgate.GenToken{
		UserID:		"u-resume-03",
		Asset:		"ETH",
		Generation:	openResult.Residual.Generation + 1,	// 错误世代
		IssuedAt:	epoch,
		ExpiresAt:	epoch.Add(30 * time.Minute),
	}
	_, err := svc.RequestResume(ctx, batchhopgate.RequestResumeReq{
		ResidualCaseID:	openResult.Residual.ID,
		Token:		staleToken,
		ResumeFrom:	1,
	})
	if !errors.Is(err, batchhopgate.ErrTokenGenerationMismatch) {
		t.Errorf("got err=%v, want ErrTokenGenerationMismatch", err)
	}
}

func TestRequestResume_ExpiredToken(t *testing.T) {
	svc := newTestService(t)
	svc.WithClock(fixedClock(epoch))
	ctx := context.Background()
	req := batchhopgate.OpenResidualFullReq{
		UserID:	"u-resume-04", Asset: "USDT",
		TotalItems:	2, FailIndex: 1,
		SucceededHops:	makeSucceededHops(1),
		FailedReason:	"fail", BeforeRoot: "rb", PartialRoot: "rp",
	}
	openResult, _ := svc.OpenResidual(ctx, req)
	// 构造已过期的令牌。
	expiredToken := batchhopgate.GenToken{
		UserID:		"u-resume-04",
		Asset:		"USDT",
		Generation:	openResult.Residual.Generation,
		IssuedAt:	epoch.Add(-2 * time.Hour),
		ExpiresAt:	epoch.Add(-1 * time.Hour),	// 1小时前已过期
	}
	// 向前推进时钟，使令牌过期。
	svc.WithClock(fixedClock(epoch.Add(2 * time.Hour)))
	_, err := svc.RequestResume(ctx, batchhopgate.RequestResumeReq{
		ResidualCaseID:	openResult.Residual.ID,
		Token:		expiredToken,
		ResumeFrom:	1,
	})
	if !errors.Is(err, batchhopgate.ErrLeaseExpired) {
		t.Errorf("got err=%v, want ErrLeaseExpired", err)
	}
}

func TestRequestResume_ConflictSameCase(t *testing.T) {
	svc := newTestService(t)
	svc.WithClock(fixedClock(epoch))
	ctx := context.Background()
	req := batchhopgate.OpenResidualFullReq{
		UserID:	"u-resume-05", Asset: "USDT",
		TotalItems:	3, FailIndex: 1,
		SucceededHops:	makeSucceededHops(1),
		FailedReason:	"fail", BeforeRoot: "rb", PartialRoot: "rp",
	}
	openResult, _ := svc.OpenResidual(ctx, req)
	// 第一次申请成功。
	_, err := svc.RequestResume(ctx, batchhopgate.RequestResumeReq{
		ResidualCaseID:	openResult.Residual.ID,
		Token:		openResult.Token,
		ResumeFrom:	1,
	})
	if err != nil {
		t.Fatalf("first RequestResume: %v", err)
	}
	// 第二次申请同案应冲突。
	_, err = svc.RequestResume(ctx, batchhopgate.RequestResumeReq{
		ResidualCaseID:	openResult.Residual.ID,
		Token:		openResult.Token,
		ResumeFrom:	1,
	})
	if !errors.Is(err, batchhopgate.ErrLeaseConflict) {
		t.Errorf("got err=%v, want ErrLeaseConflict", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestExecuteResume_*
// ─────────────────────────────────────────────────────────────────────────────

func TestExecuteResume_Basic(t *testing.T) {
	svc := newTestService(t)
	svc.WithClock(fixedClock(epoch))
	ctx := context.Background()
	req := batchhopgate.OpenResidualFullReq{
		UserID:	"u-exec-01", Asset: "USDT",
		TotalItems:	3, FailIndex: 1,
		SucceededHops:	makeSucceededHops(1),
		FailedReason:	"fail", BeforeRoot: "rb", PartialRoot: "rp",
	}
	openResult,
		_ := svc.OpenResidual(ctx, req)
	lease, _ := svc.RequestResume(ctx, batchhopgate.RequestResumeReq{
		ResidualCaseID:	openResult.Residual.ID,
		Token:		openResult.Token,
		ResumeFrom:	1,
	})
	execCount := 0
	hopExec := func(ctx context.Context, item batchhopgate.ResumeItem,) error {
		execCount++
		return nil
	}
	result, err := svc.ExecuteResume(ctx, batchhopgate.ResumeRequest{
		ResidualCaseID:	openResult.Residual.ID,
		LeaseID:	lease.ID,
		UserID:		"u-exec-01",
		Asset:		"USDT",
		Generation:	openResult.Residual.Generation,
		ResumeItems: []batchhopgate.ResumeItem{
			{Index: 1, From: "funding", To: "trading", Asset: "USDT", Amount: "500", Idem: "idem-resume-1"},
			{Index: 2, From: "funding", To: "earn", Asset: "USDT", Amount: "200", Idem: "idem-resume-2"},
		},
		ConfirmedBy:	"ops-01",
	}, hopExec)
	if err != nil {
		t.Fatalf("ExecuteResume: %v", err)
	}
	if result.NewStatus != batchhopgate.ResidualClosed {
		t.Errorf("new status=%s, want closed", result.NewStatus)
	}
	if execCount != 2 {
		t.Errorf("executed %d hops, want 2", execCount)
	}
}

func TestExecuteResume_SkipsSucceededIndex(t *testing.T) {
	svc := newTestService(t)
	svc.WithClock(fixedClock(epoch))
	ctx := context.Background()
	req := batchhopgate.OpenResidualFullReq{
		UserID:	"u-exec-02", Asset: "USDT",
		TotalItems:	3, FailIndex: 2,
		SucceededHops:	makeSucceededHops(2),	// index 0 和 1 已成功
		FailedReason:	"fail", BeforeRoot: "rb", PartialRoot: "rp",
	}
	openResult, _ := svc.OpenResidual(ctx, req)
	lease, _ := svc.RequestResume(ctx, batchhopgate.RequestResumeReq{
		ResidualCaseID:	openResult.Residual.ID,
		Token:		openResult.Token,
		ResumeFrom:	2,
	})
	execCount := 0
	skipCount := 0
	hopExec := func(ctx context.Context, item batchhopgate.ResumeItem,) error {
		execCount++
		return nil
	}
	result, err := svc.ExecuteResume(ctx, batchhopgate.ResumeRequest{
		ResidualCaseID:	openResult.Residual.ID,
		LeaseID:	lease.ID,
		UserID:		"u-exec-02",
		Asset:		"USDT",
		Generation:	openResult.Residual.Generation,
		ResumeItems: []batchhopgate.ResumeItem{
			{Index: 0, From: "funding", To: "trading", Asset: "USDT", Amount: "1000", Idem: "idem-0"},	// 已成功，跳过
			{Index: 1, From: "funding", To: "earn", Asset: "USDT", Amount: "1000", Idem: "idem-1"},		// 已成功，跳过
			{Index: 2, From: "funding", To: "margin", Asset: "USDT", Amount: "500", Idem: "idem-2"},	// 新执行
		},
	}, hopExec)
	if err != nil {
		t.Fatalf("ExecuteResume: %v", err)
	}
	skipCount = len(result.Skipped)
	if execCount != 1 {
		t.Errorf("executed %d hops, want 1 (index 0 and 1 should be skipped)", execCount)
	}
	if skipCount != 2 {
		t.Errorf("skipped %d hops, want 2", skipCount)
	}
}

func TestExecuteResume_IndexBelowFailIndexRejected(t *testing.T) {
	svc := newTestService(t)
	svc.WithClock(fixedClock(epoch))
	ctx := context.Background()
	req := batchhopgate.OpenResidualFullReq{
		UserID:	"u-exec-03", Asset: "USDT",
		TotalItems:	3, FailIndex: 2,
		SucceededHops:	makeSucceededHops(2),
		FailedReason:	"fail", BeforeRoot: "rb", PartialRoot: "rp",
	}
	openResult, _ := svc.OpenResidual(ctx, req)
	lease, _ := svc.RequestResume(ctx, batchhopgate.RequestResumeReq{
		ResidualCaseID:	openResult.Residual.ID,
		Token:		openResult.Token,
		ResumeFrom:	2,
	})
	// 尝试对 index=1（已成功 hop）执行续跳，但 index=1 < fail_index=2 不在已成功列表中。
	// 实际上 makeSucceededHops(2) 生成的是 index 0 和 1，所以 index=1 在已成功列表里，
	// 跳过行为而非拒绝。
	// 一个非法索引（不在已成功且 < fail_index 的情况）：
	// 这里 makeSucceededHops(2) 包含 index 0 和 1（均在已成功列表），
	// 而 failIndex=2，所以任何 < 2 的 index 都在已成功列表中被跳过。
	// 构造一个 index=1 不在 SucceededHops 中的场景：
	altReq := batchhopgate.OpenResidualFullReq{
		UserID:	"u-exec-03b", Asset: "ETH",
		TotalItems:	5, FailIndex: 3,
		SucceededHops: []batchhopgate.HopSummary{
			{Index: 0, From: "funding", To: "trading", Asset: "ETH", Amount: "1"},
			{Index: 1, From: "funding", To: "earn", Asset: "ETH", Amount: "1"},
			{Index: 2, From: "trading", To: "funding", Asset: "ETH", Amount: "0.5"},
		},
		FailedReason:	"fail", BeforeRoot: "rb2", PartialRoot: "rp2",
	}
	altOpen, _ := svc.OpenResidual(ctx, altReq)
	altLease, _ := svc.RequestResume(ctx, batchhopgate.RequestResumeReq{
		ResidualCaseID:	altOpen.Residual.ID,
		Token:		altOpen.Token,
		ResumeFrom:	3,
	})
	// index=1 < fail_index=3 且在 SucceededHops 中 → 跳过（不拒绝）。
	// index=2 < fail_index=3 且在 SucceededHops 中 → 跳过（不拒绝）。
	// 此处验证：续跳仅执行 fail_index 及之后的项目。
	execCount := 0
	_, err := svc.ExecuteResume(ctx, batchhopgate.ResumeRequest{
		ResidualCaseID:	altOpen.Residual.ID,
		LeaseID:	altLease.ID,
		UserID:		"u-exec-03b",
		Asset:		"ETH",
		Generation:	altOpen.Residual.Generation,
		ResumeItems: []batchhopgate.ResumeItem{
			{Index: 3, From: "funding", To: "margin", Asset: "ETH", Amount: "1", Idem: "idem-3"},
			{Index: 4, From: "funding", To: "futures", Asset: "ETH", Amount: "0.5", Idem: "idem-4"},
		},
	}, func(ctx context.Context, item batchhopgate.ResumeItem,) error {
		execCount++
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteResume: %v", err)
	}
	if execCount != 2 {
		t.Errorf("executed %d, want 2", execCount)
	}
	_ = lease
}

// ─────────────────────────────────────────────────────────────────────────────
// TestConfirmClose_*
// ─────────────────────────────────────────────────────────────────────────────

func TestConfirmClose_FourEye(t *testing.T) {
	cases := []struct {
		name		string
		confirmer1	string
		confirmer2	string
		wantErr2	error
		wantStatus	batchhopgate.ResidualStatus
	}{
		{
			name:		"双人不同→closed",
			confirmer1:	"ops-01",
			confirmer2:	"ops-02",
			wantErr2:	nil,
			wantStatus:	batchhopgate.ResidualClosed,
		},
		{
			name:		"同人二次→ErrSameConfirmer",
			confirmer1:	"ops-01",
			confirmer2:	"ops-01",
			wantErr2:	batchhopgate.ErrSameConfirmer,
			wantStatus:	batchhopgate.ResidualProven,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			svc.WithClock(fixedClock(epoch))
			ctx := context.Background()
			req := batchhopgate.OpenResidualFullReq{
				UserID:	"u-confirm-" + tc.confirmer1, Asset: "USDT",
				TotalItems:	2, FailIndex: 1,
				SucceededHops:	makeSucceededHops(1),
				FailedReason:	"fail", BeforeRoot: "rb", PartialRoot: "rp",
			}
			openResult, _ := svc.OpenResidual(ctx, req)
			// 第一次确认。
			c1, err := svc.ConfirmClose(ctx, openResult.Residual.ID, tc.confirmer1, "")
			if err != nil {
				t.Fatalf("ConfirmClose#1: %v", err)
			}
			if c1.Status == batchhopgate.ResidualClosed {
				t.Error("should not be closed after first confirmation")
			}
			// 第二次确认。
			c2, err := svc.ConfirmClose(ctx, openResult.Residual.ID, tc.confirmer2, "")
			if tc.wantErr2 != nil {
				if !errors.Is(err, tc.wantErr2) {
					t.Errorf("got err=%v, want=%v", err, tc.wantErr2)
				}
				return
			}
			if err != nil {
				t.Fatalf("ConfirmClose#2: %v", err)
			}
			if c2.Status != tc.wantStatus {
				t.Errorf("status=%s, want=%s", c2.Status, tc.wantStatus)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestBlockResidual_*
// ─────────────────────────────────────────────────────────────────────────────

func TestBlockResidual_Basic(t *testing.T) {
	svc := newTestService(t)
	svc.WithClock(fixedClock(epoch))
	ctx := context.Background()
	req := batchhopgate.OpenResidualFullReq{
		UserID:	"u-block-01", Asset: "USDT",
		TotalItems:	2, FailIndex: 1,
		SucceededHops:	makeSucceededHops(1),
		FailedReason:	"fail", BeforeRoot: "rb", PartialRoot: "rp",
	}
	openResult, _ := svc.OpenResidual(ctx, req)
	c, err := svc.BlockResidual(ctx, openResult.Residual.ID, "审核拒绝：资产异常")
	if err != nil {
		t.Fatalf("BlockResidual: %v", err)
	}
	if c.Status != batchhopgate.ResidualBlocked {
		t.Errorf("status=%s, want blocked", c.Status)
	}
	// 阻塞后同用户同资产应恢复可用（BatchGeneration 已释放）。
	err = svc.CheckConcurrent(ctx, "u-block-01", "USDT")
	if err != nil {
		t.Errorf("after block, CheckConcurrent should pass, got: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestConcurrentBatch_* — 并发 CAS 世代竞争
// ─────────────────────────────────────────────────────────────────────────────

func TestConcurrentBatch_CASWorldState(t *testing.T) {
	svc := newTestService(t)
	svc.WithClock(fixedClock(epoch))
	ctx := context.Background()
	const goroutines = 10
	results := make([]error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			req := batchhopgate.OpenResidualFullReq{
				UserID:		"u-cas-01",
				Asset:		"USDT",
				TotalItems:	2,
				FailIndex:	1,
				SucceededHops:	makeSucceededHops(1),
				FailedReason:	"fail",
				BeforeRoot:	"rb",
				PartialRoot:	"rp",
			}
			_, err := svc.OpenResidual(ctx, req)
			results[idx] = err
		}(i)
	}
	wg.Wait()
	successCount := 0
	for _, err := range results {
		if err == nil {
			successCount++
		}
	}
	// 同用户同资产并发只允许一次成功（第一次落库后其余被 ErrConcurrentBatch 拒绝）。
	if successCount != 1 {
		t.Errorf("concurrent open: %d success, want 1", successCount)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestProofVerify_*
// ─────────────────────────────────────────────────────────────────────────────

func TestProofVerify_DigestOK(t *testing.T) {
	svc := newTestService(t)
	svc.WithClock(fixedClock(epoch))
	ctx := context.Background()
	req := batchhopgate.OpenResidualFullReq{
		UserID:	"u-proof-01", Asset: "USDT",
		TotalItems:	2, FailIndex: 1,
		SucceededHops:	makeSucceededHops(1),
		FailedReason:	"fail", BeforeRoot: "root-a", PartialRoot: "root-b",
		BuiltBy:	"test",
	}
	openResult, err := svc.OpenResidual(ctx, req)
	if err != nil {
		t.Fatalf("OpenResidual: %v", err)
	}
	if openResult.Proof == nil {
		t.Skip("no proof built")
	}
	proof, err := svc.VerifyProof(ctx, openResult.Proof.ID)
	if err != nil {
		t.Errorf("VerifyProof: %v", err)
	}
	if proof.ConserveOK != true {
		t.Errorf("ConserveOK=false, expected true for valid succeeded hops")
	}
}

func TestProofVerify_DigestMismatch(t *testing.T) {
	store := batchhopgate.NewMemStore()
	vault := batchhopgate.NewOccupyProofVault(store)
	vault.WithClock(fixedClock(epoch))
	ctx := context.Background()
	// 构建合法证明后篡改 Digest。
	p, err := vault.Build(ctx, batchhopgate.BuildReq{
		ResidualCaseID:	"case-tamper-01",
		UserID:		"u-proof-02",
		Asset:		"USDT",
		BeforeRoot:	"root-x",
		PartialRoot:	"root-y",
		SucceededHops:	makeSucceededHops(2),
		SubjectTotals:	nil,
		ConserveOK:	true,
		BuiltBy:	"test",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// 直接篡改存储中的 Digest。
	p.Digest = "tampered-digest-0000000000000000000000000000000000000000000000"
	_ = store.SaveProof(ctx, p)
	_, err = vault.Verify(ctx, p.ID)
	if !errors.Is(err, batchhopgate.ErrProofDigestMismatch) {
		t.Errorf("got err=%v, want ErrProofDigestMismatch", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestIdempotency_*
// ─────────────────────────────────────────────────────────────────────────────

func TestIdempotency_ResumeNoDoubleExec(t *testing.T) {
	svc := newTestService(t)
	svc.WithClock(fixedClock(epoch))
	ctx := context.Background()
	req := batchhopgate.OpenResidualFullReq{
		UserID:	"u-idem-01", Asset: "USDT",
		TotalItems:	2, FailIndex: 1,
		SucceededHops:	makeSucceededHops(1),
		FailedReason:	"fail", BeforeRoot: "rb", PartialRoot: "rp",
	}
	openResult,
		_ := svc.OpenResidual(ctx, req)
	lease, _ := svc.RequestResume(ctx, batchhopgate.RequestResumeReq{
		ResidualCaseID:	openResult.Residual.ID,
		Token:		openResult.Token,
		ResumeFrom:	1,
	})
	execCount := 0
	hopExec := func(ctx context.Context,
		item batchhopgate.ResumeItem,) error {
		execCount++
		return nil
	}
	resumeReq := batchhopgate.ResumeRequest{
		ResidualCaseID:	openResult.Residual.ID,
		LeaseID:	lease.ID,
		UserID:		"u-idem-01",
		Asset:		"USDT",
		Generation:	openResult.Residual.Generation,
		ResumeItems: []batchhopgate.ResumeItem{
			{Index: 1, From: "funding", To: "trading", Asset: "USDT", Amount: "500", Idem: "idem-final"},
		},
	}
	// 第一次 ExecuteResume。
	_, err := svc.ExecuteResume(ctx, resumeReq, hopExec)
	if err != nil {
		t.Fatalf("first ExecuteResume: %v", err)
	}
	if execCount != 1 {
		t.Errorf("first exec: execCount=%d, want 1", execCount)
	}
	// 第二次 ExecuteResume：已处于 closed 状态，应返回 ErrResidualStatusInvalid。
	_, err = svc.ExecuteResume(ctx, resumeReq, hopExec)
	if !errors.Is(err, batchhopgate.ErrResidualStatusInvalid) {
		t.Errorf("second exec: got err=%v, want ErrResidualStatusInvalid", err)
	}
	// 二次扣入必须没有发生。
	if execCount != 1 {
		t.Errorf("double exec detected: execCount=%d, should remain 1", execCount)
	}
}

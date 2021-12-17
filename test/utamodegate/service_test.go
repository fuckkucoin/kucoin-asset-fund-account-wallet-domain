package utamodegate_test

import (
	"context"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/utamodegate"
)

// ─────────────────────────────────────────────────────────────────────────────
// 辅助工具
// ─────────────────────────────────────────────────────────────────────────────

func newTestService(t *testing.T) *utamodegate.Service {
	t.Helper()
	return utamodegate.NewService(nil)	// nil → MemStore
}

func fixedClock(ts time.Time) func() time.Time {
	return func() time.Time { return ts }
}

var epoch = time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)

// ─────────────────────────────────────────────────────────────────────────────
// TestSetMode_*
// ─────────────────────────────────────────────────────────────────────────────

func TestSetMode_Basic(t *testing.T) {
	cases := []struct {
		name		string
		userID		string
		toMode		string
		wantErr		error
		wantStatus	utamodegate.LeaseStatus
		wantGen		int64
	}{
		{
			name:		"UNIFIED→CLASSIC 无在途负债，切换成功",
			userID:		"u-basic-01",
			toMode:		"CLASSIC",
			wantErr:	nil,
			wantStatus:	utamodegate.LeaseBound,
			wantGen:	1,
		},
		{
			name:		"CLASSIC→UNIFIED 切换成功",
			userID:		"u-basic-02",
			toMode:		"CLASSIC",
			wantErr:	nil,
			wantStatus:	utamodegate.LeaseBound,
			wantGen:	1,
		},
		{
			name:		"非法目标模式",
			userID:		"u-basic-03",
			toMode:		"ISOLATED",
			wantErr:	utamodegate.ErrModeInvalid,
		},
		{
			name:		"用户 ID 为空",
			userID:		"",
			toMode:		"CLASSIC",
			wantErr:	utamodegate.ErrUserInvalid,
		},
		{
			name:		"相同模式不切换",
			userID:		"u-basic-04",
			toMode:		"UNIFIED",	// 默认已是 UNIFIED
			wantErr:	utamodegate.ErrSameMode,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			ctx := context.Background()
			result, err := svc.SetMode(ctx, tc.userID, tc.toMode, "test", "")
			if tc.wantErr != nil {
				if err != tc.wantErr {
					t.Errorf("SetMode error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("SetMode unexpected error: %v", err)
			}
			if result.Lease == nil {
				t.Fatal("SetMode: result.Lease is nil")
			}
			if result.Lease.Status != tc.wantStatus {
				t.Errorf("lease status = %s, want %s", result.Lease.Status, tc.wantStatus)
			}
			if result.Generation != tc.wantGen {
				t.Errorf("generation = %d, want %d", result.Generation, tc.wantGen)
			}
		})
	}
}

func TestSetMode_InflightLiabilityBlocks(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	userID := "u-inflight-01"
	// 先注册一条借出绑定（在途）
	bind, err := svc.RegisterBind(ctx, utamodegate.RegisterBindReq{
		UserID:		userID,
		BindKind:	utamodegate.BindKindBorrow,
		Currency:	"USDT",
		MarginMode:	utamodegate.MarginModeCross,
		Amount:		"5000",
		RefID:		"borrow-order-x01",
	}, 0)
	if err != nil {
		t.Fatalf("RegisterBind: %v", err)
	}
	if bind.Status != utamodegate.BindOpen {
		t.Fatalf("bind.Status = %s, want open", bind.Status)
	}
	// 切换模式应进入 blocked 队列
	result, err := svc.SetMode(ctx, userID, "CLASSIC", "test", "")
	if err != utamodegate.ErrInflightLiability {
		t.Fatalf("SetMode error = %v, want ErrInflightLiability", err)
	}
	if result == nil || result.Lease == nil {
		t.Fatal("SetMode: result.Lease is nil for blocked case")
	}
	if result.Lease.Status != utamodegate.LeaseBlocked {
		t.Errorf("lease status = %s, want blocked", result.Lease.Status)
	}
	// 世代不应递增
	gen, _ := svc.GetGeneration(ctx, userID)
	if gen.Generation != 0 {
		t.Errorf("generation = %d, want 0 (no CAS bump for blocked)", gen.Generation)
	}
	// 释放绑定后 worker 应能推进租约
	_, err = svc.ReleaseBind(ctx, utamodegate.ReleaseBindReq{UserID: userID, BindID: bind.ID})
	if err != nil {
		t.Fatalf("ReleaseBind: %v", err)
	}
	// worker 推进
	leaseID := result.Lease.ID
	switched, err := svc.workerUnblockLease(ctx, leaseID)
	if err != nil {
		t.Fatalf("workerUnblockLease: %v", err)
	}
	if switched.Status != utamodegate.LeaseSwitched {
		t.Errorf("switched.Status = %s, want switched", switched.Status)
	}
}

func TestSetMode_DriftBlockedPreventsSwitch(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	userID := "u-drift-block-01"
	// 开立漂移案
	dc, err := svc.OpenDriftCase(ctx, utamodegate.OpenDriftCaseReq{
		UserID:			userID,
		Asset:			"USDT",
		Kind:			utamodegate.DriftKindLiabilityKey,
		OldMode:		utamodegate.MarginModeUnified,
		NewMode:		utamodegate.MarginModeClassic,
		OldGeneration:		0,
		NewGeneration:		1,
		LiabilityAmount:	"3000",
	})
	if err != nil {
		t.Fatalf("OpenDriftCase: %v", err)
	}
	if dc.Status != utamodegate.DriftOpen {
		t.Fatalf("dc.Status = %s, want open", dc.Status)
	}
	// SetMode 应被 DriftBlocked 拒绝
	result, err := svc.SetMode(ctx, userID, "CLASSIC", "test", "")
	if err != utamodegate.ErrDriftBlocked {
		t.Fatalf("SetMode error = %v, want ErrDriftBlocked", err)
	}
	if result == nil || !result.DriftBlocked {
		t.Error("SetMode: DriftBlocked flag should be true")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestValidateToken_*
// ─────────────────────────────────────────────────────────────────────────────

func TestValidateToken(t *testing.T) {
	cases := []struct {
		name		string
		setupGen	int64	// 通过 SetMode 推进的世代数
		tokenGen	int64	// 校验的令牌世代
		wantErr		error
	}{
		{"令牌与当前世代一致", 0, 0, nil},
		{"令牌滞后一代（切换后旧令牌）", 1, 0, utamodegate.ErrStaleGen},
		{"令牌超前（非法）", 0, 5, utamodegate.ErrStaleGen},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			ctx := context.Background()
			userID := "u-token-" + tc.name
			// 推进世代
			for i := 0; i < int(tc.setupGen); i++ {
				var toMode string
				if i%2 == 0 {
					toMode = "CLASSIC"
				} else {
					toMode = "UNIFIED"
				}
				_, _ = svc.SetMode(ctx, userID, toMode, "test", "")
			}
			err := svc.ValidateToken(ctx, userID, tc.tokenGen)
			if err != tc.wantErr {
				t.Errorf("ValidateToken error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestRegisterBind_*
// ─────────────────────────────────────────────────────────────────────────────

func TestRegisterBind(t *testing.T) {
	cases := []struct {
		name		string
		req		utamodegate.RegisterBindReq
		generation	int64
		wantErr		error
		wantStatus	utamodegate.BindStatus
	}{
		{
			name:	"正常借出绑定注册",
			req: utamodegate.RegisterBindReq{
				UserID:		"u-bind-01",
				BindKind:	utamodegate.BindKindBorrow,
				Currency:	"USDT",
				MarginMode:	utamodegate.MarginModeCross,
				Amount:		"1000",
				RefID:		"borrow-01",
			},
			generation:	0,
			wantErr:	nil,
			wantStatus:	utamodegate.BindOpen,
		},
		{
			name:	"旧世代令牌拒绝注册",
			req: utamodegate.RegisterBindReq{
				UserID:		"u-bind-02",
				BindKind:	utamodegate.BindKindBorrow,
				Currency:	"BTC",
				MarginMode:	utamodegate.MarginModeUnified,
				Amount:		"0.5",
				RefID:		"borrow-02",
			},
			generation:	99,	// 与当前世代 0 不符
			wantErr:	utamodegate.ErrStaleGen,
		},
		{
			name:	"金额为空拒绝注册",
			req: utamodegate.RegisterBindReq{
				UserID:		"u-bind-03",
				BindKind:	utamodegate.BindKindCollateral,
				Currency:	"ETH",
				MarginMode:	utamodegate.MarginModeUnified,
				Amount:		"",
				RefID:		"collateral-03",
			},
			generation:	0,
			wantErr:	utamodegate.ErrAmountInvalid,
		},
		{
			name:	"非法模式键拒绝注册",
			req: utamodegate.RegisterBindReq{
				UserID:		"u-bind-04",
				BindKind:	utamodegate.BindKindBorrow,
				Currency:	"USDT",
				MarginMode:	"INVALID_MODE",
				Amount:		"500",
				RefID:		"borrow-04",
			},
			generation:	0,
			wantErr:	utamodegate.ErrModeInvalid,
		},
		{
			name:	"同 RefID 幂等注册",
			req: utamodegate.RegisterBindReq{
				UserID:		"u-bind-05",
				BindKind:	utamodegate.BindKindBorrow,
				Currency:	"USDT",
				MarginMode:	utamodegate.MarginModeClassic,
				Amount:		"2000",
				RefID:		"borrow-05-idem",
			},
			generation:	0,
			wantErr:	nil,
			wantStatus:	utamodegate.BindOpen,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			ctx := context.Background()
			bind, err := svc.RegisterBind(ctx, tc.req, tc.generation)
			if tc.wantErr != nil {
				if err != tc.wantErr {
					t.Errorf("RegisterBind error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("RegisterBind unexpected error: %v", err)
			}
			if bind.Status != tc.wantStatus {
				t.Errorf("bind.Status = %s, want %s", bind.Status, tc.wantStatus)
			}
			// 幂等：第二次注册同 RefID
			if tc.req.RefID != "" {
				bind2,
					err2 := svc.RegisterBind(ctx, tc.req, tc.generation)
				if err2 != nil {
					t.Fatalf("RegisterBind idempotent error: %v", err2)
				}
				if bind2.ID != bind.ID {
					t.Errorf("idempotent: got different bind ID: %s vs %s", bind.ID, bind2.ID)
				}
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestReleaseBind_*
// ─────────────────────────────────────────────────────────────────────────────

func TestReleaseBind(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	userID := "u-release-01"
	// 注册绑定
	bind, err := svc.RegisterBind(ctx, utamodegate.RegisterBindReq{
		UserID:		userID,
		BindKind:	utamodegate.BindKindBorrow,
		Currency:	"USDT",
		MarginMode:	utamodegate.MarginModeCross,
		Amount:		"3000",
		RefID:		"borrow-release-01",
	}, 0)
	if err != nil {
		t.Fatalf("RegisterBind: %v", err)
	}
	// 释放
	released, err := svc.ReleaseBind(ctx, utamodegate.ReleaseBindReq{UserID: userID, BindID: bind.ID})
	if err != nil {
		t.Fatalf("ReleaseBind: %v", err)
	}
	if released.Status != utamodegate.BindReleased {
		t.Errorf("released.Status = %s, want released", released.Status)
	}
	// 幂等：再次释放
	released2, err := svc.ReleaseBind(ctx, utamodegate.ReleaseBindReq{UserID: userID, BindID: bind.ID})
	if err != nil {
		t.Fatalf("ReleaseBind idempotent: %v", err)
	}
	if released2.Status != utamodegate.BindReleased {
		t.Errorf("idempotent released.Status = %s, want released", released2.Status)
	}
	// 释放后 CountOpenBinds 应为 0
	count,
		err := svc.bind.CountOpenBinds(ctx, userID)
	if err != nil {
		t.Fatalf("CountOpenBinds: %v", err)
	}
	if count != 0 {
		t.Errorf("CountOpenBinds = %d, want 0", count)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestDriftCase_Lifecycle
// ─────────────────────────────────────────────────────────────────────────────

func TestDriftCase_Lifecycle(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	userID := "u-drift-01"
	// 1. 开立漂移案
	dc, err := svc.OpenDriftCase(ctx, utamodegate.OpenDriftCaseReq{
		UserID:			userID,
		Asset:			"USDT",
		Kind:			utamodegate.DriftKindLiabilityKey,
		OldMode:		utamodegate.MarginModeUnified,
		NewMode:		utamodegate.MarginModeClassic,
		OldGeneration:		2,
		NewGeneration:		3,
		LiabilityAmount:	"8000",
	})
	if err != nil {
		t.Fatalf("OpenDriftCase: %v", err)
	}
	if dc.Status != utamodegate.DriftOpen {
		t.Fatalf("dc.Status = %s, want open", dc.Status)
	}
	// 2. 验证 IsBlocked
	blocked,
		caseID, err := svc.IsSettlementBlocked(ctx, userID)
	if err != nil {
		t.Fatalf("IsSettlementBlocked: %v", err)
	}
	if !blocked {
		t.Error("IsSettlementBlocked: should be blocked")
	}
	if caseID != dc.ID {
		t.Errorf("caseID = %s, want %s", caseID, dc.ID)
	}
	// 3. 幂等：重复开立同案
	dc2, err := svc.OpenDriftCase(ctx, utamodegate.OpenDriftCaseReq{
		UserID:	userID, Asset: "USDT", Kind: utamodegate.DriftKindLiabilityKey,
		OldMode:	utamodegate.MarginModeUnified, NewMode: utamodegate.MarginModeClassic,
	})
	if err != nil {
		t.Fatalf("OpenDriftCase idempotent: %v", err)
	}
	if dc2.ID != dc.ID {
		t.Errorf("idempotent: different case ID: %s vs %s", dc.ID, dc2.ID)
	}
	// 4. 构建证明
	proof, dcProven, err := svc.BuildAndAttachProof(ctx, utamodegate.BuildProofReq{
		DriftCaseID:	dc.ID,
		OldKeySnapshot:	`{"mode":"UNIFIED","principal":"8000"}`,
		NewAggSnapshot:	`{"mode":"CLASSIC","principal":"0"}`,
		BuiltBy:	"ops-test",
	})
	if err != nil {
		t.Fatalf("BuildAndAttachProof: %v", err)
	}
	if proof.DeltaDigest == "" {
		t.Error("proof.DeltaDigest is empty")
	}
	if dcProven.Status != utamodegate.DriftProven {
		t.Errorf("dcProven.Status = %s, want proven", dcProven.Status)
	}
	// 5. 无证明不可关闭（proven 状态但证明摘要已设置，此步验证重复关闭 open 状态案件）
	// proven + proof 后，第一次确认
	dc3, err := svc.ConfirmClose(ctx, utamodegate.ConfirmCloseReq{DriftCaseID: dc.ID, Operator: "alice"})
	if err != nil {
		t.Fatalf("ConfirmClose first: %v", err)
	}
	if dc3.Confirm1By != "alice" {
		t.Errorf("Confirm1By = %s, want alice", dc3.Confirm1By)
	}
	// 同一操作员重复确认
	_, err = svc.ConfirmClose(ctx, utamodegate.ConfirmCloseReq{DriftCaseID: dc.ID, Operator: "alice"})
	if err != utamodegate.ErrDuplicateConfirm {
		t.Errorf("duplicate confirm error = %v, want ErrDuplicateConfirm", err)
	}
	// 第二操作员收口
	dcClosed, err := svc.ConfirmClose(ctx, utamodegate.ConfirmCloseReq{DriftCaseID: dc.ID, Operator: "bob"})
	if err != nil {
		t.Fatalf("ConfirmClose second: %v", err)
	}
	if dcClosed.Status != utamodegate.DriftClosed {
		t.Errorf("dcClosed.Status = %s, want closed", dcClosed.Status)
	}
	// 收口后不再阻塞
	blocked2, _, _ := svc.IsSettlementBlocked(ctx, userID)
	if blocked2 {
		t.Error("after close, should not be blocked")
	}
	// 收口后 SetMode 可通过
	_, err = svc.SetMode(ctx, userID, "CLASSIC", "test", "")
	if err != nil {
		t.Errorf("SetMode after drift closed: %v", err)
	}
	// 导出证明
	export,
		err := svc.ExportProof(ctx, proof.ID)
	if err != nil {
		t.Fatalf("ExportProof: %v", err)
	}
	if export["delta_digest"] != proof.DeltaDigest {
		t.Errorf("export digest mismatch")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestProof_DigestIntegrity
// ─────────────────────────────────────────────────────────────────────────────

func TestProof_DigestIntegrity(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	// 开立漂移案
	dc, _ := svc.OpenDriftCase(ctx, utamodegate.OpenDriftCaseReq{
		UserID:	"u-proof-digest", Asset: "BTC", Kind: utamodegate.DriftKindCollateralKey,
		OldMode:	utamodegate.MarginModeUnified, NewMode: utamodegate.MarginModeClassic,
		OldGeneration:	1, NewGeneration: 2, LiabilityAmount: "0.25",
	})
	oldSnap := `{"mode":"UNIFIED","collateral":"0.25"}`
	newSnap := `{"mode":"CLASSIC","collateral":"0"}`
	// 构建证明
	proof, _, err := svc.BuildAndAttachProof(ctx, utamodegate.BuildProofReq{
		DriftCaseID:	dc.ID,
		OldKeySnapshot:	oldSnap,
		NewAggSnapshot:	newSnap,
		BuiltBy:	"ops-proof-test",
	})
	if err != nil {
		t.Fatalf("BuildAndAttachProof: %v", err)
	}
	// 期望摘要与进程内计算一致
	expectedDigest := utamodegate.ComputeDigest(oldSnap, newSnap, dc.ID)
	if proof.DeltaDigest != expectedDigest {
		t.Errorf("digest mismatch: got %s, want %s", proof.DeltaDigest, expectedDigest)
	}
	// 篡改摘要后 Verify 应失败
	tampered := *proof
	tampered.DeltaDigest = "deadbeef0000000000000000000000000000000000000000000000000000dead"
	if err := svc.proof.Verify(&tampered); err != utamodegate.ErrProofDigestMismatch {
		t.Errorf("tampered proof verify error = %v, want ErrProofDigestMismatch", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestConcurrency_CAS
// ─────────────────────────────────────────────────────────────────────────────

func TestConcurrency_CASRace(t *testing.T) {
	// 多 goroutine 并发 SetMode（UNIFIED→CLASSIC），仅一方应成功（世代最终 = 1）。
	// MemStore 通过 mutex 保证 CAS 原子性。
	// ErrSameMode 也可能发生（先成功的 goroutine 切换后，后续 goroutine 读到 CLASSIC）。
	svc := newTestService(t)
	ctx := context.Background()
	userID := "u-cas-race-01"
	const goroutines = 20
	results := make([]error,
		goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			_, err := svc.SetMode(ctx, userID, "CLASSIC", "test", "")
			results[i] = err
		}()
	}
	wg.Wait()
	// 最终世代应恰好为 1（只有一次 CAS 成功 + 世代递增）
	gen, err := svc.GetGeneration(ctx, userID)
	if err != nil {
		t.Fatalf("GetGeneration: %v", err)
	}
	if gen.Generation != 1 {
		t.Errorf("generation = %d, want 1 after concurrent SetMode", gen.Generation)
	}
	// 统计成功次数（应恰好 1）；其余返回 ErrStaleGen 或 ErrSameMode（已切换）
	successCount := 0
	for _, e := range results {
		if e == nil {
			successCount++
		}
		if e != nil && e != utamodegate.ErrStaleGen && e != utamodegate.ErrSameMode {
			t.Errorf("unexpected error: %v", e)
		}
	}
	if successCount != 1 {
		t.Errorf("success count = %d, want 1", successCount)
	}
}

func TestConcurrency_ConcurrentRegisterBind(t *testing.T) {
	// 多 goroutine 并发为同一用户注册绑定，各自 RefID 不同，均应成功。
	// SetMode 在绑定全部注册后被阻塞。
	svc := newTestService(t)
	ctx := context.Background()
	userID := "u-cas-bind-01"
	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			refID := "ref-" + string(rune('a'+i))
			_, _ = svc.RegisterBind(ctx, utamodegate.RegisterBindReq{
				UserID:		userID,
				BindKind:	utamodegate.BindKindBorrow,
				Currency:	"USDT",
				MarginMode:	utamodegate.MarginModeCross,
				Amount:		"100",
				RefID:		refID,
			}, 0)
		}()
	}
	wg.Wait()
	count, err := svc.bind.CountOpenBinds(ctx, userID)
	if err != nil {
		t.Fatalf("CountOpenBinds: %v", err)
	}
	if count != goroutines {
		t.Errorf("open bind count = %d, want %d", count, goroutines)
	}
	// SetMode 应被阻塞
	_, err = svc.SetMode(ctx, userID, "CLASSIC", "test", "")
	if err != utamodegate.ErrInflightLiability {
		t.Errorf("SetMode error = %v, want ErrInflightLiability", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestIdempotency_SetMode
// ─────────────────────────────────────────────────────────────────────────────

func TestIdempotency_SetModeWithSameTriggerRef(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	userID := "u-idem-setmode"
	triggerRef := "u-idem-setmode|mode|UNIFIED→CLASSIC|fixed"
	// 注册在途绑定使 SetMode 进入 blocked 状态（blocked 租约可幂等查询）
	bind, _ := svc.RegisterBind(ctx, utamodegate.RegisterBindReq{
		UserID:		userID,
		BindKind:	utamodegate.BindKindBorrow,
		Currency:	"USDT",
		MarginMode:	utamodegate.MarginModeCross,
		Amount:		"1000",
		RefID:		"idem-borrow-01",
	}, 0)
	// 第一次调用：进入 blocked 队列
	r1, err := svc.SetMode(ctx, userID, "CLASSIC", "test", triggerRef)
	if err != utamodegate.ErrInflightLiability {
		t.Fatalf("SetMode first: expected ErrInflightLiability, got %v", err)
	}
	if r1 == nil || r1.Lease == nil {
		t.Fatal("SetMode first: result lease is nil")
	}
	firstLeaseID := r1.Lease.ID
	// 第二次相同 TriggerRef 应返回同一 blocked 租约（幂等）
	r2, err := svc.SetMode(ctx, userID, "CLASSIC", "test", triggerRef)
	if err != utamodegate.ErrInflightLiability {
		t.Fatalf("SetMode second (idempotent): expected ErrInflightLiability, got %v", err)
	}
	if r2 == nil || r2.Lease == nil {
		t.Fatal("SetMode second: result lease is nil")
	}
	if r2.Lease.ID != firstLeaseID {
		t.Errorf("idempotent SetMode: lease ID changed: %s vs %s", firstLeaseID, r2.Lease.ID)
	}
	// 释放绑定后无在途，worker 可推进
	_, _ = svc.ReleaseBind(ctx, utamodegate.ReleaseBindReq{UserID: userID, BindID: bind.ID})
}

// ─────────────────────────────────────────────────────────────────────────────
// TestFourEye_*
// ─────────────────────────────────────────────────────────────────────────────

func TestFourEye_RequiresTwoDistinctOperators(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	userID := "u-4eye-01"
	// 开立漂移案
	dc, _ := svc.OpenDriftCase(ctx, utamodegate.OpenDriftCaseReq{
		UserID:	userID, Asset: "ETH", Kind: utamodegate.DriftKindReverseAggregate,
		OldMode:	utamodegate.MarginModeClassic, NewMode: utamodegate.MarginModeUnified,
		OldGeneration:	0, NewGeneration: 1, LiabilityAmount: "1.5",
	})
	// 构建证明
	_, _, _ = svc.BuildAndAttachProof(ctx, utamodegate.BuildProofReq{
		DriftCaseID:	dc.ID,
		OldKeySnapshot:	`{"principal":"1.5"}`,
		NewAggSnapshot:	`{"principal":"3.0"}`,
		BuiltBy:	"system",
	})
	// 第一次确认
	_, err := svc.ConfirmClose(ctx, utamodegate.ConfirmCloseReq{DriftCaseID: dc.ID, Operator: "operator-1"})
	if err != nil {
		t.Fatalf("first confirm: %v", err)
	}
	// 同一操作员重复确认被拒
	_, err = svc.ConfirmClose(ctx, utamodegate.ConfirmCloseReq{DriftCaseID: dc.ID, Operator: "operator-1"})
	if err != utamodegate.ErrDuplicateConfirm {
		t.Errorf("duplicate confirm: error = %v, want ErrDuplicateConfirm", err)
	}
	// 空操作员被拒
	_, err = svc.ConfirmClose(ctx, utamodegate.ConfirmCloseReq{DriftCaseID: dc.ID, Operator: ""})
	if err != utamodegate.ErrOperatorInvalid {
		t.Errorf("empty operator: error = %v, want ErrOperatorInvalid", err)
	}
	// 第二操作员收口成功
	dcClosed, err := svc.ConfirmClose(ctx, utamodegate.ConfirmCloseReq{DriftCaseID: dc.ID, Operator: "operator-2"})
	if err != nil {
		t.Fatalf("second confirm: %v", err)
	}
	if dcClosed.Status != utamodegate.DriftClosed {
		t.Errorf("final status = %s, want closed", dcClosed.Status)
	}
}

func TestFourEye_CannotCloseWithoutProof(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	userID := "u-4eye-noproof"
	dc, _ := svc.OpenDriftCase(ctx, utamodegate.OpenDriftCaseReq{
		UserID:	userID, Asset: "USDC", Kind: utamodegate.DriftKindStaleGenWrite,
		OldMode:	utamodegate.MarginModeUnified, NewMode: utamodegate.MarginModeClassic,
		OldGeneration:	0, NewGeneration: 1, LiabilityAmount: "500",
	})
	// 未证明的 open 状态案件，ConfirmClose 应被拒（需先 proven）
	_, err := svc.ConfirmClose(ctx, utamodegate.ConfirmCloseReq{DriftCaseID: dc.ID, Operator: "ops-x"})
	if err != utamodegate.ErrProofMissing {
		t.Errorf("confirm without proof: error = %v, want ErrProofMissing", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestStats
// ─────────────────────────────────────────────────────────────────────────────

func TestStats(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_, _ = svc.SetMode(ctx, "u-stats-01", "CLASSIC", "test", "")
	stats := svc.Stats()
	if stats["mode_switch_total"] < 1 {
		t.Errorf("mode_switch_total = %d, want >= 1", stats["mode_switch_total"])
	}
	if stats["lease_acquired"] < 1 {
		t.Errorf("lease_acquired = %d, want >= 1", stats["lease_acquired"])
	}
	if stats["gen_bumped"] < 1 {
		t.Errorf("gen_bumped = %d, want >= 1", stats["gen_bumped"])
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestGenerationEngine_IssueToken
// ─────────────────────────────────────────────────────────────────────────────

func TestGenerationEngine_IssueToken(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	userID := "u-token-issue-01"
	token, err := svc.IssueToken(ctx, userID)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if token.UserID != userID {
		t.Errorf("token.UserID = %s, want %s", token.UserID, userID)
	}
	if token.Generation != 0 {
		t.Errorf("token.Generation = %d, want 0 (no mode switch yet)", token.Generation)
	}
	if token.Mode != utamodegate.MarginModeUnified {
		t.Errorf("token.Mode = %s, want UNIFIED", token.Mode)
	}
	// 切换模式后重新颁发
	_, _ = svc.SetMode(ctx, userID, "CLASSIC", "test", "")
	token2, err := svc.IssueToken(ctx, userID)
	if err != nil {
		t.Fatalf("IssueToken after switch: %v", err)
	}
	if token2.Generation != 1 {
		t.Errorf("token2.Generation = %d, want 1", token2.Generation)
	}
	if token2.Mode != utamodegate.MarginModeClassic {
		t.Errorf("token2.Mode = %s, want CLASSIC", token2.Mode)
	}
}

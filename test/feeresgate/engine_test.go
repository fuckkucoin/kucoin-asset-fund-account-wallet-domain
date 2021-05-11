package feeresgate_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/feereserve"
	"account-assets.kucoin.net/funding/internal/feeresgate"
)

// ══════════════════════════════════════════
// 辅助：固定时钟 + 快速 Engine 构建
// ══════════════════════════════════════════

func fixedClock(t time.Time) func() time.Time	{ return func() time.Time { return t } }

func newTestEngine(now time.Time) *feeresgate.Engine {
	return feeresgate.New(feeresgate.Config{
		Store:		feereserve.NewMemStore(),
		Clock:		fixedClock(now),
		LeaseTTL:	10 * time.Minute,
	})
}

func mustReserve(t *testing.T, eng *feeresgate.Engine, wid, uid, asset, fee string) feereserve.ReserveLease {
	t.Helper()
	l, err := eng.Reserve(context.Background(), feereserve.ReserveReq{
		WithdrawID:	wid, UserID: uid, Asset: asset, Fee: fee,
	})
	if err != nil {
		t.Fatalf("reserve %s: %v", wid, err)
	}
	return l
}

// ══════════════════════════════════════════
// TestReserveBasic 基本 Reserve 流程
// ══════════════════════════════════════════

func TestReserveBasic(t *testing.T) {
	now := time.Date(2021, 6, 22, 10, 0, 0, 0, time.UTC)
	eng := newTestEngine(now)
	ctx := context.Background()
	lease, err := eng.Reserve(ctx, feereserve.ReserveReq{
		WithdrawID:	"WD-001", UserID: "USER-A", Asset: "USDT", Fee: "10.5",
	})
	if err != nil {
		t.Fatal(err)
	}
	if lease.State != feereserve.LeaseReserved {
		t.Errorf("expected reserved, got %s", lease.State)
	}
	if lease.Generation != 1 {
		t.Errorf("expected generation 1, got %d", lease.Generation)
	}
	if lease.ReservedFee != "10.5" {
		t.Errorf("expected reserved_fee 10.5, got %s", lease.ReservedFee)
	}
	if lease.Token == "" {
		t.Error("expected non-empty token")
	}
}

// ══════════════════════════════════════════
// TestReserveBumpGeneration 重复 Reserve 升代
// ══════════════════════════════════════════

func TestReserveBumpGeneration(t *testing.T) {
	now := time.Date(2021, 6, 22, 10, 0, 0, 0, time.UTC)
	eng := newTestEngine(now)
	ctx := context.Background()
	l1, err := eng.Reserve(ctx, feereserve.ReserveReq{
		WithdrawID:	"WD-002", UserID: "USER-A", Asset: "USDT", Fee: "10.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if l1.Generation != 1 {
		t.Fatalf("expected gen 1, got %d", l1.Generation)
	}
	l2, err := eng.Reserve(ctx, feereserve.ReserveReq{
		WithdrawID:	"WD-002", UserID: "USER-A", Asset: "USDT", Fee: "12.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if l2.Generation != 2 {
		t.Errorf("expected gen 2, got %d", l2.Generation)
	}
	if l2.ReservedFee != "12" {
		t.Errorf("expected reserved_fee 12, got %s", l2.ReservedFee)
	}
	if l2.Token == l1.Token {
		t.Error("expected new token after bump")
	}
}

// ══════════════════════════════════════════
// Table-Driven: 结算释放竞态矩阵
// ══════════════════════════════════════════

func TestSettleReleaseMutex(t *testing.T) {
	type testCase struct {
		name		string
		firstLeg	string
		secondLeg	string
		firstVoucher	string
		secondVoucher	string
		wantErr		string
		wantState	feereserve.LeaseState
		wantCaseKind	feereserve.CaseKind
	}
	cases := []testCase{
		{
			name:		"settle_then_release_conflict",
			firstLeg:	"settle",
			secondLeg:	"release",
			firstVoucher:	"VCH-A",
			secondVoucher:	"VCH-B",
			wantErr:	"dual voucher",
			wantState:	feereserve.LeaseSettled,
			wantCaseKind:	feereserve.CaseDualVoucher,
		},
		{
			name:		"release_then_settle_conflict",
			firstLeg:	"release",
			secondLeg:	"settle",
			firstVoucher:	"VCH-C",
			secondVoucher:	"VCH-D",
			wantErr:	"dual voucher",
			wantState:	feereserve.LeaseReleased,
			wantCaseKind:	feereserve.CaseDualVoucher,
		},
		{
			name:		"settle_then_settle_idempotent",
			firstLeg:	"settle",
			secondLeg:	"settle",
			firstVoucher:	"VCH-E",
			secondVoucher:	"VCH-E",
			wantErr:	"",
			wantState:	feereserve.LeaseSettled,
		},
		{
			name:		"release_then_release_idempotent",
			firstLeg:	"release",
			secondLeg:	"release",
			firstVoucher:	"VCH-F",
			secondVoucher:	"VCH-F",
			wantErr:	"",
			wantState:	feereserve.LeaseReleased,
		},
		{
			name:		"settle_then_settle_diff_voucher",
			firstLeg:	"settle",
			secondLeg:	"settle",
			firstVoucher:	"VCH-G",
			secondVoucher:	"VCH-H",
			wantErr:	"dual voucher",
			wantState:	feereserve.LeaseSettled,
			wantCaseKind:	feereserve.CaseDualVoucher,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2021, 6, 22, 10, 0, 0, 0, time.UTC)
			eng := newTestEngine(now)
			ctx := context.Background()
			wid := "WD-" + tc.name
			lease := mustReserve(t, eng, wid, "USER-A", "USDT", "10.0")
			doFirst := func() error {
				if tc.firstLeg == "settle" {
					_, err := eng.Settle(ctx, feereserve.SettleReq{
						WithdrawID:	wid, LeaseID: lease.ID,
						Generation:	lease.Generation, Token: lease.Token,
						ActualFee:	"10.0", VoucherID: tc.firstVoucher,
					})
					return err
				}
				_, err := eng.Release(ctx, feereserve.ReleaseReq{
					WithdrawID:	wid,
					LeaseID:	lease.ID,
					Generation:	lease.Generation, Token: lease.Token,
					RefundAmt:	"10.0", VoucherID: tc.firstVoucher,
				})
				return err
			}
			doSecond := func() error {
				if tc.secondLeg == "settle" {
					_, err := eng.Settle(ctx, feereserve.SettleReq{
						WithdrawID:	wid, LeaseID: lease.ID,
						Generation:	lease.Generation, Token: lease.Token,
						ActualFee:	"10.0", VoucherID: tc.secondVoucher,
					})
					return err
				}
				_, err := eng.Release(ctx, feereserve.ReleaseReq{
					WithdrawID:	wid, LeaseID: lease.ID,
					Generation:	lease.Generation, Token: lease.Token,
					RefundAmt:	"10.0", VoucherID: tc.secondVoucher,
				})
				return err
			}
			if err := doFirst(); err != nil {
				t.Fatalf("first leg failed: %v", err)
			}
			err := doSecond()
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error %q, got %v", tc.wantErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			got, err := eng.GetLeaseByWithdraw(ctx, wid)
			if err != nil {
				t.Fatal(err)
			}
			if got.State != tc.wantState {
				t.Errorf("expected state %s, got %s", tc.wantState, got.State)
			}
			if tc.wantCaseKind != "" {
				cc, err := eng.CaseListByWithdraw(ctx, wid, 10)
				if err != nil {
					t.Fatal(err)
				}
				if len(cc) == 0 {
					t.Fatal("expected at least one case")
				}
				found := false
				for _, c := range cc {
					if c.Kind == tc.wantCaseKind {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected case kind %s, not found in %d cases", tc.wantCaseKind, len(cc))
				}
			}
		})
	}
}

// ══════════════════════════════════════════
// Table-Driven: 代次+令牌验证
// ══════════════════════════════════════════

func TestGenerationTokenValidation(t *testing.T) {
	type testCase struct {
		name	string
		gen	int64
		token	string
		expired	bool
		wantErr	string
	}
	cases := []testCase{
		{
			name:		"correct_gen_token",
			gen:		0,
			token:		"",
			wantErr:	"",
		},
		{
			name:		"stale_generation",
			gen:		999,
			token:		"bad-token",
			wantErr:	"generation mismatch",
		},
		{
			name:		"wrong_token",
			gen:		0,
			token:		"wrong-token",
			wantErr:	"generation mismatch",
		},
		{
			name:		"expired_lease",
			gen:		0,
			token:		"",
			expired:	true,
			wantErr:	"lease expired",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2021, 6, 22, 10, 0, 0, 0, time.UTC)
			eng := newTestEngine(now)
			ctx := context.Background()
			wid := "WD-GEN-" + tc.name
			lease := mustReserve(t, eng, wid, "USER-A", "USDT", "5.0")
			gen := lease.Generation
			tok := lease.Token
			if tc.gen != 0 {
				gen = tc.gen
			}
			if tc.token != "" {
				tok = tc.token
			}
			if tc.expired {
				eng.mu.Lock()
				eng.clock = fixedClock(now.Add(20 * time.Minute))
				eng.mu.Unlock()
			}
			_, err := eng.Settle(ctx, feereserve.SettleReq{
				WithdrawID:	wid, LeaseID: lease.ID,
				Generation:	gen, Token: tok,
				ActualFee:	"5.0", VoucherID: "VCH-GEN-" + tc.name,
			})
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected %q in error, got %v", tc.wantErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

// ══════════════════════════════════════════
// Table-Driven: Delta 差额检测
// ══════════════════════════════════════════

func TestDeltaDetection(t *testing.T) {
	type testCase struct {
		name		string
		reserved	string
		actual		string
		wantSign	string
		wantDelta	string
		wantCase	feereserve.CaseKind
		wantNoProof	bool
	}
	cases := []testCase{
		{
			name:		"exact_match_no_delta",
			reserved:	"10.0",
			actual:		"10",
			wantNoProof:	true,
		},
		{
			name:		"undercharge",
			reserved:	"10.0",
			actual:		"7.5",
			wantSign:	"undercharge",
			wantDelta:	"2.5",
			wantCase:	feereserve.CaseUndercharge,
		},
		{
			name:		"over_refund",
			reserved:	"5.0",
			actual:		"8.0",
			wantSign:	"over_refund",
			wantDelta:	"3",
			wantCase:	feereserve.CaseOverRefund,
		},
		{
			name:		"tiny_undercharge",
			reserved:	"0.001",
			actual:		"0.0005",
			wantSign:	"undercharge",
			wantDelta:	"0.0005",
			wantCase:	feereserve.CaseUndercharge,
		},
		{
			name:		"large_over_refund",
			reserved:	"100",
			actual:		"250",
			wantSign:	"over_refund",
			wantDelta:	"150",
			wantCase:	feereserve.CaseOverRefund,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2021, 6, 22, 10, 0, 0, 0, time.UTC)
			eng := newTestEngine(now)
			ctx := context.Background()
			wid := "WD-DELTA-" + tc.name
			lease := mustReserve(t, eng, wid, "USER-A", "USDT", tc.reserved)
			_, err := eng.Settle(ctx, feereserve.SettleReq{
				WithdrawID:	wid, LeaseID: lease.ID,
				Generation:	lease.Generation, Token: lease.Token,
				ActualFee:	tc.actual, VoucherID: "VCH-DELTA-" + tc.name,
			})
			if err != nil {
				t.Fatal(err)
			}
			proofs, err := eng.ProofExport(ctx, feereserve.ProofExportReq{WithdrawID: wid})
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantNoProof {
				if len(proofs) != 0 {
					t.Errorf("expected no proofs, got %d", len(proofs))
				}
				return
			}
			if len(proofs) != 1 {
				t.Fatalf("expected 1 proof, got %d", len(proofs))
			}
			p := proofs[0]
			if p.DeltaSign != tc.wantSign {
				t.Errorf("expected sign %s, got %s", tc.wantSign, p.DeltaSign)
			}
			if p.DeltaAmt != tc.wantDelta {
				t.Errorf("expected delta %s, got %s", tc.wantDelta, p.DeltaAmt)
			}
			cc, _ := eng.CaseListByWithdraw(ctx, wid, 10)
			if len(cc) == 0 {
				t.Fatal("expected at least one case")
			}
			if cc[0].Kind != tc.wantCase {
				t.Errorf("expected case kind %s, got %s", tc.wantCase, cc[0].Kind)
			}
		})
	}
}

// ══════════════════════════════════════════
// Table-Driven: 封口+阻断
// ══════════════════════════════════════════

func TestSealAndBlock(t *testing.T) {
	now := time.Date(2021, 6, 22, 10, 0, 0, 0, time.UTC)
	eng := newTestEngine(now)
	ctx := context.Background()
	lease := mustReserve(t, eng, "WD-SEAL", "USER-A", "USDT", "10.0")
	_, err := eng.Settle(ctx, feereserve.SettleReq{
		WithdrawID:	"WD-SEAL", LeaseID: lease.ID,
		Generation:	lease.Generation, Token: lease.Token,
		ActualFee:	"8.0", VoucherID: "VCH-SEAL",
	})
	if err != nil {
		t.Fatal(err)
	}
	proofs, _ := eng.ProofExport(ctx, feereserve.ProofExportReq{WithdrawID: "WD-SEAL"})
	if len(proofs) != 1 {
		t.Fatalf("expected 1 proof, got %d", len(proofs))
	}
	sealed, err := eng.SealProof(ctx, proofs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !sealed.Sealed {
		t.Error("expected sealed=true")
	}
	got, err := eng.GetLeaseByWithdraw(ctx, "WD-SEAL")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != feereserve.LeaseBlocked {
		t.Errorf("expected blocked, got %s", got.State)
	}
	if !strings.Contains(got.BlockReason, "proof_sealed") {
		t.Errorf("expected block_reason to contain proof_sealed, got %s", got.BlockReason)
	}
}

// ══════════════════════════════════════════
// Table-Driven: Case 状态机转移
// ══════════════════════════════════════════

func TestCaseStateMachine(t *testing.T) {
	type testCase struct {
		name		string
		steps		[]string
		wantState	feereserve.CaseState
		wantErrAt	int
		wantErr		string
	}
	cases := []testCase{
		{
			name:		"open_to_proven",
			steps:		[]string{"prove"},
			wantState:	feereserve.CaseProven,
		},
		{
			name:		"open_to_proven_to_closed",
			steps:		[]string{"prove", "close"},
			wantState:	feereserve.CaseClosed,
		},
		{
			name:		"open_then_close_via_two_steps",
			steps:		[]string{"prove", "close"},
			wantState:	feereserve.CaseClosed,
		},
		{
			name:		"proven_then_close_then_prove_fails",
			steps:		[]string{"prove", "close", "prove"},
			wantState:	feereserve.CaseClosed,
			wantErrAt:	2,
			wantErr:	"case already closed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2021,
				6, 22, 10, 0, 0, 0, time.UTC)
			eng := newTestEngine(now)
			ctx := context.Background()
			wid := "WD-CASE-" + tc.name
			lease := mustReserve(t, eng, wid, "USER-A", "USDT", "10.0")
			_, _ = eng.Settle(ctx, feereserve.SettleReq{
				WithdrawID:	wid, LeaseID: lease.ID,
				Generation:	lease.Generation,
				Token:		lease.Token,
				ActualFee:	"8.0", VoucherID: "VCH-CASE-" + tc.name,
			})
			cc, _ := eng.CaseListByWithdraw(ctx, wid, 10)
			if len(cc) == 0 {
				t.Fatal("expected at least one case from delta")
			}
			caseID := cc[0].ID
			for i, step := range tc.steps {
				_, err := eng.CaseConfirm(ctx, feereserve.CaseConfirmReq{
					CaseID:		caseID,
					Resolution:	"step_" + step,
				})
				if tc.wantErrAt == i && tc.wantErr != "" {
					if err == nil {
						t.Fatalf("step %d: expected error %q", i, tc.wantErr)
					}
					if !strings.Contains(err.Error(), tc.wantErr) {
						t.Fatalf("step %d: expected %q, got %v", i, tc.wantErr, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("step %d: unexpected error: %v", i, err)
				}
			}
			rc, err := eng.store.GetCase(ctx, caseID)
			if err != nil {
				t.Fatal(err)
			}
			if rc.State != tc.wantState {
				t.Errorf("expected case state %s, got %s", tc.wantState, rc.State)
			}
		})
	}
}

// ══════════════════════════════════════════
// Table-Driven: 操作已终态的租约
// ══════════════════════════════════════════

func TestOperateAfterTerminal(t *testing.T) {
	type testCase struct {
		name		string
		terminalLeg	string
		incomingLeg	string
		sameVoucher	bool
		wantErr		string
	}
	cases := []testCase{
		{
			name:		"settled_then_release_diff_voucher",
			terminalLeg:	"settle",
			incomingLeg:	"release",
			sameVoucher:	false,
			wantErr:	"dual voucher",
		},
		{
			name:		"released_then_settle_diff_voucher",
			terminalLeg:	"release",
			incomingLeg:	"settle",
			sameVoucher:	false,
			wantErr:	"dual voucher",
		},
		{
			name:		"settled_then_settle_same_voucher_idempotent",
			terminalLeg:	"settle",
			incomingLeg:	"settle",
			sameVoucher:	true,
			wantErr:	"",
		},
		{
			name:		"released_then_release_same_voucher_idempotent",
			terminalLeg:	"release",
			incomingLeg:	"release",
			sameVoucher:	true,
			wantErr:	"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2021, 6, 22, 10, 0, 0, 0, time.UTC)
			eng := newTestEngine(now)
			ctx := context.Background()
			wid := "WD-AFTER-" + tc.name
			lease := mustReserve(t, eng, wid, "USER-A", "USDT", "10.0")
			voucherFirst := "VCH-FIRST-" + tc.name
			if tc.terminalLeg == "settle" {
				_, err := eng.Settle(ctx, feereserve.SettleReq{
					WithdrawID:	wid, LeaseID: lease.ID,
					Generation:	lease.Generation, Token: lease.Token,
					ActualFee:	"10.0", VoucherID: voucherFirst,
				})
				if err != nil {
					t.Fatal(err)
				}
			} else {
				_, err := eng.Release(ctx, feereserve.ReleaseReq{
					WithdrawID:	wid, LeaseID: lease.ID,
					Generation:	lease.Generation, Token: lease.Token,
					RefundAmt:	"10.0", VoucherID: voucherFirst,
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			voucherSecond := "VCH-SECOND-" + tc.name
			if tc.sameVoucher {
				voucherSecond = voucherFirst
			}
			var err error
			if tc.incomingLeg == "settle" {
				_, err = eng.Settle(ctx, feereserve.SettleReq{
					WithdrawID:	wid,
					LeaseID:	lease.ID,
					Generation:	lease.Generation, Token: lease.Token,
					ActualFee:	"10.0", VoucherID: voucherSecond,
				})
			} else {
				_, err = eng.Release(ctx, feereserve.ReleaseReq{
					WithdrawID:	wid, LeaseID: lease.ID,
					Generation:	lease.Generation,
					Token:		lease.Token,
					RefundAmt:	"10.0", VoucherID: voucherSecond,
				})
			}
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected %q, got %v", tc.wantErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

// ══════════════════════════════════════════
// Table-Driven: 隔离性——不同 WithdrawID 互不影响
// ══════════════════════════════════════════

func TestIsolation(t *testing.T) {
	now := time.Date(2021, 6, 22, 10, 0, 0, 0, time.UTC)
	eng := newTestEngine(now)
	ctx := context.Background()
	leaseA := mustReserve(t, eng, "WD-A", "USER-X", "USDT", "5.0")
	leaseB := mustReserve(t, eng, "WD-B", "USER-Y", "BTC", "0.0001")
	_, err := eng.Settle(ctx, feereserve.SettleReq{
		WithdrawID:	"WD-A", LeaseID: leaseA.ID,
		Generation:	leaseA.Generation,
		Token:		leaseA.Token,
		ActualFee:	"5.0", VoucherID: "VCH-A",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = eng.Release(ctx, feereserve.ReleaseReq{
		WithdrawID:	"WD-B", LeaseID: leaseB.ID,
		Generation:	leaseB.Generation,
		Token:		leaseB.Token,
		RefundAmt:	"0.0001", VoucherID: "VCH-B",
	})
	if err != nil {
		t.Fatal(err)
	}
	gotA, _ := eng.GetLeaseByWithdraw(ctx, "WD-A")
	gotB, _ := eng.GetLeaseByWithdraw(ctx, "WD-B")
	if gotA.State != feereserve.LeaseSettled {
		t.Errorf("WD-A expected settled, got %s", gotA.State)
	}
	if gotB.State != feereserve.LeaseReleased {
		t.Errorf("WD-B expected released, got %s", gotB.State)
	}
}

// ══════════════════════════════════════════
// Table-Driven: Block 操作
// ══════════════════════════════════════════

func TestBlockOperations(t *testing.T) {
	type testCase struct {
		name		string
		preSettle	bool
		blockReason	string
		wantState	feereserve.LeaseState
		doubleBlock	bool
	}
	cases := []testCase{
		{
			name:		"block_reserved",
			preSettle:	false,
			blockReason:	"manual_block",
			wantState:	feereserve.LeaseBlocked,
		},
		{
			name:		"block_settled",
			preSettle:	true,
			blockReason:	"audit_review",
			wantState:	feereserve.LeaseBlocked,
		},
		{
			name:		"double_block_idempotent",
			preSettle:	false,
			blockReason:	"first_block",
			wantState:	feereserve.LeaseBlocked,
			doubleBlock:	true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2021, 6, 22, 10, 0, 0, 0, time.UTC)
			eng := newTestEngine(now)
			ctx := context.Background()
			wid := "WD-BLK-" + tc.name
			lease := mustReserve(t, eng, wid, "USER-A", "USDT", "10.0")
			if tc.preSettle {
				_, _ = eng.Settle(ctx, feereserve.SettleReq{
					WithdrawID:	wid, LeaseID: lease.ID,
					Generation:	lease.Generation, Token: lease.Token,
					ActualFee:	"10.0", VoucherID: "VCH-BLK-" + tc.name,
				})
			}
			got, err := eng.Block(ctx, feereserve.BlockReq{
				WithdrawID:	wid, LeaseID: lease.ID,
				Reason:	tc.blockReason,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.State != tc.wantState {
				t.Errorf("expected %s, got %s", tc.wantState, got.State)
			}
			if tc.doubleBlock {
				got2, err := eng.Block(ctx, feereserve.BlockReq{
					WithdrawID:	wid, LeaseID: lease.ID,
					Reason:	"second_block",
				})
				if err != nil {
					t.Fatal(err)
				}
				if got2.State != feereserve.LeaseBlocked {
					t.Errorf("double block: expected blocked, got %s", got2.State)
				}
			}
		})
	}
}

// ══════════════════════════════════════════
// Table-Driven: 输入校验
// ══════════════════════════════════════════

func TestInputValidation(t *testing.T) {
	type testCase struct {
		name	string
		req	feereserve.ReserveReq
		wantErr	string
	}
	cases := []testCase{
		{
			name:		"empty_withdraw_id",
			req:		feereserve.ReserveReq{WithdrawID: "", UserID: "USER-A", Asset: "USDT", Fee: "10.0"},
			wantErr:	"withdraw_id",
		},
		{
			name:		"empty_user_id",
			req:		feereserve.ReserveReq{WithdrawID: "WD-V1", UserID: "", Asset: "USDT", Fee: "10.0"},
			wantErr:	"withdraw_id",
		},
		{
			name:		"empty_asset",
			req:		feereserve.ReserveReq{WithdrawID: "WD-V2", UserID: "USER-A", Asset: "", Fee: "10.0"},
			wantErr:	"asset",
		},
		{
			name:		"zero_fee",
			req:		feereserve.ReserveReq{WithdrawID: "WD-V3", UserID: "USER-A", Asset: "USDT", Fee: "0"},
			wantErr:	"amount",
		},
		{
			name:		"negative_fee",
			req:		feereserve.ReserveReq{WithdrawID: "WD-V4", UserID: "USER-A", Asset: "USDT", Fee: "-5"},
			wantErr:	"amount",
		},
		{
			name:		"invalid_fee_format",
			req:		feereserve.ReserveReq{WithdrawID: "WD-V5", UserID: "USER-A", Asset: "USDT", Fee: "abc"},
			wantErr:	"amount",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2021, 6, 22, 10, 0, 0, 0, time.UTC)
			eng := newTestEngine(now)
			_, err := eng.Reserve(context.Background(), tc.req)
			if err == nil {
				t.Fatalf("expected error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected %q in error, got %v", tc.wantErr, err)
			}
		})
	}
}

// ══════════════════════════════════════════
// Table-Driven: Settle 输入校验
// ══════════════════════════════════════════

func TestSettleInputValidation(t *testing.T) {
	type testCase struct {
		name	string
		mod	func(*feereserve.SettleReq)
		wantErr	string
	}
	cases := []testCase{
		{
			name:		"empty_withdraw_id",
			mod:		func(r *feereserve.SettleReq) { r.WithdrawID = "" },
			wantErr:	"withdraw_id",
		},
		{
			name:		"empty_lease_id",
			mod:		func(r *feereserve.SettleReq) { r.LeaseID = "" },
			wantErr:	"withdraw_id",
		},
		{
			name:		"empty_voucher",
			mod:		func(r *feereserve.SettleReq) { r.VoucherID = "" },
			wantErr:	"request invalid",
		},
		{
			name:		"zero_actual_fee",
			mod:		func(r *feereserve.SettleReq) { r.ActualFee = "0" },
			wantErr:	"amount",
		},
		{
			name:		"negative_actual_fee",
			mod:		func(r *feereserve.SettleReq) { r.ActualFee = "-1" },
			wantErr:	"amount",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2021, 6, 22, 10, 0, 0, 0, time.UTC)
			eng := newTestEngine(now)
			ctx := context.Background()
			lease := mustReserve(t, eng, "WD-SV-"+tc.name, "USER-A", "USDT", "10.0")
			req := feereserve.SettleReq{
				WithdrawID:	"WD-SV-" + tc.name,
				LeaseID:	lease.ID,
				Generation:	lease.Generation,
				Token:		lease.Token,
				ActualFee:	"10.0",
				VoucherID:	"VCH-SV",
			}
			tc.mod(&req)
			_, err := eng.Settle(ctx, req)
			if err == nil {
				t.Fatalf("expected error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// ══════════════════════════════════════════
// Table-Driven: Release 输入校验
// ══════════════════════════════════════════

func TestReleaseInputValidation(t *testing.T) {
	type testCase struct {
		name	string
		mod	func(*feereserve.ReleaseReq)
		wantErr	string
	}
	cases := []testCase{
		{
			name:		"empty_withdraw_id",
			mod:		func(r *feereserve.ReleaseReq) { r.WithdrawID = "" },
			wantErr:	"withdraw_id",
		},
		{
			name:		"empty_lease_id",
			mod:		func(r *feereserve.ReleaseReq) { r.LeaseID = "" },
			wantErr:	"withdraw_id",
		},
		{
			name:		"empty_voucher",
			mod:		func(r *feereserve.ReleaseReq) { r.VoucherID = "" },
			wantErr:	"request invalid",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2021, 6, 22, 10, 0, 0, 0, time.UTC)
			eng := newTestEngine(now)
			ctx := context.Background()
			lease := mustReserve(t, eng, "WD-RV-"+tc.name, "USER-A", "USDT", "10.0")
			req := feereserve.ReleaseReq{
				WithdrawID:	"WD-RV-" + tc.name,
				LeaseID:	lease.ID,
				Generation:	lease.Generation,
				Token:		lease.Token,
				RefundAmt:	"10.0",
				VoucherID:	"VCH-RV",
			}
			tc.mod(&req)
			_, err := eng.Release(ctx, req)
			if err == nil {
				t.Fatalf("expected error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// ══════════════════════════════════════════
// Table-Driven: Worker 功能
// ══════════════════════════════════════════

func TestTerminalGapPatrolJob(t *testing.T) {
	now := time.Date(2021, 6, 22, 10, 0, 0, 0, time.UTC)
	eng := feeresgate.New(feeresgate.Config{
		Store:		feereserve.NewMemStore(),
		Clock:		fixedClock(now),
		LeaseTTL:	5 * time.Minute,
	})
	ctx := context.Background()
	mustReserve(t, eng, "WD-GAP-1", "USER-A", "USDT", "10.0")
	mustReserve(t, eng, "WD-GAP-2", "USER-B", "BTC", "0.001")
	eng.mu.Lock()
	eng.clock = fixedClock(now.Add(40 * time.Minute))
	eng.mu.Unlock()
	job := feeresgate.TerminalGapPatrolJob(feeresgate.TerminalGapPatrolConfig{
		Engine:		eng,
		GapThreshold:	30 * time.Minute,
		BatchSize:	50,
	})
	if err := job(ctx); err != nil {
		t.Fatal(err)
	}
	for _, wid := range []string{"WD-GAP-1", "WD-GAP-2"} {
		lease, err := eng.GetLeaseByWithdraw(ctx, wid)
		if err != nil {
			t.Fatal(err)
		}
		if lease.State != feereserve.LeaseBlocked {
			t.Errorf("%s: expected blocked, got %s", wid, lease.State)
		}
		cc, _ := eng.CaseListByWithdraw(ctx, wid, 10)
		if len(cc) == 0 {
			t.Errorf("%s: expected terminal_gap case", wid)
		} else if cc[0].Kind != feereserve.CaseTerminalGap {
			t.Errorf("%s: expected terminal_gap, got %s", wid, cc[0].Kind)
		}
	}
}

func TestUnderchargeScanJob(t *testing.T) {
	now := time.Date(2021, 6, 22, 10, 0, 0, 0, time.UTC)
	eng := newTestEngine(now)
	ctx := context.Background()
	lease := mustReserve(t, eng, "WD-USCAN", "USER-A", "USDT", "10.0")
	_, _ = eng.Settle(ctx, feereserve.SettleReq{
		WithdrawID:	"WD-USCAN", LeaseID: lease.ID,
		Generation:	lease.Generation, Token: lease.Token,
		ActualFee:	"7.0", VoucherID: "VCH-USCAN",
	})
	proofs, _ := eng.ProofExport(ctx, feereserve.ProofExportReq{WithdrawID: "WD-USCAN"})
	for _, p := range proofs {
		_, _ = eng.SealProof(ctx, p.ID)
	}
	job := feeresgate.UnderchargeScanJob(feeresgate.UnderchargeScanConfig{Engine: eng, BatchSize: 50})
	if err := job(ctx); err != nil {
		t.Fatal(err)
	}
	cc, _ := eng.CaseListByWithdraw(ctx, "WD-USCAN", 10)
	foundProven := false
	for _, c := range cc {
		if c.Kind == feereserve.CaseUndercharge && c.State == feereserve.CaseProven {
			foundProven = true
		}
	}
	if !foundProven {
		t.Error("expected undercharge case to be proven after scan")
	}
}

func TestProofSealJob(t *testing.T) {
	now := time.Date(2021, 6, 22, 10, 0, 0, 0, time.UTC)
	eng := newTestEngine(now)
	ctx := context.Background()
	lease := mustReserve(t, eng, "WD-PSEAL", "USER-A", "USDT", "10.0")
	_, _ = eng.Settle(ctx, feereserve.SettleReq{
		WithdrawID:	"WD-PSEAL", LeaseID: lease.ID,
		Generation:	lease.Generation,
		Token:		lease.Token,
		ActualFee:	"6.0", VoucherID: "VCH-PSEAL",
	})
	cc, _ := eng.CaseListByWithdraw(ctx, "WD-PSEAL", 10)
	if len(cc) == 0 {
		t.Fatal("expected case")
	}
	_, _ = eng.CaseConfirm(ctx, feereserve.CaseConfirmReq{
		CaseID:	cc[0].ID, Resolution: "confirmed",
	})
	job := feeresgate.ProofSealJob(feeresgate.ProofSealConfig{Engine: eng,
		BatchSize:	50})
	if err := job(ctx); err != nil {
		t.Fatal(err)
	}
	proofs, _ := eng.ProofExport(ctx, feereserve.ProofExportReq{WithdrawID: "WD-PSEAL"})
	for _, p := range proofs {
		if !p.Sealed {
			t.Errorf("expected proof %s to be sealed", p.ID)
		}
	}
}

// ══════════════════════════════════════════
// Golden Test: 加载 testdata JSON 验证结构
// ══════════════════════════════════════════

type goldenAction struct {
	Type string `json:"type"`
}

type goldenCase struct {
	Name		string		`json:"name"`
	Description	string		`json:"description"`
	Actions		[]goldenAction	`json:"actions"`
	Expected	map[string]any	`json:"expected"`
}

func TestGoldenFilesLoadable(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "feeresgate")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("testdata dir not found: %v", err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			var gc goldenCase
			if err := json.Unmarshal(data, &gc); err != nil {
				t.Fatal(err)
			}
			if gc.Name == "" {
				t.Error("golden case missing name")
			}
			if gc.Description == "" {
				t.Error("golden case missing description")
			}
			if len(gc.Actions) == 0 {
				t.Error("golden case missing actions")
			}
			if gc.Expected == nil {
				t.Error("golden case missing expected")
			}
		})
	}
}

// ══════════════════════════════════════════
// Nil Engine 安全
// ══════════════════════════════════════════

func TestNilEngineSafety(t *testing.T) {
	var eng *feeresgate.Engine
	ctx := context.Background()
	_, err := eng.Reserve(ctx, feereserve.ReserveReq{})
	if err != feeresgate.ErrEngineNil {
		t.Errorf("expected ErrEngineNil, got %v", err)
	}
	_, err = eng.Settle(ctx, feereserve.SettleReq{})
	if err != feeresgate.ErrEngineNil {
		t.Errorf("expected ErrEngineNil, got %v", err)
	}
	_, err = eng.Release(ctx, feereserve.ReleaseReq{})
	if err != feeresgate.ErrEngineNil {
		t.Errorf("expected ErrEngineNil, got %v", err)
	}
	_, err = eng.Block(ctx, feereserve.BlockReq{})
	if err != feeresgate.ErrEngineNil {
		t.Errorf("expected ErrEngineNil, got %v", err)
	}
	_, err = eng.GetLease(ctx, "x")
	if err != feeresgate.ErrEngineNil {
		t.Errorf("expected ErrEngineNil, got %v", err)
	}
	_, err = eng.CaseConfirm(ctx, feereserve.CaseConfirmReq{})
	if err != feeresgate.ErrEngineNil {
		t.Errorf("expected ErrEngineNil, got %v", err)
	}
	_, err = eng.ProofExport(ctx, feereserve.ProofExportReq{})
	if err != feeresgate.ErrEngineNil {
		t.Errorf("expected ErrEngineNil, got %v", err)
	}
	_, err = eng.SealProof(ctx, "x")
	if err != feeresgate.ErrEngineNil {
		t.Errorf("expected ErrEngineNil, got %v", err)
	}
}

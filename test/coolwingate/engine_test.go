package coolwingate_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/coolwingate"
)

// ──────────────────────────────────────────
// Mock Ledger
// ──────────────────────────────────────────

type mockLedger struct {
	lockUntil	time.Time
	reason		string
	err		error
}

func (m *mockLedger) WithdrawLockUntil(_ context.Context, _ string) (time.Time, string, error) {
	return m.lockUntil, m.reason, m.err
}

type mockSecMark struct {
	marks	[]string
	err	error
}

func (m *mockSecMark) Mark(_ context.Context, userID, reason string) error {
	if m.err != nil {
		return m.err
	}
	m.marks = append(m.marks, userID+":"+reason)
	return nil
}

func newTestEngine(ledger coolwingate.LedgerQuerier, secmark coolwingate.SecurityMarker, now time.Time) *coolwingate.Engine {
	return coolwingate.New(coolwingate.Config{
		Store:		coolwingate.NewMemStore(),
		Ledger:		ledger,
		SecMark:	secmark,
		Clock:		func() time.Time { return now },
	})
}

// ──────────────────────────────────────────
// TestMarkCoolDown 冷却标记 table-driven
// ──────────────────────────────────────────

func TestMarkCoolDown(t *testing.T) {
	baseTime := time.Date(2021, 8, 17, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		userID		string
		reason		string
		prevMarks	int
		wantGen		int64
		wantState	coolwingate.LeaseState
		wantErr		string
	}{
		{
			name:		"trading_password_first_mark",
			userID:		"USER-001",
			reason:		"trading_password",
			prevMarks:	0,
			wantGen:	1,
			wantState:	coolwingate.LeaseOpen,
		},
		{
			name:		"google_2fa_second_mark",
			userID:		"USER-001",
			reason:		"google_2fa",
			prevMarks:	1,
			wantGen:	2,
			wantState:	coolwingate.LeaseOpen,
		},
		{
			name:		"phone_change_mark",
			userID:		"USER-002",
			reason:		"phone",
			prevMarks:	0,
			wantGen:	1,
			wantState:	coolwingate.LeaseOpen,
		},
		{
			name:		"email_change_mark",
			userID:		"USER-003",
			reason:		"email",
			prevMarks:	0,
			wantGen:	1,
			wantState:	coolwingate.LeaseOpen,
		},
		{
			name:		"passkey_delete_mark",
			userID:		"USER-004",
			reason:		"passkey_delete",
			prevMarks:	0,
			wantGen:	1,
			wantState:	coolwingate.LeaseOpen,
		},
		{
			name:		"login_password_mark",
			userID:		"USER-005",
			reason:		"login_password",
			prevMarks:	0,
			wantGen:	1,
			wantState:	coolwingate.LeaseOpen,
		},
		{
			name:		"security_change_generic",
			userID:		"USER-006",
			reason:		"security_change",
			prevMarks:	0,
			wantGen:	1,
			wantState:	coolwingate.LeaseOpen,
		},
		{
			name:		"normalize_2fa_alias",
			userID:		"USER-007",
			reason:		"2fa",
			prevMarks:	0,
			wantGen:	1,
			wantState:	coolwingate.LeaseOpen,
		},
		{
			name:		"empty_user_id_rejected",
			userID:		"",
			reason:		"phone",
			wantErr:	"user",
		},
		{
			name:		"empty_reason_rejected",
			userID:		"USER-008",
			reason:		"",
			wantErr:	"reason",
		},
		{
			name:		"triple_mark_gen_3",
			userID:		"USER-009",
			reason:		"unfreeze",
			prevMarks:	2,
			wantGen:	3,
			wantState:	coolwingate.LeaseOpen,
		},
		{
			name:		"whitespace_user_rejected",
			userID:		"   ",
			reason:		"phone",
			wantErr:	"user",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := &mockSecMark{}
			eng := newTestEngine(nil, sm, baseTime)
			ctx := context.Background()
			for i := 0; i < tt.prevMarks; i++ {
				_, _ = eng.MarkCoolDown(ctx, coolwingate.MarkReq{
					UserID:	tt.userID,
					Reason:	fmt.Sprintf("prev_mark_%d", i),
				})
			}
			lease, err := eng.MarkCoolDown(ctx, coolwingate.MarkReq{
				UserID:	tt.userID,
				Reason:	tt.reason,
			})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if lease.CoolGen != tt.wantGen {
				t.Errorf("cool_gen: want %d, got %d", tt.wantGen, lease.CoolGen)
			}
			if lease.State != tt.wantState {
				t.Errorf("state: want %s, got %s", tt.wantState, lease.State)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestBindPreflight 预检绑定 table-driven
// ──────────────────────────────────────────

func TestBindPreflight(t *testing.T) {
	baseTime := time.Date(2021, 8, 17, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		userID		string
		withdrawID	string
		lockActive	bool
		lockReason	string
		prevBind	bool
		wantState	coolwingate.BindState
		wantErr		string
	}{
		{
			name:		"normal_bind",
			userID:		"USER-001",
			withdrawID:	"WD-001",
			wantState:	coolwingate.BindOpen,
		},
		{
			name:		"lock_window_active_rejected",
			userID:		"USER-002",
			withdrawID:	"WD-002",
			lockActive:	true,
			lockReason:	"phone",
			wantErr:	"lock window active",
		},
		{
			name:		"empty_user_rejected",
			userID:		"",
			wantErr:	"user",
		},
		{
			name:		"empty_withdraw_rejected",
			userID:		"USER-003",
			withdrawID:	"",
			wantErr:	"withdraw_id",
		},
		{
			name:		"duplicate_bind_rejected",
			userID:		"USER-004",
			withdrawID:	"WD-004",
			prevBind:	true,
			wantErr:	"duplicate",
		},
		{
			name:		"no_lock_bind_succeeds",
			userID:		"USER-005",
			withdrawID:	"WD-005",
			lockActive:	false,
			wantState:	coolwingate.BindOpen,
		},
		{
			name:		"2fa_lock_rejected",
			userID:		"USER-006",
			withdrawID:	"WD-006",
			lockActive:	true,
			lockReason:	"google_2fa",
			wantErr:	"lock window active",
		},
		{
			name:		"email_lock_rejected",
			userID:		"USER-007",
			withdrawID:	"WD-007",
			lockActive:	true,
			lockReason:	"email",
			wantErr:	"lock window active",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var led coolwingate.LedgerQuerier
			if tt.lockActive {
				led = &mockLedger{
					lockUntil:	baseTime.Add(24 * time.Hour),
					reason:		tt.lockReason,
				}
			}
			eng := newTestEngine(led, nil, baseTime)
			ctx := context.Background()
			if tt.prevBind && tt.withdrawID != "" {
				_, _ = eng.BindPreflight(ctx, coolwingate.PreflightBindReq{
					UserID:		tt.userID,
					WithdrawID:	tt.withdrawID,
				})
			}
			bind, err := eng.BindPreflight(ctx, coolwingate.PreflightBindReq{
				UserID:		tt.userID,
				WithdrawID:	tt.withdrawID,
			})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if bind.State != tt.wantState {
				t.Errorf("state: want %s, got %s", tt.wantState, bind.State)
			}
			if bind.Token == "" {
				t.Error("expected non-empty token")
			}
		})
	}
}

// ──────────────────────────────────────────
// TestCheckBroadcast 广播校验 table-driven
// ──────────────────────────────────────────

func TestCheckBroadcast(t *testing.T) {
	baseTime := time.Date(2021, 8, 17, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		setupMark	bool
		markReason	string
		bindExpired	bool
		wantErr		string
	}{
		{
			name:		"happy_path_consumed",
			wantErr:	"",
		},
		{
			name:		"mark_after_preflight_revoked",
			setupMark:	true,
			markReason:	"trading_password",
			wantErr:	"broadcast blocked",
		},
		{
			name:		"phone_mark_revoked",
			setupMark:	true,
			markReason:	"phone",
			wantErr:	"broadcast blocked",
		},
		{
			name:		"2fa_mark_revoked",
			setupMark:	true,
			markReason:	"google_2fa",
			wantErr:	"broadcast blocked",
		},
		{
			name:		"email_mark_revoked",
			setupMark:	true,
			markReason:	"email",
			wantErr:	"broadcast blocked",
		},
		{
			name:		"passkey_mark_revoked",
			setupMark:	true,
			markReason:	"passkey_delete",
			wantErr:	"broadcast blocked",
		},
		{
			name:		"login_pw_mark_revoked",
			setupMark:	true,
			markReason:	"login_password",
			wantErr:	"broadcast blocked",
		},
		{
			name:		"expired_bind_revoked",
			bindExpired:	true,
			wantErr:	"bind expired",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := baseTime
			eng := newTestEngine(nil, &mockSecMark{}, now)
			ctx := context.Background()
			userID := "USER-BC-" + tt.name
			withdrawID := "WD-BC-" + tt.name
			bind, err := eng.BindPreflight(ctx, coolwingate.PreflightBindReq{
				UserID:		userID,
				WithdrawID:	withdrawID,
			})
			if err != nil {
				t.Fatalf("bind preflight: %v", err)
			}
			if tt.setupMark {
				_, err := eng.MarkCoolDown(ctx, coolwingate.MarkReq{
					UserID:	userID,
					Reason:	tt.markReason,
				})
				if err != nil {
					t.Fatalf("mark: %v", err)
				}
			}
			if tt.bindExpired {
				eng.mu.Lock()
				eng.clock = func() time.Time { return baseTime.Add(20 * time.Minute) }
				eng.mu.Unlock()
			}
			err = eng.CheckBroadcast(ctx, coolwingate.BroadcastCheckReq{
				UserID:		userID,
				WithdrawID:	withdrawID,
				BindToken:	bind.Token,
			})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestRequireWithdraw 出金锁窗口 table-driven
// ──────────────────────────────────────────

func TestRequireWithdraw(t *testing.T) {
	baseTime := time.Date(2021, 8, 17, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		userID		string
		lockActive	bool
		lockReason	string
		wantAllow	bool
		wantErr		string
	}{
		{
			name:		"no_lock_allowed",
			userID:		"USER-RW-001",
			wantAllow:	true,
		},
		{
			name:		"phone_lock_blocked",
			userID:		"USER-RW-002",
			lockActive:	true,
			lockReason:	"phone",
			wantAllow:	false,
		},
		{
			name:		"2fa_lock_blocked",
			userID:		"USER-RW-003",
			lockActive:	true,
			lockReason:	"google_2fa",
			wantAllow:	false,
		},
		{
			name:		"email_lock_blocked",
			userID:		"USER-RW-004",
			lockActive:	true,
			lockReason:	"email",
			wantAllow:	false,
		},
		{
			name:		"unfreeze_lock_blocked",
			userID:		"USER-RW-005",
			lockActive:	true,
			lockReason:	"unfreeze",
			wantAllow:	false,
		},
		{
			name:		"trading_password_lock_blocked",
			userID:		"USER-RW-006",
			lockActive:	true,
			lockReason:	"trading_password",
			wantAllow:	false,
		},
		{
			name:		"empty_user_rejected",
			userID:		"",
			wantErr:	"user",
		},
		{
			name:		"nil_ledger_allowed",
			userID:		"USER-RW-007",
			wantAllow:	true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var led coolwingate.LedgerQuerier
			if tt.lockActive {
				led = &mockLedger{
					lockUntil:	baseTime.Add(24 * time.Hour),
					reason:		tt.lockReason,
				}
			}
			eng := newTestEngine(led, nil, baseTime)
			ctx := context.Background()
			resp, err := eng.RequireWithdraw(ctx, coolwingate.RequireWithdrawReq{UserID: tt.userID})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Allowed != tt.wantAllow {
				t.Errorf("allowed: want %v, got %v", tt.wantAllow, resp.Allowed)
			}
			if !tt.wantAllow && resp.Remain == "" {
				t.Error("expected non-empty remain when blocked")
			}
		})
	}
}

// ──────────────────────────────────────────
// TestCaseConfirm 案例确认 table-driven
// ──────────────────────────────────────────

func TestCaseConfirm(t *testing.T) {
	baseTime := time.Date(2021, 8, 17, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		setupCase	bool
		manualCase	bool
		proofIDs	[]string
		resolution	string
		wantState	coolwingate.CaseState
		wantErr		string
	}{
		{
			name:		"case_not_found",
			wantErr:	"not found",
		},
		{
			name:		"no_proof_rejected",
			manualCase:	true,
			wantErr:	"proven before close",
		},
		{
			name:		"with_proof_closes",
			setupCase:	true,
			proofIDs:	nil,
			resolution:	"confirmed_by_ops",
			wantState:	coolwingate.CaseClosed,
		},
		{
			name:		"with_proof_default_resolution",
			setupCase:	true,
			proofIDs:	nil,
			resolution:	"",
			wantState:	coolwingate.CaseClosed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil,
				&mockSecMark{}, baseTime)
			ctx := context.Background()
			var caseID string
			if tt.manualCase {
				wc := coolwingate.CoolWindowCase{
					ID:	"CASE-MANUAL-" + tt.name,
					UserID:	"USER-MANUAL",
					State:	coolwingate.CaseOpen,
					Kind:	coolwingate.CaseMarkAfterPreflight,
				}
				_, _ = eng.store.SaveCase(ctx, wc)
				caseID = wc.ID
			} else if tt.setupCase {
				userID := "USER-CC-" + tt.name
				withdrawID := "WD-CC-" + tt.name
				_, _ = eng.BindPreflight(ctx, coolwingate.PreflightBindReq{
					UserID:		userID,
					WithdrawID:	withdrawID,
				})
				_, _ = eng.MarkCoolDown(ctx, coolwingate.MarkReq{
					UserID:	userID,
					Reason:	"trading_password",
				})
				cases, _ := eng.CaseList(ctx, coolwingate.CaseOpen, 10)
				if len(cases) == 0 {
					t.Fatal("expected at least 1 case")
				}
				caseID = cases[len(cases)-1].ID
			} else {
				caseID = "NONEXIST"
			}
			proofIDs := tt.proofIDs
			if proofIDs == nil && tt.setupCase {
				existingProofs, _ := eng.ProofExport(ctx, coolwingate.ProofExportReq{CaseID: caseID, Limit: 10})
				for _, p := range existingProofs {
					proofIDs = append(proofIDs, p.ID)
				}
			}
			wc, err := eng.CaseConfirm(ctx, coolwingate.CaseConfirmReq{
				CaseID:		caseID,
				ProofIDs:	proofIDs,
				Resolution:	tt.resolution,
			})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if wc.State != tt.wantState {
				t.Errorf("state: want %s, got %s", tt.wantState, wc.State)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestCoolGenQuery 代次查询 table-driven
// ──────────────────────────────────────────

func TestCoolGenQuery(t *testing.T) {
	baseTime := time.Date(2021, 8, 17, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name	string
		userID	string
		marks	int
		wantGen	int64
		wantErr	string
	}{
		{
			name:		"no_marks_gen_0",
			userID:		"USER-CGQ-001",
			marks:		0,
			wantGen:	0,
		},
		{
			name:		"one_mark_gen_1",
			userID:		"USER-CGQ-002",
			marks:		1,
			wantGen:	1,
		},
		{
			name:		"three_marks_gen_3",
			userID:		"USER-CGQ-003",
			marks:		3,
			wantGen:	3,
		},
		{
			name:		"empty_user_rejected",
			userID:		"",
			wantErr:	"user",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, &mockSecMark{}, baseTime)
			ctx := context.Background()
			for i := 0; i < tt.marks; i++ {
				_, _ = eng.MarkCoolDown(ctx, coolwingate.MarkReq{
					UserID:	tt.userID,
					Reason:	fmt.Sprintf("reason_%d", i),
				})
			}
			resp, err := eng.CoolGenQuery(ctx, coolwingate.CoolGenQueryReq{UserID: tt.userID})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.CurrentGen != tt.wantGen {
				t.Errorf("gen: want %d, got %d", tt.wantGen, resp.CurrentGen)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestFSMTransitions FSM 状态转移 table-driven
// ──────────────────────────────────────────

func TestLeaseFSMTransitions(t *testing.T) {
	tests := []struct {
		name	string
		from	coolwingate.LeaseState
		event	string
		wantTo	coolwingate.LeaseState
		wantErr	bool
	}{
		{"open_to_bound", coolwingate.LeaseOpen, coolwingate.LeaseEvBind, coolwingate.LeaseBound, false},
		{"open_to_expired", coolwingate.LeaseOpen, coolwingate.LeaseEvExpire, coolwingate.LeaseExpired, false},
		{"bound_to_expired", coolwingate.LeaseBound, coolwingate.LeaseEvExpire, coolwingate.LeaseExpired, false},
		{"expired_no_bind", coolwingate.LeaseExpired, coolwingate.LeaseEvBind, coolwingate.LeaseExpired, true},
		{"expired_no_expire", coolwingate.LeaseExpired, coolwingate.LeaseEvExpire, coolwingate.LeaseExpired, true},
		{"bound_no_bind", coolwingate.LeaseBound, coolwingate.LeaseEvBind, coolwingate.LeaseBound, true},
	}
	fsm := coolwingate.NewLeaseFSM()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			to, err := fsm.Transition(tt.from, tt.event)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if to != tt.wantTo {
				t.Errorf("want %s, got %s", tt.wantTo, to)
			}
		})
	}
}

func TestBindFSMTransitions(t *testing.T) {
	tests := []struct {
		name	string
		from	coolwingate.BindState
		event	string
		wantTo	coolwingate.BindState
		wantErr	bool
	}{
		{"open_to_revoked", coolwingate.BindOpen, coolwingate.BindEvRevoke, coolwingate.BindRevoked, false},
		{"open_to_consumed", coolwingate.BindOpen, coolwingate.BindEvConsume, coolwingate.BindConsumed, false},
		{"revoked_no_consume", coolwingate.BindRevoked, coolwingate.BindEvConsume, coolwingate.BindRevoked, true},
		{"revoked_no_revoke", coolwingate.BindRevoked, coolwingate.BindEvRevoke, coolwingate.BindRevoked, true},
		{"consumed_no_revoke", coolwingate.BindConsumed, coolwingate.BindEvRevoke, coolwingate.BindConsumed, true},
		{"consumed_no_consume", coolwingate.BindConsumed, coolwingate.BindEvConsume, coolwingate.BindConsumed, true},
	}
	fsm := coolwingate.NewBindFSM()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			to, err := fsm.Transition(tt.from, tt.event)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if to != tt.wantTo {
				t.Errorf("want %s, got %s", tt.wantTo, to)
			}
		})
	}
}

func TestCaseFSMTransitions(t *testing.T) {
	tests := []struct {
		name	string
		from	coolwingate.CaseState
		event	string
		wantTo	coolwingate.CaseState
		wantErr	bool
	}{
		{"open_to_proven", coolwingate.CaseOpen, coolwingate.CaseEvProve, coolwingate.CaseProven, false},
		{"proven_to_closed", coolwingate.CaseProven, coolwingate.CaseEvClose, coolwingate.CaseClosed, false},
		{"open_no_close", coolwingate.CaseOpen, coolwingate.CaseEvClose, coolwingate.CaseOpen, true},
		{"closed_no_prove", coolwingate.CaseClosed, coolwingate.CaseEvProve, coolwingate.CaseClosed, true},
		{"closed_no_close", coolwingate.CaseClosed, coolwingate.CaseEvClose, coolwingate.CaseClosed, true},
	}
	fsm := coolwingate.NewCaseFSM()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			to, err := fsm.Transition(tt.from, tt.event)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if to != tt.wantTo {
				t.Errorf("want %s, got %s", tt.wantTo, to)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestProofExport 证明导出 table-driven
// ──────────────────────────────────────────

func TestProofExport(t *testing.T) {
	baseTime := time.Date(2021,
		8, 17, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		userID		string
		caseID		string
		setupProof	bool
		wantCount	int
		wantErr		string
	}{
		{
			name:		"export_by_user_empty",
			userID:		"USER-PE-001",
			wantCount:	0,
		},
		{
			name:		"missing_params",
			wantErr:	"user_id or case_id",
		},
		{
			name:		"export_by_case",
			caseID:		"CASE-PE-001",
			setupProof:	true,
			wantCount:	1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, &mockSecMark{}, baseTime)
			ctx := context.Background()
			if tt.setupProof && tt.caseID != "" {
				_, _ = eng.store.SaveProof(ctx, coolwingate.WindowProof{
					ID:	"PROOF-PE-" + tt.name,
					CaseID:	tt.caseID,
					UserID:	"USER-PE-SETUP",
				})
			}
			proofs, err := eng.ProofExport(ctx, coolwingate.ProofExportReq{
				UserID:	tt.userID,
				CaseID:	tt.caseID,
			})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(proofs) != tt.wantCount {
				t.Errorf("proof count: want %d, got %d", tt.wantCount, len(proofs))
			}
		})
	}
}

// ──────────────────────────────────────────
// TestSealProof 证明封口 table-driven
// ──────────────────────────────────────────

func TestSealProof(t *testing.T) {
	baseTime := time.Date(2021, 8, 17, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		proofID		string
		setup		bool
		preSealed	bool
		wantSealed	bool
		wantErr		string
	}{
		{
			name:		"not_found",
			proofID:	"NONEXIST",
			wantErr:	"not found",
		},
		{
			name:		"seal_unsealed",
			proofID:	"PROOF-SEAL-001",
			setup:		true,
			wantSealed:	true,
		},
		{
			name:		"already_sealed_idempotent",
			proofID:	"PROOF-SEAL-002",
			setup:		true,
			preSealed:	true,
			wantSealed:	true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, nil, baseTime)
			ctx := context.Background()
			if tt.setup {
				_, _ = eng.store.SaveProof(ctx, coolwingate.WindowProof{
					ID:	tt.proofID,
					CaseID:	"CASE-SEAL",
					Sealed:	tt.preSealed,
				})
			}
			proof, err := eng.SealProof(ctx, tt.proofID)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(),
					tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if proof.Sealed != tt.wantSealed {
				t.Errorf("sealed: want %v, got %v", tt.wantSealed, proof.Sealed)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestRevokeBind 手动撤回 table-driven
// ──────────────────────────────────────────

func TestRevokeBind(t *testing.T) {
	baseTime := time.Date(2021, 8, 17, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		setup		bool
		preState	coolwingate.BindState
		wantState	coolwingate.BindState
		wantErr		string
	}{
		{
			name:		"not_found",
			wantErr:	"not found",
		},
		{
			name:		"revoke_open",
			setup:		true,
			preState:	coolwingate.BindOpen,
			wantState:	coolwingate.BindRevoked,
		},
		{
			name:		"revoke_already_revoked_noop",
			setup:		true,
			preState:	coolwingate.BindRevoked,
			wantState:	coolwingate.BindRevoked,
		},
		{
			name:		"revoke_consumed_noop",
			setup:		true,
			preState:	coolwingate.BindConsumed,
			wantState:	coolwingate.BindConsumed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, nil, baseTime)
			ctx := context.Background()
			bindID := "BIND-REVOKE-" + tt.name
			if tt.setup {
				_, _ = eng.store.SaveBind(ctx, coolwingate.PreflightBind{
					ID:	bindID,
					State:	tt.preState,
					UserID:	"USER-REVOKE",
					Token:	"TOKEN-" + tt.name,
				})
			} else {
				bindID = "NONEXIST"
			}
			bind, err := eng.RevokeBind(ctx, bindID, "manual")
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if bind.State != tt.wantState {
				t.Errorf("state: want %s, got %s", tt.wantState, bind.State)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestExpireLease 手动过期 table-driven
// ──────────────────────────────────────────

func TestExpireLease(t *testing.T) {
	baseTime := time.Date(2021, 8, 17, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		setup		bool
		preState	coolwingate.LeaseState
		wantState	coolwingate.LeaseState
		wantErr		string
	}{
		{
			name:		"not_found",
			wantErr:	"not found",
		},
		{
			name:		"expire_open",
			setup:		true,
			preState:	coolwingate.LeaseOpen,
			wantState:	coolwingate.LeaseExpired,
		},
		{
			name:		"expire_bound",
			setup:		true,
			preState:	coolwingate.LeaseBound,
			wantState:	coolwingate.LeaseExpired,
		},
		{
			name:		"expire_already_expired_noop",
			setup:		true,
			preState:	coolwingate.LeaseExpired,
			wantState:	coolwingate.LeaseExpired,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, nil, baseTime)
			ctx := context.Background()
			leaseID := "LEASE-EXPIRE-" + tt.name
			if tt.setup {
				_, _ = eng.store.SaveLease(ctx, coolwingate.CoolLease{
					ID:		leaseID,
					State:		tt.preState,
					UserID:		"USER-EXPIRE",
					LeaseExpiry:	baseTime.Add(time.Hour),
				})
			} else {
				leaseID = "NONEXIST"
			}
			lease, err := eng.ExpireLease(ctx, leaseID)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if lease.State != tt.wantState {
				t.Errorf("state: want %s, got %s", tt.wantState, lease.State)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestBindLease 租约绑定 table-driven
// ──────────────────────────────────────────

func TestBindLease(t *testing.T) {
	baseTime := time.Date(2021, 8, 17, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		setup		bool
		preState	coolwingate.LeaseState
		preExpired	bool
		wantState	coolwingate.LeaseState
		wantErr		string
	}{
		{
			name:		"not_found",
			wantErr:	"not found",
		},
		{
			name:		"bind_open_lease",
			setup:		true,
			preState:	coolwingate.LeaseOpen,
			wantState:	coolwingate.LeaseBound,
		},
		{
			name:		"bind_expired_lease_rejected",
			setup:		true,
			preState:	coolwingate.LeaseOpen,
			preExpired:	true,
			wantErr:	"expired",
		},
		{
			name:		"bind_bound_lease_rejected",
			setup:		true,
			preState:	coolwingate.LeaseBound,
			wantErr:	"transition",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, nil, baseTime)
			ctx := context.Background()
			leaseID := "LEASE-BIND-" + tt.name
			if tt.setup {
				expiry := baseTime.Add(24 * time.Hour)
				if tt.preExpired {
					expiry = baseTime.Add(-time.Hour)
				}
				_, _ = eng.store.SaveLease(ctx, coolwingate.CoolLease{
					ID:		leaseID,
					State:		tt.preState,
					UserID:		"USER-BIND",
					LeaseExpiry:	expiry,
				})
			} else {
				leaseID = "NONEXIST"
			}
			lease, err := eng.BindLease(ctx, leaseID, "WD-BIND-001")
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if lease.State != tt.wantState {
				t.Errorf("state: want %s, got %s", tt.wantState, lease.State)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestWorkerPreflightBindScan 预检绑定过期扫描
// ──────────────────────────────────────────

func TestWorkerPreflightBindScan(t *testing.T) {
	baseTime := time.Date(2021, 8, 17, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		bindExpired	bool
		wantRevoked	bool
	}{
		{
			name:		"expired_bind_revoked",
			bindExpired:	true,
			wantRevoked:	true,
		},
		{
			name:		"active_bind_untouched",
			bindExpired:	false,
			wantRevoked:	false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil,
				nil, baseTime)
			ctx := context.Background()
			expiry := baseTime.Add(time.Hour)
			if tt.bindExpired {
				expiry = baseTime.Add(-time.Minute)
			}
			_, _ = eng.store.SaveBind(ctx, coolwingate.PreflightBind{
				ID:		"BIND-SCAN-" + tt.name,
				UserID:		"USER-SCAN",
				State:		coolwingate.BindOpen,
				BindExpiry:	expiry,
			})
			job := coolwingate.PreflightBindScanJob(coolwingate.PreflightBindScanConfig{Engine: eng})
			if err := job(ctx); err != nil {
				t.Fatalf("job: %v", err)
			}
			bind, _ := eng.store.GetBind(ctx, "BIND-SCAN-"+tt.name)
			if tt.wantRevoked && bind.State != coolwingate.BindRevoked {
				t.Errorf("expected revoked, got %s", bind.State)
			}
			if !tt.wantRevoked && bind.State != coolwingate.BindOpen {
				t.Errorf("expected open, got %s", bind.State)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestWorkerMarkRevokePump Mark 撤回泵
// ──────────────────────────────────────────

func TestWorkerMarkRevokePump(t *testing.T) {
	baseTime := time.Date(2021, 8, 17, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		leaseExpired	bool
		hasBind		bool
		wantCase	bool
	}{
		{
			name:		"expired_lease_with_bind_creates_case",
			leaseExpired:	true,
			hasBind:	true,
			wantCase:	true,
		},
		{
			name:		"expired_lease_no_bind",
			leaseExpired:	true,
			hasBind:	false,
			wantCase:	false,
		},
		{
			name:		"active_lease_with_bind",
			leaseExpired:	false,
			hasBind:	true,
			wantCase:	false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, nil, baseTime)
			ctx := context.Background()
			expiry := baseTime.Add(time.Hour)
			if tt.leaseExpired {
				expiry = baseTime.Add(-time.Minute)
			}
			leaseID := "LEASE-PUMP-" + tt.name
			_, _ = eng.store.SaveLease(ctx, coolwingate.CoolLease{
				ID:		leaseID,
				UserID:		"USER-PUMP",
				State:		coolwingate.LeaseOpen,
				CoolGen:	1,
				LeaseExpiry:	expiry,
			})
			if tt.hasBind {
				_, _ = eng.store.SaveBind(ctx, coolwingate.PreflightBind{
					ID:		"BIND-PUMP-" + tt.name,
					UserID:		"USER-PUMP",
					LeaseID:	leaseID,
					State:		coolwingate.BindOpen,
				})
			}
			job := coolwingate.MarkRevokePumpJob(coolwingate.MarkRevokePumpConfig{Engine: eng})
			if err := job(ctx); err != nil {
				t.Fatalf("job: %v", err)
			}
			cases, _ := eng.store.ListCasesByState(ctx, coolwingate.CaseOpen, 10)
			if tt.wantCase && len(cases) == 0 {
				t.Error("expected case to be created")
			}
			if !tt.wantCase && len(cases) > 0 {
				t.Error("expected no cases")
			}
		})
	}
}

// ──────────────────────────────────────────
// TestWorkerLeakPatrol 泄漏巡检
// ──────────────────────────────────────────

func TestWorkerLeakPatrol(t *testing.T) {
	baseTime := time.Date(2021,
		8, 17, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name		string
		proofSealed	bool
		wantProven	bool
	}{
		{
			name:		"sealed_proofs_promote_case",
			proofSealed:	true,
			wantProven:	true,
		},
		{
			name:		"unsealed_proofs_no_promote",
			proofSealed:	false,
			wantProven:	false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := newTestEngine(nil, nil, baseTime)
			ctx := context.Background()
			caseID := "CASE-PATROL-" + tt.name
			_, _ = eng.store.SaveCase(ctx, coolwingate.CoolWindowCase{
				ID:	caseID,
				State:	coolwingate.CaseOpen,
			})
			_, _ = eng.store.SaveProof(ctx, coolwingate.WindowProof{
				ID:	"PROOF-PATROL-" + tt.name,
				CaseID:	caseID,
				Sealed:	tt.proofSealed,
			})
			job := coolwingate.LeakPatrolJob(coolwingate.LeakPatrolConfig{Engine: eng})
			if err := job(ctx); err != nil {
				t.Fatalf("job: %v", err)
			}
			wc, _ := eng.store.GetCase(ctx, caseID)
			if tt.wantProven && wc.State != coolwingate.CaseProven {
				t.Errorf("expected proven, got %s", wc.State)
			}
			if !tt.wantProven && wc.State != coolwingate.CaseOpen {
				t.Errorf("expected open, got %s", wc.State)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestNilEngine 空引擎防御
// ──────────────────────────────────────────

func TestNilEngine(t *testing.T) {
	var eng *coolwingate.Engine
	ctx := context.Background()
	_, err := eng.MarkCoolDown(ctx, coolwingate.MarkReq{UserID: "u", Reason: "r"})
	if !errors.Is(err, coolwingate.ErrEngineNil) {
		t.Errorf("MarkCoolDown: want ErrEngineNil, got %v", err)
	}
	_, err = eng.BindPreflight(ctx, coolwingate.PreflightBindReq{UserID: "u", WithdrawID: "w"})
	if !errors.Is(err, coolwingate.ErrEngineNil) {
		t.Errorf("BindPreflight: want ErrEngineNil, got %v", err)
	}
	err = eng.CheckBroadcast(ctx, coolwingate.BroadcastCheckReq{UserID: "u", WithdrawID: "w", BindToken: "t"})
	if !errors.Is(err, coolwingate.ErrEngineNil) {
		t.Errorf("CheckBroadcast: want ErrEngineNil, got %v", err)
	}
	_, err = eng.RequireWithdraw(ctx, coolwingate.RequireWithdrawReq{UserID: "u"})
	if !errors.Is(err, coolwingate.ErrEngineNil) {
		t.Errorf("RequireWithdraw: want ErrEngineNil, got %v", err)
	}
	_, err = eng.CoolGenQuery(ctx, coolwingate.CoolGenQueryReq{UserID: "u"})
	if !errors.Is(err, coolwingate.ErrEngineNil) {
		t.Errorf("CoolGenQuery: want ErrEngineNil, got %v", err)
	}
	_, err = eng.CaseConfirm(ctx, coolwingate.CaseConfirmReq{CaseID: "c"})
	if !errors.Is(err, coolwingate.ErrEngineNil) {
		t.Errorf("CaseConfirm: want ErrEngineNil, got %v", err)
	}
	_, err = eng.ProofExport(ctx, coolwingate.ProofExportReq{UserID: "u"})
	if !errors.Is(err, coolwingate.ErrEngineNil) {
		t.Errorf("ProofExport: want ErrEngineNil, got %v", err)
	}
	_, err = eng.SealProof(ctx, "p")
	if !errors.Is(err, coolwingate.ErrEngineNil) {
		t.Errorf("SealProof: want ErrEngineNil, got %v", err)
	}
	_, err = eng.GetLease(ctx, "l")
	if !errors.Is(err, coolwingate.ErrEngineNil) {
		t.Errorf("GetLease: want ErrEngineNil, got %v", err)
	}
	_, err = eng.GetBind(ctx, "b")
	if !errors.Is(err, coolwingate.ErrEngineNil) {
		t.Errorf("GetBind: want ErrEngineNil, got %v", err)
	}
	_, err = eng.RevokeBind(ctx, "b", "r")
	if !errors.Is(err, coolwingate.ErrEngineNil) {
		t.Errorf("RevokeBind: want ErrEngineNil, got %v", err)
	}
	_, err = eng.ExpireLease(ctx, "l")
	if !errors.Is(err, coolwingate.ErrEngineNil) {
		t.Errorf("ExpireLease: want ErrEngineNil, got %v", err)
	}
	_, err = eng.BindLease(ctx, "l", "w")
	if !errors.Is(err, coolwingate.ErrEngineNil) {
		t.Errorf("BindLease: want ErrEngineNil, got %v", err)
	}
}

// ──────────────────────────────────────────
// TestEndToEnd_PreflightMarkBroadcast 端到端场景
// ──────────────────────────────────────────

func TestEndToEnd_PreflightMarkBroadcast(t *testing.T) {
	baseTime := time.Date(2021,
		8, 17, 10, 0, 0, 0, time.UTC)
	sm := &mockSecMark{}
	eng := newTestEngine(nil, sm, baseTime)
	ctx := context.Background()
	bind, err := eng.BindPreflight(ctx, coolwingate.PreflightBindReq{
		UserID:		"USER-E2E-001",
		WithdrawID:	"WD-E2E-001",
	})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	if bind.State != coolwingate.BindOpen {
		t.Fatalf("bind state: want open, got %s", bind.State)
	}
	if bind.Token == "" {
		t.Fatal("expected token")
	}
	lease, err := eng.MarkCoolDown(ctx, coolwingate.MarkReq{
		UserID:	"USER-E2E-001",
		Reason:	"trading_password",
	})
	if err != nil {
		t.Fatalf("mark: %v", err)
	}
	if lease.CoolGen != 1 {
		t.Fatalf("gen: want 1, got %d", lease.CoolGen)
	}
	rBind, _ := eng.GetBindByWithdraw(ctx, "WD-E2E-001")
	if rBind.State != coolwingate.BindRevoked {
		t.Fatalf("bind after mark: want revoked, got %s", rBind.State)
	}
	err = eng.CheckBroadcast(ctx, coolwingate.BroadcastCheckReq{
		UserID:		"USER-E2E-001",
		WithdrawID:	"WD-E2E-001",
		BindToken:	bind.Token,
	})
	if err == nil {
		t.Fatal("expected broadcast blocked")
	}
	if !strings.Contains(err.Error(), "broadcast blocked") {
		t.Fatalf("unexpected error: %v", err)
	}
	cases, _ := eng.CaseList(ctx, coolwingate.CaseOpen, 10)
	found := false
	for _, c := range cases {
		if c.WithdrawID == "WD-E2E-001" && c.Kind == coolwingate.CaseMarkAfterPreflight {
			found = true
		}
	}
	if !found {
		t.Fatal("expected CoolWindowCase for WD-E2E-001")
	}
	proofs, _ := eng.ProofExport(ctx, coolwingate.ProofExportReq{UserID: "USER-E2E-001"})
	if len(proofs) == 0 {
		t.Fatal("expected at least 1 proof")
	}
	if proofs[0].PreflightGen != 0 || proofs[0].MarkGen != 1 {
		t.Errorf("proof gen: want preflight=0 mark=1, got %d/%d", proofs[0].PreflightGen, proofs[0].MarkGen)
	}
}

// ──────────────────────────────────────────
// TestEndToEnd_HappyPath 正常出金无安全变更
// ──────────────────────────────────────────

func TestEndToEnd_HappyPath(t *testing.T) {
	baseTime := time.Date(2021, 8, 17, 10, 0, 0, 0, time.UTC)
	eng := newTestEngine(nil, nil, baseTime)
	ctx := context.Background()
	bind, err := eng.BindPreflight(ctx, coolwingate.PreflightBindReq{
		UserID:		"USER-HAPPY",
		WithdrawID:	"WD-HAPPY",
	})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	err = eng.CheckBroadcast(ctx, coolwingate.BroadcastCheckReq{
		UserID:		"USER-HAPPY",
		WithdrawID:	"WD-HAPPY",
		BindToken:	bind.Token,
	})
	if err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	consumed, _ := eng.GetBindByWithdraw(ctx, "WD-HAPPY")
	if consumed.State != coolwingate.BindConsumed {
		t.Errorf("bind state: want consumed, got %s", consumed.State)
	}
	cases,
		_ := eng.CaseList(ctx, coolwingate.CaseOpen, 10)
	for _, c := range cases {
		if c.WithdrawID == "WD-HAPPY" {
			t.Error("expected no cases for happy path")
		}
	}
}

// ──────────────────────────────────────────
// TestEndToEnd_ConcurrentMarkAndRequire Mark 后出金查询
// ──────────────────────────────────────────

func TestEndToEnd_ConcurrentMarkAndRequire(t *testing.T) {
	baseTime := time.Date(2021, 8, 17, 10, 0, 0, 0, time.UTC)
	led := &mockLedger{
		lockUntil:	baseTime.Add(24 * time.Hour),
		reason:		"trading_password",
	}
	eng := newTestEngine(led, &mockSecMark{}, baseTime)
	ctx := context.Background()
	_, err := eng.MarkCoolDown(ctx, coolwingate.MarkReq{
		UserID:	"USER-CONC",
		Reason:	"trading_password",
	})
	if err != nil {
		t.Fatalf("mark: %v", err)
	}
	resp, err := eng.RequireWithdraw(ctx, coolwingate.RequireWithdrawReq{UserID: "USER-CONC"})
	if err != nil {
		t.Fatalf("require: %v", err)
	}
	if resp.Allowed {
		t.Fatal("expected blocked")
	}
	if resp.Remain == "" {
		t.Error("expected remain")
	}
	_, err = eng.BindPreflight(ctx, coolwingate.PreflightBindReq{
		UserID:		"USER-CONC",
		WithdrawID:	"WD-CONC",
	})
	if err == nil {
		t.Fatal("expected preflight rejected during lock")
	}
	if !strings.Contains(err.Error(), "lock window") {
		t.Fatalf("unexpected: %v", err)
	}
}

// ──────────────────────────────────────────
// TestMemStore 存储层 table-driven
// ──────────────────────────────────────────

func TestMemStoreLease(t *testing.T) {
	store := coolwingate.NewMemStore()
	ctx := context.Background()
	_, err := store.GetLease(ctx, "nonexist")
	if !errors.Is(err, coolwingate.ErrLeaseNotFound) {
		t.Errorf("want ErrLeaseNotFound, got %v", err)
	}
	saved, err := store.SaveLease(ctx, coolwingate.CoolLease{
		ID:		"LEASE-1",
		UserID:		"USER-1",
		CoolGen:	1,
		State:		coolwingate.LeaseOpen,
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.ID != "LEASE-1" {
		t.Errorf("id: want LEASE-1, got %s", saved.ID)
	}
	got, err := store.GetLease(ctx, "LEASE-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CoolGen != 1 {
		t.Errorf("gen: want 1, got %d", got.CoolGen)
	}
	byUser, err := store.GetLeaseByUser(ctx, "USER-1")
	if err != nil {
		t.Fatalf("by user: %v", err)
	}
	if byUser.ID != "LEASE-1" {
		t.Errorf("id: want LEASE-1, got %s", byUser.ID)
	}
}

func TestMemStoreBind(t *testing.T) {
	store := coolwingate.NewMemStore()
	ctx := context.Background()
	_, err := store.GetBind(ctx, "nonexist")
	if !errors.Is(err, coolwingate.ErrBindNotFound) {
		t.Errorf("want ErrBindNotFound, got %v", err)
	}
	saved, err := store.SaveBind(ctx, coolwingate.PreflightBind{
		ID:		"BIND-1",
		UserID:		"USER-1",
		WithdrawID:	"WD-1",
		Token:		"TOKEN-1",
		State:		coolwingate.BindOpen,
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.ID != "BIND-1" {
		t.Errorf("id: want BIND-1, got %s", saved.ID)
	}
	byWD, err := store.GetBindByWithdraw(ctx, "WD-1")
	if err != nil {
		t.Fatalf("by withdraw: %v", err)
	}
	if byWD.Token != "TOKEN-1" {
		t.Errorf("token mismatch")
	}
	byToken, err := store.GetBindByToken(ctx, "TOKEN-1")
	if err != nil {
		t.Fatalf("by token: %v", err)
	}
	if byToken.WithdrawID != "WD-1" {
		t.Errorf("withdraw_id mismatch")
	}
}

func TestMemStoreCase(t *testing.T) {
	store := coolwingate.NewMemStore()
	ctx := context.Background()
	_, err := store.GetCase(ctx, "nonexist")
	if !errors.Is(err, coolwingate.ErrCaseNotFound) {
		t.Errorf("want ErrCaseNotFound, got %v", err)
	}
	_, err = store.SaveCase(ctx, coolwingate.CoolWindowCase{
		ID:		"CASE-1",
		UserID:		"USER-1",
		WithdrawID:	"WD-1",
		State:		coolwingate.CaseOpen,
		Kind:		coolwingate.CaseMarkAfterPreflight,
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	cases, _ := store.ListCasesByState(ctx, coolwingate.CaseOpen, 10)
	if len(cases) != 1 {
		t.Errorf("want 1 case, got %d", len(cases))
	}
	byUser, _ := store.ListCasesByUser(ctx, "USER-1", 10)
	if len(byUser) != 1 {
		t.Errorf("want 1 case, got %d", len(byUser))
	}
	byWD, _ := store.ListCasesByWithdraw(ctx, "WD-1", 10)
	if len(byWD) != 1 {
		t.Errorf("want 1 case, got %d", len(byWD))
	}
}

func TestMemStoreProof(t *testing.T) {
	store := coolwingate.NewMemStore()
	ctx := context.Background()
	_, err := store.GetProof(ctx, "nonexist")
	if !errors.Is(err, coolwingate.ErrProofNotFound) {
		t.Errorf("want ErrProofNotFound, got %v", err)
	}
	_, err = store.SaveProof(ctx, coolwingate.WindowProof{
		ID:	"PROOF-1",
		CaseID:	"CASE-1",
		UserID:	"USER-1",
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	proofs, _ := store.ListProofsByCase(ctx, "CASE-1", 10)
	if len(proofs) != 1 {
		t.Errorf("want 1 proof, got %d", len(proofs))
	}
	byUser, _ := store.ListProofsByUser(ctx, "USER-1", 10)
	if len(byUser) != 1 {
		t.Errorf("want 1 proof, got %d", len(byUser))
	}
}

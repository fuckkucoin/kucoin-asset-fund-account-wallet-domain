package allowleasegate_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/allowleasegate"
)

// ──────────────────────────────────────────
// Mock CaseQuerier
// ──────────────────────────────────────────

type mockCaseQuerier struct {
	mu	sync.RWMutex
	cases	map[string]allowleasegate.CaseSnapshot
}

func newMockCaseQuerier() *mockCaseQuerier {
	return &mockCaseQuerier{cases: make(map[string]allowleasegate.CaseSnapshot)}
}

func (m *mockCaseQuerier) SetCase(cs allowleasegate.CaseSnapshot,) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cases[cs.ID] = cs
}

func (m *mockCaseQuerier) GetCase(id string) (allowleasegate.CaseSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cs, ok := m.cases[id]
	if !ok {
		return allowleasegate.CaseSnapshot{}, fmt.Errorf("case not found: %s", id)
	}
	return cs, nil
}

func (m *mockCaseQuerier) GetCaseByMovement(movementID string) (allowleasegate.CaseSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, cs := range m.cases {
		if cs.MovementID == movementID {
			return cs, nil
		}
	}
	return allowleasegate.CaseSnapshot{}, fmt.Errorf("case not found for movement: %s", movementID)
}

// ──────────────────────────────────────────
// 辅助
// ──────────────────────────────────────────

func testEngine(cq allowleasegate.CaseQuerier,) *allowleasegate.Engine {
	eng := allowleasegate.New(allowleasegate.Config{
		Store:		allowleasegate.NewMemStore(),
		CaseQuerier:	cq,
		DefaultTTL:	5 * time.Minute,
		MaxTTL:		30 * time.Minute,
	})
	return eng
}

func mustIssue(t *testing.T, eng *allowleasegate.Engine, req allowleasegate.IssueReq,) *allowleasegate.AllowLease {
	t.Helper()
	lease,
		err := eng.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}
	return lease
}

func defaultIssueReq() allowleasegate.IssueReq {
	return allowleasegate.IssueReq{
		MovementID:	"MVT-TEST-001",
		UserID:		"USER-TEST-001",
		Asset:		"USDT",
		Chain:		"TRC20",
		ToAddress:	"TAddrTest001",
		Amount:		"500.00",
		Decision:	"allow",
		PolicyVer:	3,
	}
}

// ──────────────────────────────────────────
// TestIssue 签发租约表驱动
// ──────────────────────────────────────────

func TestIssue(t *testing.T) {
	tests := []struct {
		name		string
		req		allowleasegate.IssueReq
		wantErr		bool
		errContains	string
		wantState	allowleasegate.LeaseState
	}{
		{
			name:	"allow decision issues lease",
			req: allowleasegate.IssueReq{
				MovementID:	"MVT-I01", UserID: "U01", Asset: "USDT", Chain: "TRC20",
				ToAddress:	"TAddr01", Amount: "100", Decision: "allow", PolicyVer: 1,
			},
			wantState:	allowleasegate.StateAllowed,
		},
		{
			name:	"review decision rejected",
			req: allowleasegate.IssueReq{
				MovementID:	"MVT-I02", UserID: "U02", Asset: "USDT", Chain: "TRC20",
				ToAddress:	"TAddr02", Amount: "100", Decision: "review", PolicyVer: 1,
			},
			wantErr:	true, errContains: "case state does not permit",
		},
		{
			name:	"deny decision rejected",
			req: allowleasegate.IssueReq{
				MovementID:	"MVT-I03", UserID: "U03", Asset: "BTC", Chain: "BTC",
				ToAddress:	"bc1q03", Amount: "1.0", Decision: "deny", PolicyVer: 2,
			},
			wantErr:	true, errContains: "case state does not permit",
		},
		{
			name:	"error decision rejected",
			req: allowleasegate.IssueReq{
				MovementID:	"MVT-I04", UserID: "U04", Asset: "ETH", Chain: "ERC20",
				ToAddress:	"0x04", Amount: "5.0", Decision: "error", PolicyVer: 3,
			},
			wantErr:	true, errContains: "case state does not permit",
		},
		{
			name:	"empty decision rejected",
			req: allowleasegate.IssueReq{
				MovementID:	"MVT-I05", UserID: "U05", Asset: "USDT", Chain: "TRC20",
				ToAddress:	"TAddr05", Amount: "50", Decision: "", PolicyVer: 1,
			},
			wantErr:	true, errContains: "case state does not permit",
		},
		{
			name:	"zero policy version rejected",
			req: allowleasegate.IssueReq{
				MovementID:	"MVT-I06", UserID: "U06", Asset: "USDT", Chain: "TRC20",
				ToAddress:	"TAddr06", Amount: "50", Decision: "allow", PolicyVer: 0,
			},
			wantErr:	true, errContains: "policy_version",
		},
		{
			name:	"negative policy version rejected",
			req: allowleasegate.IssueReq{
				MovementID:	"MVT-I07", UserID: "U07", Asset: "USDT", Chain: "TRC20",
				ToAddress:	"TAddr07", Amount: "50", Decision: "allow", PolicyVer: -1,
			},
			wantErr:	true, errContains: "policy_version",
		},
		{
			name:	"allow with custom TTL",
			req: allowleasegate.IssueReq{
				MovementID:	"MVT-I08", UserID: "U08", Asset: "BTC", Chain: "BTC",
				ToAddress:	"bc1q08", Amount: "2.0", Decision: "allow", PolicyVer: 5,
				TTL:	2 * time.Minute,
			},
			wantState:	allowleasegate.StateAllowed,
		},
		{
			name:	"allow with case_id",
			req: allowleasegate.IssueReq{
				MovementID:	"MVT-I09", UserID: "U09", Asset: "ETH", Chain: "ERC20",
				ToAddress:	"0x09", Amount: "10", Decision: "allow", PolicyVer: 4,
				CaseID:	"CASE-I09",
			},
			wantState:	allowleasegate.StateAllowed,
		},
		{
			name:	"ALLOW case insensitive",
			req: allowleasegate.IssueReq{
				MovementID:	"MVT-I10", UserID: "U10", Asset: "USDT", Chain: "TRC20",
				ToAddress:	"TAddr10", Amount: "100", Decision: "ALLOW", PolicyVer: 1,
			},
			wantState:	allowleasegate.StateAllowed,
		},
		{
			name:	"Allow mixed case",
			req: allowleasegate.IssueReq{
				MovementID:	"MVT-I11", UserID: "U11", Asset: "USDT", Chain: "TRC20",
				ToAddress:	"TAddr11", Amount: "100", Decision: " Allow ", PolicyVer: 1,
			},
			wantState:	allowleasegate.StateAllowed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := testEngine(nil)
			lease,
				err := eng.Issue(context.Background(), tt.req)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if lease.State != tt.wantState {
				t.Fatalf("want state %s, got %s", tt.wantState, lease.State)
			}
			if lease.ID == "" {
				t.Fatal("lease ID should not be empty")
			}
			if lease.PolicyVer != tt.req.PolicyVer {
				t.Fatalf("want policy_ver %d, got %d", tt.req.PolicyVer, lease.PolicyVer)
			}
			if lease.ExpiresAt.IsZero() {
				t.Fatal("expires_at should not be zero")
			}
		})
	}
}

// ──────────────────────────────────────────
// TestIssueWithDeniedCase 签发时案件已否决
// ──────────────────────────────────────────

func TestIssueWithDeniedCase(t *testing.T) {
	cq := newMockCaseQuerier()
	cq.SetCase(allowleasegate.CaseSnapshot{ID: "CASE-D01", State: "denied", DecidedBy: "reviewer-A"})
	eng := testEngine(cq)
	_, err := eng.Issue(context.Background(), allowleasegate.IssueReq{
		MovementID:	"MVT-D01", UserID: "U-D01", Asset: "USDT", Chain: "TRC20",
		ToAddress:	"TAddr-D01", Amount: "100", Decision: "allow", PolicyVer: 1, CaseID: "CASE-D01",
	})
	if err == nil {
		t.Fatal("expected error for denied case")
	}
	if !errors.Is(err, allowleasegate.ErrCaseNotAllowed) {
		t.Fatalf("expected ErrCaseNotAllowed, got: %v", err)
	}
}

func TestIssueWithExpiredCase(t *testing.T) {
	cq := newMockCaseQuerier()
	cq.SetCase(allowleasegate.CaseSnapshot{ID: "CASE-E01", State: "expired"})
	eng := testEngine(cq)
	_, err := eng.Issue(context.Background(), allowleasegate.IssueReq{
		MovementID:	"MVT-E01", UserID: "U-E01", Asset: "BTC", Chain: "BTC",
		ToAddress:	"bc1q-E01", Amount: "0.5", Decision: "allow", PolicyVer: 2, CaseID: "CASE-E01",
	})
	if err == nil {
		t.Fatal("expected error for expired case")
	}
	if !errors.Is(err, allowleasegate.ErrCaseNotAllowed) {
		t.Fatalf("expected ErrCaseNotAllowed, got: %v", err)
	}
}

// ──────────────────────────────────────────
// TestSubmitCheck 出金提交校验表驱动
// ──────────────────────────────────────────

func TestSubmitCheck(t *testing.T) {
	tests := []struct {
		name		string
		setup		func(eng *allowleasegate.Engine,) string
		submitReq	func(leaseID string) allowleasegate.SubmitReq
		wantAllowed	bool
		wantErr		error
		wantProof	bool
		wantDriftKind	string
	}{
		{
			name:	"valid lease allows submit",
			setup: func(eng *allowleasegate.Engine,) string {
				l := mustIssue(t,
					eng, defaultIssueReq())
				return l.ID
			},
			submitReq: func(lid string) allowleasegate.SubmitReq {
				return allowleasegate.SubmitReq{LeaseID: lid, MovementID: "MVT-TEST-001", UserID: "USER-TEST-001"}
			},
			wantAllowed:	true,
		},
		{
			name:	"no lease ID rejects",
			setup:	func(_ *allowleasegate.Engine,) string { return "" },
			submitReq: func(_ string) allowleasegate.SubmitReq {
				return allowleasegate.SubmitReq{LeaseID: "", MovementID: "MVT-NL", UserID: "U-NL"}
			},
			wantAllowed:	false, wantErr: allowleasegate.ErrNoLease, wantProof: true, wantDriftKind: "no_lease",
		},
		{
			name:	"unknown lease ID rejects",
			setup:	func(_ *allowleasegate.Engine,) string { return "nonexistent-id" },
			submitReq: func(lid string) allowleasegate.SubmitReq {
				return allowleasegate.SubmitReq{LeaseID: lid, MovementID: "MVT-UNK", UserID: "U-UNK"}
			},
			wantAllowed:	false, wantErr: allowleasegate.ErrLeaseNotFound, wantProof: true, wantDriftKind: "lease_not_found",
		},
		{
			name:	"movement mismatch rejects",
			setup: func(eng *allowleasegate.Engine,) string {
				l := mustIssue(t, eng, defaultIssueReq())
				return l.ID
			},
			submitReq: func(lid string) allowleasegate.SubmitReq {
				return allowleasegate.SubmitReq{LeaseID: lid, MovementID: "MVT-WRONG", UserID: "USER-TEST-001"}
			},
			wantAllowed:	false, wantErr: allowleasegate.ErrMovementMismatch, wantProof: true, wantDriftKind: "movement_mismatch",
		},
		{
			name:	"expired lease rejects",
			setup: func(eng *allowleasegate.Engine,) string {
				past := time.Now().UTC().Add(-10 * time.Minute)
				eng.SetClock(func() time.Time { return past })
				req := defaultIssueReq()
				req.TTL = 1 * time.Minute
				l := mustIssue(t, eng, req)
				eng.SetClock(func() time.Time { return time.Now().UTC() })
				return l.ID
			},
			submitReq: func(lid string) allowleasegate.SubmitReq {
				return allowleasegate.SubmitReq{LeaseID: lid, MovementID: "MVT-TEST-001", UserID: "USER-TEST-001"}
			},
			wantAllowed:	false, wantErr: allowleasegate.ErrLeaseExpired, wantProof: true, wantDriftKind: "lease_expired",
		},
		{
			name:	"revoked lease rejects",
			setup: func(eng *allowleasegate.Engine,) string {
				l := mustIssue(t, eng, defaultIssueReq())
				_, _ = eng.Revoke(context.Background(), allowleasegate.RevokeReq{LeaseID: l.ID, Reason: "test revoke"})
				return l.ID
			},
			submitReq: func(lid string) allowleasegate.SubmitReq {
				return allowleasegate.SubmitReq{LeaseID: lid, MovementID: "MVT-TEST-001", UserID: "USER-TEST-001"}
			},
			wantAllowed:	false, wantErr: allowleasegate.ErrLeaseRevoked, wantProof: true, wantDriftKind: "lease_revoked",
		},
		{
			name:	"consumed lease rejects",
			setup: func(eng *allowleasegate.Engine,) string {
				l := mustIssue(t, eng, defaultIssueReq())
				_, _ = eng.SubmitCheck(context.Background(), allowleasegate.SubmitReq{
					LeaseID:	l.ID, MovementID: "MVT-TEST-001", UserID: "USER-TEST-001",
				})
				_, _ = eng.Consume(context.Background(), l.ID)
				return l.ID
			},
			submitReq: func(lid string) allowleasegate.SubmitReq {
				return allowleasegate.SubmitReq{LeaseID: lid, MovementID: "MVT-TEST-001", UserID: "USER-TEST-001"}
			},
			wantAllowed:	false, wantErr: allowleasegate.ErrLeaseConsumed, wantProof: true, wantDriftKind: "lease_consumed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := testEngine(nil)
			lid := tt.setup(eng)
			req := tt.submitReq(lid)
			result, err := eng.SubmitCheck(context.Background(), req)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected %v, got %v", tt.wantErr, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Allowed != tt.wantAllowed {
				t.Fatalf("want allowed=%v, got %v", tt.wantAllowed, result.Allowed)
			}
			if tt.wantProof {
				if result.Proof == nil {
					t.Fatal("expected proof, got nil")
				}
				if result.Proof.DriftKind != tt.wantDriftKind {
					t.Fatalf("want drift_kind=%s, got %s", tt.wantDriftKind, result.Proof.DriftKind)
				}
				if !result.Proof.Blocked {
					t.Fatal("proof should be blocked")
				}
				if !result.Proof.Sealed {
					t.Fatal("proof should be sealed")
				}
			}
		})
	}
}

// ──────────────────────────────────────────
// TestSubmitCheckWithCaseDenied 提交时案件被否决
// ──────────────────────────────────────────

func TestSubmitCheckWithCaseDenied(t *testing.T) {
	cq := newMockCaseQuerier()
	cq.SetCase(allowleasegate.CaseSnapshot{
		ID:	"CASE-SD01", MovementID: "MVT-SD01", State: "approved", DecidedBy: "reviewer-A",
	})
	eng := testEngine(cq)
	lease := mustIssue(t, eng, allowleasegate.IssueReq{
		MovementID:	"MVT-SD01", UserID: "U-SD01", Asset: "USDT", Chain: "TRC20",
		ToAddress:	"TAddr-SD01", Amount: "100", Decision: "allow", PolicyVer: 1, CaseID: "CASE-SD01",
	})
	cq.SetCase(allowleasegate.CaseSnapshot{
		ID:	"CASE-SD01", MovementID: "MVT-SD01", State: "denied", DecidedBy: "reviewer-B",
	})
	result, err := eng.SubmitCheck(context.Background(), allowleasegate.SubmitReq{
		LeaseID:	lease.ID, MovementID: "MVT-SD01", UserID: "U-SD01",
	})
	if !errors.Is(err, allowleasegate.ErrCaseDenied) {
		t.Fatalf("expected ErrCaseDenied, got: %v", err)
	}
	if result.Allowed {
		t.Fatal("should not be allowed")
	}
	if result.Proof == nil {
		t.Fatal("expected proof")
	}
	if result.Proof.DriftKind != "case_denied" {
		t.Fatalf("want drift_kind=case_denied, got %s", result.Proof.DriftKind)
	}
}

func TestSubmitCheckWithCaseExpired(t *testing.T) {
	cq := newMockCaseQuerier()
	cq.SetCase(allowleasegate.CaseSnapshot{
		ID:	"CASE-SE01", MovementID: "MVT-SE01", State: "approved",
	})
	eng := testEngine(cq)
	lease := mustIssue(t, eng, allowleasegate.IssueReq{
		MovementID:	"MVT-SE01", UserID: "U-SE01", Asset: "BTC", Chain: "BTC",
		ToAddress:	"bc1q-SE01", Amount: "0.5", Decision: "allow", PolicyVer: 2, CaseID: "CASE-SE01",
	})
	cq.SetCase(allowleasegate.CaseSnapshot{
		ID:	"CASE-SE01", MovementID: "MVT-SE01", State: "expired",
	})
	result, err := eng.SubmitCheck(context.Background(), allowleasegate.SubmitReq{
		LeaseID:	lease.ID, MovementID: "MVT-SE01", UserID: "U-SE01",
	})
	if !errors.Is(err, allowleasegate.ErrCaseExpired) {
		t.Fatalf("expected ErrCaseExpired, got: %v", err)
	}
	if result.Allowed {
		t.Fatal("should not be allowed")
	}
	if result.Proof == nil || result.Proof.DriftKind != "case_expired" {
		t.Fatal("expected case_expired proof")
	}
}

// ──────────────────────────────────────────
// TestConsume 消费租约
// ──────────────────────────────────────────

func TestConsume(t *testing.T) {
	tests := []struct {
		name	string
		setup	func(eng *allowleasegate.Engine,) string
		wantErr	bool
	}{
		{
			name:	"consume bound lease succeeds",
			setup: func(eng *allowleasegate.Engine,) string {
				l := mustIssue(t, eng, defaultIssueReq())
				_, _ = eng.SubmitCheck(context.Background(), allowleasegate.SubmitReq{
					LeaseID:	l.ID, MovementID: "MVT-TEST-001", UserID: "USER-TEST-001",
				})
				return l.ID
			},
		},
		{
			name:	"consume allowed lease fails (not bound)",
			setup: func(eng *allowleasegate.Engine,) string {
				l := mustIssue(t,
					eng, defaultIssueReq())
				return l.ID
			},
			wantErr:	true,
		},
		{
			name:	"consume revoked lease fails",
			setup: func(eng *allowleasegate.Engine,) string {
				l := mustIssue(t,
					eng, defaultIssueReq())
				_, _ = eng.Revoke(context.Background(), allowleasegate.RevokeReq{LeaseID: l.ID, Reason: "test"})
				return l.ID
			},
			wantErr:	true,
		},
		{
			name:	"consume expired lease fails",
			setup: func(eng *allowleasegate.Engine,) string {
				past := time.Now().UTC().Add(-10 * time.Minute)
				eng.SetClock(func() time.Time { return past })
				req := defaultIssueReq()
				req.TTL = 1 * time.Minute
				l := mustIssue(t, eng, req)
				eng.SetClock(func() time.Time { return time.Now().UTC() })
				_, _ = eng.ExpireStale(context.Background())
				return l.ID
			},
			wantErr:	true,
		},
		{
			name:		"consume nonexistent lease fails",
			setup:		func(_ *allowleasegate.Engine,) string { return "nonexistent" },
			wantErr:	true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := testEngine(nil)
			lid := tt.setup(eng)
			lease,
				err := eng.Consume(context.Background(), lid)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if lease.State != allowleasegate.StateConsumed {
				t.Fatalf("want consumed, got %s", lease.State)
			}
			if lease.ConsumedAt == nil {
				t.Fatal("consumed_at should be set")
			}
		})
	}
}

// ──────────────────────────────────────────
// TestRevoke 撤销
// ──────────────────────────────────────────

func TestRevoke(t *testing.T) {
	tests := []struct {
		name		string
		setup		func(eng *allowleasegate.Engine,) string
		reason		string
		wantErr		bool
		wantRevoked	bool
	}{
		{
			name:	"revoke allowed lease",
			setup: func(eng *allowleasegate.Engine,) string {
				l := mustIssue(t, eng, defaultIssueReq())
				return l.ID
			},
			reason:	"ops revoke", wantRevoked: true,
		},
		{
			name:	"revoke bound lease",
			setup: func(eng *allowleasegate.Engine,) string {
				l := mustIssue(t, eng, defaultIssueReq())
				_, _ = eng.SubmitCheck(context.Background(), allowleasegate.SubmitReq{
					LeaseID:	l.ID, MovementID: "MVT-TEST-001", UserID: "USER-TEST-001",
				})
				return l.ID
			},
			reason:	"late deny", wantRevoked: true,
		},
		{
			name:	"revoke consumed lease is idempotent",
			setup: func(eng *allowleasegate.Engine,) string {
				l := mustIssue(t, eng, defaultIssueReq())
				_, _ = eng.SubmitCheck(context.Background(), allowleasegate.SubmitReq{
					LeaseID:	l.ID, MovementID: "MVT-TEST-001", UserID: "USER-TEST-001",
				})
				_,
					_ = eng.Consume(context.Background(), l.ID)
				return l.ID
			},
			reason:	"test", wantRevoked: false,
		},
		{
			name:	"revoke already revoked is idempotent",
			setup: func(eng *allowleasegate.Engine,) string {
				l := mustIssue(t, eng, defaultIssueReq())
				_, _ = eng.Revoke(context.Background(), allowleasegate.RevokeReq{LeaseID: l.ID, Reason: "first"})
				return l.ID
			},
			reason:	"second", wantRevoked: false,
		},
		{
			name:		"revoke nonexistent lease fails",
			setup:		func(_ *allowleasegate.Engine,) string { return "nonexistent" },
			reason:		"test",
			wantErr:	true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := testEngine(nil)
			lid := tt.setup(eng)
			lease, err := eng.Revoke(context.Background(), allowleasegate.RevokeReq{LeaseID: lid, Reason: tt.reason})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantRevoked && lease.State != allowleasegate.StateRevoked {
				t.Fatalf("want revoked, got %s", lease.State)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestExpireStale 过期扫描
// ──────────────────────────────────────────

func TestExpireStale(t *testing.T) {
	eng := testEngine(nil)
	past := time.Now().UTC().Add(-10 * time.Minute)
	eng.SetClock(func() time.Time { return past })
	for i := 0; i < 5; i++ {
		req := defaultIssueReq()
		req.MovementID = fmt.Sprintf("MVT-EXP-%d", i)
		req.TTL = 1 * time.Minute
		mustIssue(t, eng, req)
	}
	eng.SetClock(func() time.Time { return time.Now().UTC() })
	req := defaultIssueReq()
	req.MovementID = "MVT-EXP-FRESH"
	req.TTL = 30 * time.Minute
	mustIssue(t, eng, req)
	expired, err := eng.ExpireStale(context.Background())
	if err != nil {
		t.Fatalf("ExpireStale: %v", err)
	}
	if len(expired) != 5 {
		t.Fatalf("want 5 expired, got %d", len(expired))
	}
	active, _ := eng.store.ListActive()
	if len(active) != 1 {
		t.Fatalf("want 1 active, got %d", len(active))
	}
}

// ──────────────────────────────────────────
// TestPatrolCases 案件巡检
// ──────────────────────────────────────────

func TestPatrolCases(t *testing.T) {
	cq := newMockCaseQuerier()
	eng := testEngine(cq)
	cq.SetCase(allowleasegate.CaseSnapshot{ID: "C-P01", MovementID: "MVT-P01", State: "approved"})
	cq.SetCase(allowleasegate.CaseSnapshot{ID: "C-P02", MovementID: "MVT-P02", State: "approved"})
	cq.SetCase(allowleasegate.CaseSnapshot{ID: "C-P03", MovementID: "MVT-P03", State: "approved"})
	cq.SetCase(allowleasegate.CaseSnapshot{ID: "C-P04", MovementID: "MVT-P04", State: "open"})
	for _, tc := range []struct {
		mvt	string
		caseID	string
	}{
		{"MVT-P01", "C-P01"},
		{"MVT-P02", "C-P02"},
		{"MVT-P03", "C-P03"},
		{"MVT-P04", "C-P04"},
	} {
		req := defaultIssueReq()
		req.MovementID = tc.mvt
		req.CaseID = tc.caseID
		mustIssue(t, eng, req)
	}
	cq.SetCase(allowleasegate.CaseSnapshot{ID: "C-P02", MovementID: "MVT-P02", State: "denied", DecidedBy: "rev-B"})
	cq.SetCase(allowleasegate.CaseSnapshot{ID: "C-P03", MovementID: "MVT-P03", State: "expired"})
	result, err := eng.PatrolCases(context.Background())
	if err != nil {
		t.Fatalf("PatrolCases: %v", err)
	}
	if len(result.Revoked) != 2 {
		t.Fatalf("want 2 revoked, got %d: %v", len(result.Revoked), result.Revoked)
	}
	if len(result.Proofs) != 2 {
		t.Fatalf("want 2 proofs, got %d", len(result.Proofs))
	}
}

func TestPatrolNoCaseQuerier(t *testing.T) {
	eng := testEngine(nil)
	mustIssue(t,
		eng, defaultIssueReq())
	result, err := eng.PatrolCases(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Revoked) != 0 {
		t.Fatal("should not revoke without case querier")
	}
}

// ──────────────────────────────────────────
// TestFSM 状态机转换
// ──────────────────────────────────────────

func TestFSM(t *testing.T) {
	tests := []struct {
		name	string
		from	allowleasegate.LeaseState
		event	string
		wantTo	allowleasegate.LeaseState
		wantErr	bool
	}{
		{"allowed→bind→bound", allowleasegate.StateAllowed, allowleasegate.EventBind, allowleasegate.StateBound, false},
		{"allowed→expire→expired", allowleasegate.StateAllowed, allowleasegate.EventExpire, allowleasegate.StateExpired, false},
		{"allowed→revoke→revoked", allowleasegate.StateAllowed, allowleasegate.EventRevoke, allowleasegate.StateRevoked, false},
		{"bound→consume→consumed", allowleasegate.StateBound, allowleasegate.EventConsume, allowleasegate.StateConsumed, false},
		{"bound→expire→expired", allowleasegate.StateBound, allowleasegate.EventExpire, allowleasegate.StateExpired, false},
		{"bound→revoke→revoked", allowleasegate.StateBound, allowleasegate.EventRevoke, allowleasegate.StateRevoked, false},
		{"allowed→consume illegal", allowleasegate.StateAllowed, allowleasegate.EventConsume, allowleasegate.StateAllowed, true},
		{"consumed→bind illegal", allowleasegate.StateConsumed, allowleasegate.EventBind, allowleasegate.StateConsumed, true},
		{"expired→bind illegal", allowleasegate.StateExpired, allowleasegate.EventBind, allowleasegate.StateExpired, true},
		{"revoked→bind illegal", allowleasegate.StateRevoked, allowleasegate.EventBind, allowleasegate.StateRevoked, true},
		{"consumed→revoke illegal", allowleasegate.StateConsumed, allowleasegate.EventRevoke, allowleasegate.StateConsumed, true},
		{"expired→revoke illegal", allowleasegate.StateExpired, allowleasegate.EventRevoke, allowleasegate.StateExpired, true},
		{"consumed→expire illegal", allowleasegate.StateConsumed, allowleasegate.EventExpire, allowleasegate.StateConsumed, true},
		{"bound→bind illegal", allowleasegate.StateBound, allowleasegate.EventBind, allowleasegate.StateBound, true},
	}
	sm := allowleasegate.NewLeaseSM()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sm.apply(tt.from, tt.event)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantTo {
				t.Fatalf("want %s, got %s", tt.wantTo, got)
			}
		})
	}
}

func TestApplyEvent(t *testing.T) {
	tests := []struct {
		from	allowleasegate.LeaseState
		event	string
		wantTo	allowleasegate.LeaseState
		wantErr	bool
	}{
		{allowleasegate.StateAllowed, allowleasegate.EventBind, allowleasegate.StateBound, false},
		{allowleasegate.StateBound, allowleasegate.EventConsume, allowleasegate.StateConsumed, false},
		{allowleasegate.StateAllowed, allowleasegate.EventConsume, allowleasegate.StateAllowed, true},
	}
	for _, tt := range tests {
		got, err := allowleasegate.ApplyEvent(tt.from, tt.event)
		if tt.wantErr && err == nil {
			t.Fatalf("expected error for %s+%s", tt.from, tt.event)
		}
		if !tt.wantErr && err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !tt.wantErr && got != tt.wantTo {
			t.Fatalf("want %s, got %s", tt.wantTo, got)
		}
	}
}

// ──────────────────────────────────────────
// TestLeaseState helpers
// ──────────────────────────────────────────

func TestLeaseStateTerminal(t *testing.T) {
	tests := []struct {
		state	allowleasegate.LeaseState
		term	bool
	}{
		{allowleasegate.StateAllowed, false},
		{allowleasegate.StateBound, false},
		{allowleasegate.StateConsumed, true},
		{allowleasegate.StateExpired, true},
		{allowleasegate.StateRevoked, true},
	}
	for _, tt := range tests {
		if got := tt.state.Terminal(); got != tt.term {
			t.Fatalf("%s: want Terminal=%v, got %v", tt.state, tt.term, got)
		}
	}
}

func TestLeaseStateActive(t *testing.T) {
	tests := []struct {
		state	allowleasegate.LeaseState
		active	bool
	}{
		{allowleasegate.StateAllowed, true},
		{allowleasegate.StateBound, true},
		{allowleasegate.StateConsumed, false},
		{allowleasegate.StateExpired, false},
		{allowleasegate.StateRevoked, false},
	}
	for _, tt := range tests {
		if got := tt.state.Active(); got != tt.active {
			t.Fatalf("%s: want Active=%v, got %v", tt.state, tt.active, got)
		}
	}
}

// ──────────────────────────────────────────
// TestAllowLeaseExpired / Alive helpers
// ──────────────────────────────────────────

func TestAllowLeaseExpired(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name	string
		lease	*allowleasegate.AllowLease
		at	time.Time
		expired	bool
	}{
		{"nil lease", nil, now, false},
		{"future expiry", &allowleasegate.AllowLease{ExpiresAt: now.Add(5 * time.Minute)}, now, false},
		{"past expiry", &allowleasegate.AllowLease{ExpiresAt: now.Add(-1 * time.Minute)}, now, true},
		{"zero expiry", &allowleasegate.AllowLease{}, now, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.lease.Expired(tt.at); got != tt.expired {
				t.Fatalf("want %v, got %v", tt.expired, got)
			}
		})
	}
}

func TestAllowLeaseAlive(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name	string
		lease	*allowleasegate.AllowLease
		alive	bool
	}{
		{"nil", nil, false},
		{"allowed+future", &allowleasegate.AllowLease{State: allowleasegate.StateAllowed, ExpiresAt: now.Add(5 * time.Minute)}, true},
		{"bound+future", &allowleasegate.AllowLease{State: allowleasegate.StateBound, ExpiresAt: now.Add(5 * time.Minute)}, true},
		{"consumed", &allowleasegate.AllowLease{State: allowleasegate.StateConsumed, ExpiresAt: now.Add(5 * time.Minute)}, false},
		{"expired state", &allowleasegate.AllowLease{State: allowleasegate.StateExpired, ExpiresAt: now.Add(-1 * time.Minute)}, false},
		{"allowed but ttl expired", &allowleasegate.AllowLease{State: allowleasegate.StateAllowed, ExpiresAt: now.Add(-1 * time.Second)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.lease.Alive(now); got != tt.alive {
				t.Fatalf("want %v, got %v", tt.alive, got)
			}
		})
	}
}

// ──────────────────────────────────────────
// TestCaseSnapshot helpers
// ──────────────────────────────────────────

func TestCaseSnapshotMethods(t *testing.T) {
	tests := []struct {
		state		string
		isAllowed	bool
		isDenied	bool
		isExpired	bool
		isOpen		bool
	}{
		{"approved", true, false, false, false},
		{"allow", true, false, false, false},
		{"denied", false, true, false, false},
		{"expired", false, false, true, false},
		{"open", false, false, false, true},
		{"assigned", false, false, false, true},
		{"unknown", false, false, false, false},
	}
	for _, tt := range tests {
		cs := allowleasegate.CaseSnapshot{State: tt.state}
		if cs.IsAllowed() != tt.isAllowed {
			t.Fatalf("%s: IsAllowed want %v", tt.state, tt.isAllowed)
		}
		if cs.IsDenied() != tt.isDenied {
			t.Fatalf("%s: IsDenied want %v", tt.state, tt.isDenied)
		}
		if cs.IsExpired() != tt.isExpired {
			t.Fatalf("%s: IsExpired want %v", tt.state, tt.isExpired)
		}
		if cs.IsOpen() != tt.isOpen {
			t.Fatalf("%s: IsOpen want %v", tt.state, tt.isOpen)
		}
	}
}

// ──────────────────────────────────────────
// TestStore MemStore 操作
// ──────────────────────────────────────────

func TestMemStoreLease(t *testing.T) {
	store := allowleasegate.NewMemStore()
	lease := &allowleasegate.AllowLease{
		MovementID:	"MVT-S01", UserID: "U-S01", Asset: "USDT",
		State:	allowleasegate.StateAllowed,
	}
	if err := store.SaveLease(lease); err != nil {
		t.Fatalf("SaveLease: %v", err)
	}
	if lease.ID == "" {
		t.Fatal("ID should be auto-generated")
	}
	got, err := store.GetLease(lease.ID)
	if err != nil {
		t.Fatalf("GetLease: %v", err)
	}
	if got.MovementID != "MVT-S01" {
		t.Fatalf("want MVT-S01, got %s", got.MovementID)
	}
	gotByMove, err := store.GetLeaseByMovement("MVT-S01")
	if err != nil {
		t.Fatalf("GetLeaseByMovement: %v", err)
	}
	if gotByMove.ID != lease.ID {
		t.Fatal("should find by movement")
	}
	_, err = store.GetLease("nonexistent")
	if !errors.Is(err, allowleasegate.ErrLeaseNotFound) {
		t.Fatalf("want ErrLeaseNotFound, got %v", err)
	}
}

func TestMemStoreProof(t *testing.T) {
	store := allowleasegate.NewMemStore()
	proof := &allowleasegate.AllowDriftProof{
		MovementID:	"MVT-P01", UserID: "U-P01", DriftKind: "no_lease",
		Reason:	"test", Blocked: true, Sealed: true,
	}
	if err := store.SaveProof(proof); err != nil {
		t.Fatalf("SaveProof: %v", err)
	}
	got, err := store.GetProof(proof.ID)
	if err != nil {
		t.Fatalf("GetProof: %v", err)
	}
	if got.DriftKind != "no_lease" {
		t.Fatalf("want no_lease, got %s", got.DriftKind)
	}
	list, err := store.ListProofs("MVT-P01", 10)
	if err != nil {
		t.Fatalf("ListProofs: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1, got %d", len(list))
	}
}

func TestMemStorePolicyBind(t *testing.T) {
	store := allowleasegate.NewMemStore()
	bind := &allowleasegate.PolicyBind{
		LeaseID:	"L-PB01", PolicyVer: 3, Decision: "allow",
	}
	if err := store.SavePolicyBind(bind); err != nil {
		t.Fatalf("SavePolicyBind: %v", err)
	}
	got, err := store.GetPolicyBind("L-PB01")
	if err != nil {
		t.Fatalf("GetPolicyBind: %v", err)
	}
	if got.PolicyVer != 3 {
		t.Fatalf("want 3, got %d", got.PolicyVer)
	}
}

func TestMemStoreListActive(t *testing.T) {
	store := allowleasegate.NewMemStore()
	for i := 0; i < 3; i++ {
		_ = store.SaveLease(&allowleasegate.AllowLease{
			MovementID:	fmt.Sprintf("MVT-ACT-%d", i),
			UserID:		"U-ACT",
			State:		allowleasegate.StateAllowed,
			IssuedAt:	time.Now().UTC().Add(time.Duration(i) * time.Second),
		})
	}
	_ = store.SaveLease(&allowleasegate.AllowLease{
		MovementID:	"MVT-TERM",
		UserID:		"U-ACT",
		State:		allowleasegate.StateConsumed,
	})
	active, err := store.ListActive()
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(active) != 3 {
		t.Fatalf("want 3 active, got %d", len(active))
	}
}

func TestMemStoreListByUser(t *testing.T) {
	store := allowleasegate.NewMemStore()
	for i := 0; i < 5; i++ {
		_ = store.SaveLease(&allowleasegate.AllowLease{
			MovementID:	fmt.Sprintf("MVT-USR-%d", i),
			UserID:		"U-LST",
			State:		allowleasegate.StateAllowed,
			IssuedAt:	time.Now().UTC().Add(time.Duration(i) * time.Second),
		})
	}
	_ = store.SaveLease(&allowleasegate.AllowLease{
		MovementID:	"MVT-USR-OTHER",
		UserID:		"U-OTHER",
		State:		allowleasegate.StateAllowed,
	})
	list, err := store.ListByUser("U-LST", 10)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(list) != 5 {
		t.Fatalf("want 5, got %d", len(list))
	}
}

func TestNilStore(t *testing.T) {
	var s *allowleasegate.MemStore
	_, err := s.GetLease("x")
	if !errors.Is(err, allowleasegate.ErrStoreNil) {
		t.Fatalf("want ErrStoreNil, got %v", err)
	}
}

// ──────────────────────────────────────────
// TestAuditTrail 审计追踪
// ──────────────────────────────────────────

func TestAuditTrail(t *testing.T) {
	trail := allowleasegate.NewAuditTrail()
	for i := 0; i < 10; i++ {
		trail.Append(allowleasegate.AuditRecord{
			LeaseID:	fmt.Sprintf("L-%d", i%3),
			Action:		fmt.Sprintf("action-%d", i),
		})
	}
	all := trail.List("", 100)
	if len(all) != 10 {
		t.Fatalf("want 10, got %d", len(all))
	}
	filtered := trail.List("L-0", 100)
	if len(filtered) != 4 {
		t.Fatalf("want 4 for L-0, got %d", len(filtered))
	}
	limited := trail.List("", 3)
	if len(limited) != 3 {
		t.Fatalf("want 3, got %d", len(limited))
	}
	if trail.Count() != 10 {
		t.Fatalf("want count 10, got %d", trail.Count())
	}
}

func TestAuditTrailNil(t *testing.T) {
	var trail *allowleasegate.AuditTrail
	trail.Append(allowleasegate.AuditRecord{Action: "test"})
	if trail.List("", 10) != nil {
		t.Fatal("nil trail should return nil")
	}
	if trail.Count() != 0 {
		t.Fatal("nil trail count should be 0")
	}
}

// ──────────────────────────────────────────
// TestExportProofs 凭证导出
// ──────────────────────────────────────────

func TestExportProofs(t *testing.T) {
	eng := testEngine(nil)
	for i := 0; i < 3; i++ {
		_, _ = eng.SubmitCheck(context.Background(), allowleasegate.SubmitReq{
			LeaseID:	"", MovementID: fmt.Sprintf("MVT-EP-%d", i), UserID: "U-EP",
		})
	}
	proofs, err := eng.ExportProofs(context.Background(), allowleasegate.ProofExportReq{UserID: "U-EP"})
	if err != nil {
		t.Fatalf("ExportProofs: %v", err)
	}
	if len(proofs) != 3 {
		t.Fatalf("want 3 proofs, got %d", len(proofs))
	}
}

// ──────────────────────────────────────────
// TestNilEngine 空引擎防护
// ──────────────────────────────────────────

func TestNilEngine(t *testing.T) {
	var eng *allowleasegate.Engine
	_, err := eng.Issue(context.Background(), defaultIssueReq())
	if !errors.Is(err, allowleasegate.ErrEngineNil) {
		t.Fatalf("want ErrEngineNil, got %v", err)
	}
	_, err = eng.SubmitCheck(context.Background(), allowleasegate.SubmitReq{})
	if !errors.Is(err, allowleasegate.ErrEngineNil) {
		t.Fatalf("want ErrEngineNil, got %v", err)
	}
	_, err = eng.Consume(context.Background(), "x")
	if !errors.Is(err, allowleasegate.ErrEngineNil) {
		t.Fatalf("want ErrEngineNil, got %v", err)
	}
	_, err = eng.Revoke(context.Background(), allowleasegate.RevokeReq{LeaseID: "x"})
	if !errors.Is(err, allowleasegate.ErrEngineNil) {
		t.Fatalf("want ErrEngineNil, got %v", err)
	}
	_, err = eng.ExpireStale(context.Background())
	if !errors.Is(err, allowleasegate.ErrEngineNil) {
		t.Fatalf("want ErrEngineNil, got %v", err)
	}
	_, err = eng.PatrolCases(context.Background())
	if !errors.Is(err, allowleasegate.ErrEngineNil) {
		t.Fatalf("want ErrEngineNil, got %v", err)
	}
	_, err = eng.ExportProofs(context.Background(), allowleasegate.ProofExportReq{})
	if !errors.Is(err, allowleasegate.ErrEngineNil,
	) {
		t.Fatalf("want ErrEngineNil, got %v", err)
	}
	_, err = eng.GetLease(context.Background(), "x")
	if !errors.Is(err, allowleasegate.ErrEngineNil) {
		t.Fatalf("want ErrEngineNil, got %v", err)
	}
	_, err = eng.GetLeaseByMovement(context.Background(), "x")
	if !errors.Is(err, allowleasegate.ErrEngineNil) {
		t.Fatalf("want ErrEngineNil, got %v", err)
	}
}

// ──────────────────────────────────────────
// TestConcurrentSubmitRace 并发提交竞争
// ──────────────────────────────────────────

func TestConcurrentSubmitRace(t *testing.T) {
	eng := testEngine(nil)
	lease := mustIssue(t, eng, defaultIssueReq())
	const n = 20
	var wg sync.WaitGroup
	var allowed int64
	var rejected int64
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			result, err := eng.SubmitCheck(context.Background(), allowleasegate.SubmitReq{
				LeaseID:	lease.ID, MovementID: "MVT-TEST-001", UserID: "USER-TEST-001",
			})
			if err == nil && result.Allowed {
				atomic.AddInt64(&allowed, 1)
			} else {
				atomic.AddInt64(&rejected, 1)
			}
		}()
	}
	wg.Wait()
	if allowed < 1 {
		t.Fatal("at least one submit should be allowed")
	}
	t.Logf("concurrent submit race: allowed=%d rejected=%d", allowed, rejected)
}

// ──────────────────────────────────────────
// TestFullLifecycle 完整生命周期
// ──────────────────────────────────────────

func TestFullLifecycleHappyPath(t *testing.T) {
	eng := testEngine(nil)
	lease := mustIssue(t, eng, defaultIssueReq())
	if lease.State != allowleasegate.StateAllowed {
		t.Fatalf("want allowed, got %s", lease.State)
	}
	result, err := eng.SubmitCheck(context.Background(), allowleasegate.SubmitReq{
		LeaseID:	lease.ID, MovementID: "MVT-TEST-001", UserID: "USER-TEST-001",
	})
	if err != nil || !result.Allowed {
		t.Fatalf("submit should be allowed: err=%v allowed=%v", err, result.Allowed)
	}
	if result.Lease.State != allowleasegate.StateBound {
		t.Fatalf("want bound, got %s", result.Lease.State)
	}
	consumed, err := eng.Consume(context.Background(), lease.ID)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if consumed.State != allowleasegate.StateConsumed {
		t.Fatalf("want consumed, got %s", consumed.State)
	}
	_, err = eng.Consume(context.Background(), lease.ID)
	if err == nil {
		t.Fatal("double consume should fail")
	}
}

func TestFullLifecycleExpirePath(t *testing.T) {
	eng := testEngine(nil)
	past := time.Now().UTC().Add(-10 * time.Minute)
	eng.SetClock(func() time.Time { return past })
	req := defaultIssueReq()
	req.TTL = 1 * time.Minute
	lease := mustIssue(t, eng, req)
	eng.SetClock(func() time.Time { return time.Now().UTC() })
	expired, err := eng.ExpireStale(context.Background())
	if err != nil {
		t.Fatalf("ExpireStale: %v", err)
	}
	if len(expired) != 1 || expired[0] != lease.ID {
		t.Fatalf("unexpected expired: %v", expired)
	}
	result, err := eng.SubmitCheck(context.Background(), allowleasegate.SubmitReq{
		LeaseID:	lease.ID, MovementID: "MVT-TEST-001", UserID: "USER-TEST-001",
	})
	if !errors.Is(err, allowleasegate.ErrLeaseExpired) {
		t.Fatalf("want ErrLeaseExpired, got %v", err)
	}
	if result.Allowed {
		t.Fatal("should not be allowed")
	}
	if result.Proof == nil || result.Proof.DriftKind != "lease_expired" {
		t.Fatal("expected lease_expired proof")
	}
}

func TestFullLifecycleRevokePath(t *testing.T) {
	cq := newMockCaseQuerier()
	cq.SetCase(allowleasegate.CaseSnapshot{ID: "C-FLR", MovementID: "MVT-FLR", State: "approved"})
	eng := testEngine(cq)
	lease := mustIssue(t, eng, allowleasegate.IssueReq{
		MovementID:	"MVT-FLR", UserID: "U-FLR", Asset: "USDT", Chain: "TRC20",
		ToAddress:	"TAddr-FLR", Amount: "100", Decision: "allow", PolicyVer: 1, CaseID: "C-FLR",
	})
	cq.SetCase(allowleasegate.CaseSnapshot{ID: "C-FLR", MovementID: "MVT-FLR", State: "denied", DecidedBy: "rev-B"})
	pr, err := eng.PatrolCases(context.Background())
	if err != nil {
		t.Fatalf("PatrolCases: %v", err)
	}
	if len(pr.Revoked) != 1 {
		t.Fatalf("want 1 revoked, got %d", len(pr.Revoked))
	}
	result, err := eng.SubmitCheck(context.Background(), allowleasegate.SubmitReq{
		LeaseID:	lease.ID, MovementID: "MVT-FLR", UserID: "U-FLR",
	})
	if !errors.Is(err, allowleasegate.ErrLeaseRevoked) {
		t.Fatalf("want ErrLeaseRevoked, got %v", err)
	}
	if result.Allowed {
		t.Fatal("should not be allowed")
	}
}

// ──────────────────────────────────────────
// TestDuration JSON 序列化
// ──────────────────────────────────────────

func TestDurationJSON(t *testing.T) {
	d := allowleasegate.Duration(5 * time.Minute)
	b, err := d.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if string(b) != `"5m0s"` {
		t.Fatalf("want \"5m0s\", got %s", string(b))
	}
	var d2 allowleasegate.Duration
	if err := d2.UnmarshalJSON(b); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if d2.Std() != 5*time.Minute {
		t.Fatalf("want 5m, got %v", d2.Std())
	}
}

// ──────────────────────────────────────────
// TestConfigDefaults 配置默认值
// ──────────────────────────────────────────

func TestConfigDefaults(t *testing.T) {
	cfg := allowleasegate.Config{}.withDefaults()
	if cfg.DefaultTTL != 5*time.Minute {
		t.Fatalf("want 5m DefaultTTL, got %v", cfg.DefaultTTL)
	}
	if cfg.MaxTTL != 30*time.Minute {
		t.Fatalf("want 30m MaxTTL, got %v", cfg.MaxTTL)
	}
	if cfg.PatrolInterval != 10*time.Second {
		t.Fatalf("want 10s PatrolInterval, got %v", cfg.PatrolInterval)
	}
	if cfg.ExpireInterval != 5*time.Second {
		t.Fatalf("want 5s ExpireInterval, got %v", cfg.ExpireInterval)
	}
	if cfg.MaxProofsExport != 500 {
		t.Fatalf("want 500 MaxProofsExport, got %d", cfg.MaxProofsExport)
	}
}

// ──────────────────────────────────────────
// TestIssueTTLClamping 签发 TTL 限制
// ──────────────────────────────────────────

func TestIssueTTLClamping(t *testing.T) {
	eng := testEngine(nil)
	lease,
		err := eng.Issue(context.Background(), allowleasegate.IssueReq{
		MovementID:	"MVT-TTL-CLAMP", UserID: "U-TTL", Asset: "USDT", Chain: "TRC20",
		ToAddress:	"TAddr-TTL", Amount: "100", Decision: "allow", PolicyVer: 1,
		TTL:	2 * time.Hour,
	})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if lease.TTL.Std() > 30*time.Minute {
		t.Fatalf("TTL should be clamped to MaxTTL, got %v", lease.TTL.Std())
	}
}

func TestIssueDefaultTTL(t *testing.T) {
	eng := testEngine(nil)
	lease, err := eng.Issue(context.Background(), allowleasegate.IssueReq{
		MovementID:	"MVT-TTL-DEF", UserID: "U-TTL", Asset: "USDT", Chain: "TRC20",
		ToAddress:	"TAddr-TTL", Amount: "100", Decision: "allow", PolicyVer: 1,
	})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if lease.TTL.Std() != 5*time.Minute {
		t.Fatalf("want 5m default TTL, got %v", lease.TTL.Std())
	}
}

// ──────────────────────────────────────────
// TestPolicyBindCreated 策略绑定快照
// ──────────────────────────────────────────

func TestPolicyBindCreated(t *testing.T) {
	eng := testEngine(nil)
	lease := mustIssue(t, eng, allowleasegate.IssueReq{
		MovementID:	"MVT-PB", UserID: "U-PB", Asset: "USDT", Chain: "TRC20",
		ToAddress:	"TAddr-PB", Amount: "100", Decision: "allow", PolicyVer: 7,
		Hook:	"kyt",
	})
	bind,
		err := eng.store.GetPolicyBind(lease.ID)
	if err != nil {
		t.Fatalf("GetPolicyBind: %v", err)
	}
	if bind.PolicyVer != 7 {
		t.Fatalf("want policy_ver 7, got %d", bind.PolicyVer)
	}
	if bind.Decision != "allow" {
		t.Fatalf("want decision allow, got %s", bind.Decision)
	}
}

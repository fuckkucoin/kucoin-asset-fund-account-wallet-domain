package feereserve_test

import (
	"context"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/feereserve"
)

// ══════════════════════════════════════════
// FSM
// ══════════════════════════════════════════

func TestLeaseFSM(t *testing.T) {
	type testCase struct {
		name	string
		from	feereserve.LeaseState
		event	string
		want	feereserve.LeaseState
		wantErr	bool
	}
	cases := []testCase{
		{"reserved_settle", feereserve.LeaseReserved, feereserve.LeaseEvSettle, feereserve.LeaseSettled, false},
		{"reserved_release", feereserve.LeaseReserved, feereserve.LeaseEvRelease, feereserve.LeaseReleased, false},
		{"reserved_block", feereserve.LeaseReserved, feereserve.LeaseEvBlock, feereserve.LeaseBlocked, false},
		{"settled_block", feereserve.LeaseSettled, feereserve.LeaseEvBlock, feereserve.LeaseBlocked, false},
		{"released_block", feereserve.LeaseReleased, feereserve.LeaseEvBlock, feereserve.LeaseBlocked, false},
		{"settled_settle_illegal", feereserve.LeaseSettled, feereserve.LeaseEvSettle, feereserve.LeaseSettled, true},
		{"settled_release_illegal", feereserve.LeaseSettled, feereserve.LeaseEvRelease, feereserve.LeaseSettled, true},
		{"released_settle_illegal", feereserve.LeaseReleased, feereserve.LeaseEvSettle, feereserve.LeaseReleased, true},
		{"released_release_illegal", feereserve.LeaseReleased, feereserve.LeaseEvRelease, feereserve.LeaseReleased, true},
		{"blocked_settle_illegal", feereserve.LeaseBlocked, feereserve.LeaseEvSettle, feereserve.LeaseBlocked, true},
		{"blocked_release_illegal", feereserve.LeaseBlocked, feereserve.LeaseEvRelease, feereserve.LeaseBlocked, true},
		{"blocked_block_illegal", feereserve.LeaseBlocked, feereserve.LeaseEvBlock, feereserve.LeaseBlocked, true},
	}
	m := feereserve.LeaseFSM()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := m.Apply(tc.from, tc.event)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got state %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("expected %s, got %s", tc.want, got)
			}
		})
	}
}

func TestCaseFSM(t *testing.T) {
	type testCase struct {
		name	string
		from	feereserve.CaseState
		event	string
		want	feereserve.CaseState
		wantErr	bool
	}
	cases := []testCase{
		{"open_prove", feereserve.CaseOpen, feereserve.CaseEvProve, feereserve.CaseProven, false},
		{"open_close", feereserve.CaseOpen, feereserve.CaseEvClose, feereserve.CaseClosed, false},
		{"proven_close", feereserve.CaseProven, feereserve.CaseEvClose, feereserve.CaseClosed, false},
		{"proven_prove_illegal", feereserve.CaseProven, feereserve.CaseEvProve, feereserve.CaseProven, true},
		{"closed_prove_illegal", feereserve.CaseClosed, feereserve.CaseEvProve, feereserve.CaseClosed, true},
		{"closed_close_illegal", feereserve.CaseClosed, feereserve.CaseEvClose, feereserve.CaseClosed, true},
	}
	m := feereserve.CaseFSM()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got,
				err := m.Apply(tc.from, tc.event)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got state %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("expected %s, got %s", tc.want, got)
			}
		})
	}
}

// ══════════════════════════════════════════
// Terminal 与 Expiry
// ══════════════════════════════════════════

func TestLeaseIsTerminal(t *testing.T) {
	cases := []struct {
		state	feereserve.LeaseState
		want	bool
	}{
		{feereserve.LeaseReserved, false},
		{feereserve.LeaseSettled, true},
		{feereserve.LeaseReleased, true},
		{feereserve.LeaseBlocked, true},
	}
	for _, tc := range cases {
		if got := (feereserve.ReserveLease{State: tc.state}).IsTerminal(); got != tc.want {
			t.Errorf("state %s: IsTerminal()=%v, want %v", tc.state, got, tc.want)
		}
	}
}

func TestLeaseExpired(t *testing.T) {
	expiry := time.Date(2021, 6, 22, 10, 10, 0, 0, time.UTC)
	lease := feereserve.ReserveLease{LeaseExpiry: expiry}
	before := time.Date(2021, 6, 22, 10, 9, 0, 0, time.UTC)
	if lease.Expired(before) {
		t.Error("should not be expired before expiry")
	}
	after := time.Date(2021,
		6, 22, 10, 11, 0, 0, time.UTC)
	if !lease.Expired(after) {
		t.Error("should be expired after expiry")
	}
}

// ══════════════════════════════════════════
// MemStore
// ══════════════════════════════════════════

func TestMemStoreLeaseCRUD(t *testing.T) {
	s := feereserve.NewMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	lease := feereserve.ReserveLease{
		ID:	"L-1", WithdrawID: "WD-1", UserID: "U-1", Asset: "USDT",
		ReservedFee:	"10", Generation: 1, Token: "tok-1",
		State:	feereserve.LeaseReserved, LeaseExpiry: now.Add(10 * time.Minute),
		CreatedAt:	now, UpdatedAt: now,
	}
	saved, err := s.SaveLease(ctx, lease)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID != "L-1" {
		t.Errorf("expected L-1, got %s", saved.ID)
	}
	got, err := s.GetLease(ctx, "L-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.WithdrawID != "WD-1" {
		t.Errorf("expected WD-1, got %s", got.WithdrawID)
	}
	got2, err := s.GetLeaseByWithdraw(ctx, "WD-1")
	if err != nil {
		t.Fatal(err)
	}
	if got2.ID != "L-1" {
		t.Errorf("expected L-1, got %s", got2.ID)
	}
	_, err = s.GetLease(ctx, "NONEXIST")
	if err != feereserve.ErrLeaseNotFound {
		t.Errorf("expected ErrLeaseNotFound, got %v", err)
	}
	_, err = s.GetLeaseByWithdraw(ctx, "NONEXIST")
	if err != feereserve.ErrLeaseNotFound {
		t.Errorf("expected ErrLeaseNotFound, got %v", err)
	}
}

func TestMemStoreTerminalBind(t *testing.T) {
	s := feereserve.NewMemStore()
	ctx := context.Background()
	_, found, err := s.GetTerminalBind(ctx, "WD-X")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("expected not found")
	}
	err = s.SaveTerminalBind(ctx, feereserve.TerminalBind{
		WithdrawID:	"WD-X", Leg: feereserve.LegSettle, LeaseID: "L-X",
		Generation:	1, Token: "tok", BoundAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	tb, found, err := s.GetTerminalBind(ctx, "WD-X")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("expected found")
	}
	if tb.Leg != feereserve.LegSettle {
		t.Errorf("expected settle, got %s", tb.Leg)
	}
}

func TestMemStoreProofCRUD(t *testing.T) {
	s := feereserve.NewMemStore()
	ctx := context.Background()
	proof := feereserve.DeltaProof{
		ID:	"P-1", WithdrawID: "WD-1", LeaseID: "L-1",
		ReservedFee:	"10", ActualFee: "8", DeltaAmt: "2",
		DeltaSign:	"undercharge", CreatedAt: time.Now(),
	}
	_, err := s.SaveProof(ctx, proof)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProof(ctx, "P-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.DeltaSign != "undercharge" {
		t.Errorf("expected undercharge, got %s", got.DeltaSign)
	}
	list, err := s.ListProofsByWithdraw(ctx, "WD-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 proof, got %d", len(list))
	}
	listL, err := s.ListProofsByLease(ctx, "L-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(listL) != 1 {
		t.Errorf("expected 1 proof by lease, got %d", len(listL))
	}
}

func TestMemStoreCaseCRUD(t *testing.T) {
	s := feereserve.NewMemStore()
	ctx := context.Background()
	rc := feereserve.ReserveCase{
		ID:	"C-1", WithdrawID: "WD-1", LeaseID: "L-1",
		Kind:	feereserve.CaseUndercharge, State: feereserve.CaseOpen,
		Detail:	"delta=2", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	_, err := s.SaveCase(ctx, rc)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetCase(ctx, "C-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != feereserve.CaseUndercharge {
		t.Errorf("expected undercharge, got %s", got.Kind)
	}
	byState,
		err := s.ListCasesByState(ctx, feereserve.CaseOpen, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(byState) != 1 {
		t.Errorf("expected 1, got %d", len(byState))
	}
	byWd, err := s.ListCasesByWithdraw(ctx, "WD-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(byWd) != 1 {
		t.Errorf("expected 1, got %d", len(byWd))
	}
}

func TestMemStoreListLeases(t *testing.T) {
	s := feereserve.NewMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	for i, st := range []feereserve.LeaseState{feereserve.LeaseReserved, feereserve.LeaseSettled, feereserve.LeaseReserved} {
		l := feereserve.ReserveLease{
			ID:	"L-" + string(rune('A'+i)), WithdrawID: "WD-" + string(rune('A'+i)),
			State:		st,
			LeaseExpiry:	now.Add(-5 * time.Minute),
			CreatedAt:	now, UpdatedAt: now,
		}
		_, _ = s.SaveLease(ctx, l)
	}
	reserved, err := s.ListLeasesByState(ctx, feereserve.LeaseReserved, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(reserved) != 2 {
		t.Errorf("expected 2 reserved, got %d", len(reserved))
	}
	expired, err := s.ListExpiredLeases(ctx, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 2 {
		t.Errorf("expected 2 expired, got %d", len(expired))
	}
}

func TestNewLeaseToken(t *testing.T) {
	tok := feereserve.NewLeaseToken()
	if len(tok) != 32 {
		t.Errorf("expected 32 char hex, got %d chars", len(tok))
	}
	tok2 := feereserve.NewLeaseToken()
	if tok == tok2 {
		t.Error("tokens should be unique")
	}
}

func TestReserveCaseCopy(t *testing.T) {
	rc := feereserve.ReserveCase{
		ID:	"C-1", ProofIDs: []string{"P-1", "P-2"},
	}
	cp := rc.Copy()
	cp.ProofIDs[0] = "CHANGED"
	if rc.ProofIDs[0] == "CHANGED" {
		t.Error("copy should not share slice")
	}
}

func TestAllDDL(t *testing.T) {
	ddls := feereserve.AllDDL()
	if len(ddls) != 4 {
		t.Errorf("expected 4 DDL statements, got %d", len(ddls))
	}
	for i, d := range ddls {
		if !strings.Contains(d, "CREATE TABLE") {
			t.Errorf("DDL %d missing CREATE TABLE", i)
		}
	}
}

package loanrace_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/loanrace"
)

// ──────────────────────────────────────────────────────────────
// 辅助：可控时钟
// ──────────────────────────────────────────────────────────────

type testClock struct {
	mu	sync.Mutex
	now	time.Time
}

func newTestClock(t time.Time) *testClock {
	return &testClock{now: t}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTestService(clk *testClock, ttl time.Duration) *loanrace.Service {
	return loanrace.New(loanrace.WithClock(clk.Now), loanrace.WithLeaseTTL(ttl))
}

// ──────────────────────────────────────────────────────────────
// 表驱动：FSM 边验证
// ──────────────────────────────────────────────────────────────

func TestLoanRaceFSMTransitions(t *testing.T) {
	m := loanrace.LoanRaceFSM()
	tests := []struct {
		name	string
		from	loanrace.LoanRaceState
		event	string
		wantTo	loanrace.LoanRaceState
		wantErr	bool
	}{
		{"active→overdue", loanrace.LRActive, loanrace.LrEvOverdueDetected, loanrace.LROverdue, false},
		{"overdue→frozen", loanrace.LROverdue, loanrace.LrEvFreezeApplied, loanrace.LRFrozen, false},
		{"frozen→repaying", loanrace.LRFrozen, loanrace.LrEvRepayStart, loanrace.LRRepaying, false},
		{"frozen→written_off", loanrace.LRFrozen, loanrace.LrEvWriteoffApplied, loanrace.LRWrittenOff, false},
		{"repaying→closed", loanrace.LRRepaying, loanrace.LrEvRepayDone, loanrace.LRClosed, false},
		{"repaying→frozen", loanrace.LRRepaying, loanrace.LrEvRepayFail, loanrace.LRFrozen, false},
		{"active→frozen illegal", loanrace.LRActive, loanrace.LrEvFreezeApplied, loanrace.LRActive, true},
		{"closed→anything illegal", loanrace.LRClosed, loanrace.LrEvRepayStart, loanrace.LRClosed, true},
		{"written_off→anything illegal", loanrace.LRWrittenOff, loanrace.LrEvRepayStart, loanrace.LRWrittenOff, true},
		{"active→repay illegal", loanrace.LRActive, loanrace.LrEvRepayStart, loanrace.LRActive, true},
		{"overdue→repay illegal", loanrace.LROverdue, loanrace.LrEvRepayStart, loanrace.LROverdue, true},
		{"active→writeoff illegal", loanrace.LRActive, loanrace.LrEvWriteoffApplied, loanrace.LRActive, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.Apply(tt.from, tt.event)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s + %s", tt.from, tt.event)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantTo {
				t.Fatalf("got %s, want %s", got, tt.wantTo)
			}
		})
	}
}

func TestFreezeLeaseFSMTransitions(t *testing.T) {
	m := loanrace.FreezeLeaseFSM()
	tests := []struct {
		name	string
		from	loanrace.LeaseState
		event	string
		wantTo	loanrace.LeaseState
		wantErr	bool
	}{
		{"acquired→applied", loanrace.LeaseAcquired, loanrace.LeaseEvApply, loanrace.LeaseApplied, false},
		{"acquired→expired", loanrace.LeaseAcquired, loanrace.LeaseEvExpire, loanrace.LeaseExpired, false},
		{"applied→released", loanrace.LeaseApplied, loanrace.LeaseEvRelease, loanrace.LeaseReleased, false},
		{"applied→expired", loanrace.LeaseApplied, loanrace.LeaseEvExpire, loanrace.LeaseExpired, false},
		{"released→anything illegal", loanrace.LeaseReleased, loanrace.LeaseEvApply, loanrace.LeaseReleased, true},
		{"expired→anything illegal", loanrace.LeaseExpired, loanrace.LeaseEvRelease, loanrace.LeaseExpired, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.Apply(tt.from, tt.event)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s + %s", tt.from, tt.event)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantTo {
				t.Fatalf("got %s, want %s", got, tt.wantTo)
			}
		})
	}
}

func TestPayGateFSMTransitions(t *testing.T) {
	m := loanrace.PayGateFSM()
	tests := []struct {
		name	string
		from	loanrace.GateState
		event	string
		wantTo	loanrace.GateState
		wantErr	bool
	}{
		{"open→cleared", loanrace.GateOpen, loanrace.GateEvClear, loanrace.GateCleared, false},
		{"open→blocked", loanrace.GateOpen, loanrace.GateEvBlock, loanrace.GateBlocked, false},
		{"cleared→closed", loanrace.GateCleared, loanrace.GateEvClose, loanrace.GateClosed, false},
		{"closed→illegal", loanrace.GateClosed, loanrace.GateEvClear, loanrace.GateClosed, true},
		{"blocked→illegal", loanrace.GateBlocked, loanrace.GateEvClose, loanrace.GateBlocked, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.Apply(tt.from, tt.event)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s + %s", tt.from, tt.event)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantTo {
				t.Fatalf("got %s, want %s", got, tt.wantTo)
			}
		})
	}
}

func TestWriteoffFSMTransitions(t *testing.T) {
	m := loanrace.WriteoffFSM()
	tests := []struct {
		name	string
		from	loanrace.WriteoffState
		event	string
		wantTo	loanrace.WriteoffState
		wantErr	bool
	}{
		{"pending→approved", loanrace.WOPending, loanrace.WoEvApprove, loanrace.WOApproved, false},
		{"pending→rejected", loanrace.WOPending, loanrace.WoEvReject, loanrace.WORejected, false},
		{"approved→applied", loanrace.WOApproved, loanrace.WoEvApply, loanrace.WOApplied, false},
		{"applied→illegal", loanrace.WOApplied, loanrace.WoEvApprove, loanrace.WOApplied, true},
		{"rejected→illegal", loanrace.WORejected, loanrace.WoEvApply, loanrace.WORejected, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.Apply(tt.from, tt.event)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s + %s", tt.from, tt.event)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantTo {
				t.Fatalf("got %s, want %s", got, tt.wantTo)
			}
		})
	}
}

func TestUnfreezeProofFSMTransitions(t *testing.T) {
	m := loanrace.UnfreezeProofFSM()
	tests := []struct {
		name	string
		from	loanrace.ProofState
		event	string
		wantTo	loanrace.ProofState
		wantErr	bool
	}{
		{"pending→valid", loanrace.ProofPending, loanrace.ProofEvValidate, loanrace.ProofValid, false},
		{"pending→rejected", loanrace.ProofPending, loanrace.ProofEvReject, loanrace.ProofRejected, false},
		{"valid→illegal", loanrace.ProofValid, loanrace.ProofEvValidate, loanrace.ProofValid, true},
		{"rejected→illegal", loanrace.ProofRejected, loanrace.ProofEvValidate, loanrace.ProofRejected, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.Apply(tt.from, tt.event)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s + %s", tt.from, tt.event)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantTo {
				t.Fatalf("got %s, want %s", got, tt.wantTo)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────
// Fixture 1: ScanOverdue vs Pay 同一贷款并发
// ──────────────────────────────────────────────────────────────

func TestScanOverdueVsPayConcurrent(t *testing.T) {
	clk := newTestClock(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 30*time.Second)
	svc.RegisterLoan("LOAN-001", "USER-A")
	lease, err := svc.FreezeForOverdue("LOAN-001", "USER-A", "scanner")
	if err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if lease.State != loanrace.LeaseApplied {
		t.Fatalf("lease state: got %s, want applied", lease.State)
	}
	_, err = svc.AcquireLease("LOAN-001", "USER-A", "pay_worker")
	if !errors.Is(err, loanrace.ErrLeaseConflict) {
		t.Fatalf("concurrent lease should conflict, got: %v", err)
	}
	state, err := svc.GetRaceState("LOAN-001")
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state != loanrace.LRFrozen {
		t.Fatalf("race state: got %s, want frozen", state)
	}
	if !svc.store.IsFrozen("LOAN-001") {
		t.Fatal("frozen bit should be set")
	}
}

func TestScanOverdueVsPayConcurrentGoroutines(t *testing.T) {
	clk := newTestClock(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 30*time.Second)
	svc.RegisterLoan("LOAN-RACE", "USER-RACE")
	var scanErr, payErr error
	var scanLease, payLease loanrace.OverdueLease
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanLease, scanErr = svc.FreezeForOverdue("LOAN-RACE", "USER-RACE", "scanner")
	}()
	go func() {
		defer wg.Done()
		payLease, payErr = svc.AcquireLease("LOAN-RACE", "USER-RACE", "pay_worker")
	}()
	wg.Wait()
	successCount := 0
	if scanErr == nil {
		successCount++
		_ = scanLease
	}
	if payErr == nil {
		successCount++
		_ = payLease
	}
	if successCount != 1 {
		t.Fatalf("exactly one should succeed; scanner=%v pay=%v", scanErr, payErr)
	}
}

// ──────────────────────────────────────────────────────────────
// Fixture 2: Frozen without PayGate → Pay must fail
// ──────────────────────────────────────────────────────────────

func TestFrozenWithoutGatePayFails(t *testing.T) {
	clk := newTestClock(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 30*time.Second)
	svc.RegisterLoan("LOAN-002", "USER-B")
	_, err := svc.FreezeForOverdue("LOAN-002", "USER-B", "scanner")
	if err != nil {
		t.Fatalf("freeze: %v", err)
	}
	err = svc.CheckPayAllowed("LOAN-002")
	if !errors.Is(err, loanrace.ErrGateBlocked) {
		t.Fatalf("expected gate blocked, got: %v", err)
	}
	_, err = svc.PayThroughGate("LOAN-002", "1000", "payer")
	if !errors.Is(err, loanrace.ErrGateBlocked) {
		t.Fatalf("expected gate blocked for pay, got: %v", err)
	}
}

// ──────────────────────────────────────────────────────────────
// Fixture 3: UnfreezeProof 必须验证后才能 clear Frozen
// ──────────────────────────────────────────────────────────────

func TestUnfreezeProofRequiredBeforeClear(t *testing.T) {
	clk := newTestClock(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 30*time.Second)
	svc.RegisterLoan("LOAN-003", "USER-C")
	_, err := svc.FreezeForOverdue("LOAN-003", "USER-C", "scanner")
	if err != nil {
		t.Fatalf("freeze: %v", err)
	}
	err = svc.ClearFrozenWithProof("LOAN-003", "ops")
	if !errors.Is(err, loanrace.ErrProofRequired) {
		t.Fatalf("expected proof required, got: %v", err)
	}
	proof, err := svc.SubmitUnfreezeProof("LOAN-003", "full_repay", "TX-0xABC", "ops", "")
	if err != nil {
		t.Fatalf("submit proof: %v", err)
	}
	err = svc.ClearFrozenWithProof("LOAN-003", "ops")
	if !errors.Is(err, loanrace.ErrProofRequired) {
		t.Fatalf("pending proof should not clear: %v", err)
	}
	_, err = svc.ValidateUnfreezeProof(proof.ID, "validator")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	err = svc.ClearFrozenWithProof("LOAN-003", "ops")
	if err != nil {
		t.Fatalf("clear with valid proof: %v", err)
	}
	state, _ := svc.GetRaceState("LOAN-003")
	if state != loanrace.LRClosed {
		t.Fatalf("state: got %s, want closed", state)
	}
	if svc.store.IsFrozen("LOAN-003") {
		t.Fatal("frozen bit should be cleared")
	}
}

// ──────────────────────────────────────────────────────────────
// Fixture 4: Writeoff → Pay rejected
// ──────────────────────────────────────────────────────────────

func TestWriteoffThenPayRejected(t *testing.T) {
	clk := newTestClock(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
	svc := newTestService(clk,
		30*time.Second)
	svc.RegisterLoan("LOAN-004", "USER-D")
	_, err := svc.FreezeForOverdue("LOAN-004", "USER-D", "scanner")
	if err != nil {
		t.Fatalf("freeze: %v", err)
	}
	wo, err := svc.ApplyWriteoff("LOAN-004", "20000", "1000", "bad debt 90d+", "applicant_a")
	if err != nil {
		t.Fatalf("apply writeoff: %v", err)
	}
	_, err = svc.ApproveWriteoff(wo.ID, "approver_b")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	_, err = svc.ExecuteWriteoff(wo.ID, "executor_c")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	state, _ := svc.GetRaceState("LOAN-004")
	if state != loanrace.LRWrittenOff {
		t.Fatalf("state: got %s, want written_off", state)
	}
	err = svc.CheckPayAllowed("LOAN-004")
	if !errors.Is(err, loanrace.ErrWrittenOff) {
		t.Fatalf("expected written off, got: %v", err)
	}
	_, err = svc.PayThroughGate("LOAN-004", "5000", "payer")
	if !errors.Is(err, loanrace.ErrWrittenOff) {
		t.Fatalf("expected written off for pay, got: %v", err)
	}
	entries := svc.SuspenseForLoan("LOAN-004")
	if len(entries) != 1 {
		t.Fatalf("suspense entries: got %d, want 1", len(entries))
	}
	if entries[0].Amount != "21000" {
		t.Fatalf("suspense amount: got %s, want 21000", entries[0].Amount)
	}
}

// ──────────────────────────────────────────────────────────────
// Fixture 5: 受控还款完整生命周期
// ──────────────────────────────────────────────────────────────

func TestControlledRepayFullLifecycle(t *testing.T) {
	clk := newTestClock(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 60*time.Second)
	svc.RegisterLoan("LOAN-005", "USER-E")
	lease, err := svc.FreezeForOverdue("LOAN-005", "USER-E", "scanner")
	if err != nil {
		t.Fatalf("freeze: %v", err)
	}
	gate, err := svc.StartControlledRepay("LOAN-005", lease.ID, "ops", "15750")
	if err != nil {
		t.Fatalf("start repay: %v", err)
	}
	if gate.State != loanrace.GateOpen {
		t.Fatalf("gate state: got %s, want open", gate.State)
	}
	state, _ := svc.GetRaceState("LOAN-005")
	if state != loanrace.LRRepaying {
		t.Fatalf("state: got %s, want repaying", state)
	}
	gate, err = svc.PayThroughGate("LOAN-005", "15750", "payer")
	if err != nil {
		t.Fatalf("pay: %v", err)
	}
	if gate.State != loanrace.GateCleared {
		t.Fatalf("gate state after pay: got %s, want cleared", gate.State)
	}
	gate, err = svc.CompleteRepay(gate.ID, "ops")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if gate.State != loanrace.GateClosed {
		t.Fatalf("gate state after complete: got %s, want closed", gate.State)
	}
	proof, err := svc.SubmitUnfreezeProof("LOAN-005", "full_repay", "TX-0xDEF456", "ops", "")
	if err != nil {
		t.Fatalf("submit proof: %v", err)
	}
	_, err = svc.ValidateUnfreezeProof(proof.ID, "validator")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	err = svc.ClearFrozenWithProof("LOAN-005", "ops")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	state, _ = svc.GetRaceState("LOAN-005")
	if state != loanrace.LRClosed {
		t.Fatalf("final state: got %s, want closed", state)
	}
	if svc.store.IsFrozen("LOAN-005") {
		t.Fatal("frozen bit should be cleared")
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：LeaseAcquire / Release
// ──────────────────────────────────────────────────────────────

func TestLeaseAcquireRelease(t *testing.T) {
	tests := []struct {
		name		string
		setup		func(svc *loanrace.Service, clk *testClock)
		action		func(svc *loanrace.Service,) error
		wantErr		error
		checkAfter	func(t *testing.T, svc *loanrace.Service,)
	}{
		{
			name:	"acquire on unregistered loan",
			setup: func(svc *loanrace.Service,
				clk *testClock) {
			},
			action: func(svc *loanrace.Service,) error {
				_, err := svc.AcquireLease("NONE", "U", "h")
				return err
			},
			wantErr:	loanrace.ErrLoanNotFound,
		},
		{
			name:	"acquire success",
			setup: func(svc *loanrace.Service, clk *testClock) {
				svc.RegisterLoan("L1", "U1")
			},
			action: func(svc *loanrace.Service,) error {
				l, err := svc.AcquireLease("L1", "U1", "scanner")
				if err != nil {
					return err
				}
				if l.State != loanrace.LeaseAcquired {
					t.Errorf("lease state %s", l.State)
				}
				return nil
			},
		},
		{
			name:	"double acquire conflicts",
			setup: func(svc *loanrace.Service, clk *testClock) {
				svc.RegisterLoan("L2", "U2")
				svc.AcquireLease("L2", "U2", "first")
			},
			action: func(svc *loanrace.Service,) error {
				_, err := svc.AcquireLease("L2", "U2", "second")
				return err
			},
			wantErr:	loanrace.ErrLeaseConflict,
		},
		{
			name:	"acquire after expiry succeeds",
			setup: func(svc *loanrace.Service, clk *testClock) {
				svc.RegisterLoan("L3", "U3")
				svc.AcquireLease("L3", "U3", "first")
				clk.Advance(60 * time.Second)
			},
			action: func(svc *loanrace.Service,) error {
				l, err := svc.AcquireLease("L3", "U3", "second")
				if err != nil {
					return err
				}
				if l.Holder != "second" {
					t.Errorf("holder: %s", l.Holder)
				}
				return nil
			},
		},
		{
			name:	"release applied lease",
			setup: func(svc *loanrace.Service, clk *testClock) {
				svc.RegisterLoan("L4", "U4")
			},
			action: func(svc *loanrace.Service,) error {
				l, err := svc.AcquireLease("L4", "U4", "h")
				if err != nil {
					return err
				}
				l, err = svc.ApplyLease(l.ID)
				if err != nil {
					return err
				}
				l, err = svc.ReleaseLease(l.ID)
				if err != nil {
					return err
				}
				if l.State != loanrace.LeaseReleased {
					t.Errorf("state: %s", l.State)
				}
				return nil
			},
		},
		{
			name:	"release already released",
			setup: func(svc *loanrace.Service, clk *testClock) {
				svc.RegisterLoan("L5", "U5")
			},
			action: func(svc *loanrace.Service,) error {
				l, _ := svc.AcquireLease("L5", "U5", "h")
				svc.ApplyLease(l.ID)
				svc.ReleaseLease(l.ID)
				_, err := svc.ReleaseLease(l.ID)
				return err
			},
			wantErr:	loanrace.ErrLeaseReleased,
		},
		{
			name:	"apply expired lease fails",
			setup: func(svc *loanrace.Service, clk *testClock) {
				svc.RegisterLoan("L6", "U6")
				svc.AcquireLease("L6", "U6", "h")
				clk.Advance(60 * time.Second)
			},
			action: func(svc *loanrace.Service,) error {
				leases := svc.store.AllLeases()
				for _, l := range leases {
					if l.LoanID == "L6" {
						_, err := svc.ApplyLease(l.ID)
						return err
					}
				}
				return loanrace.ErrLeaseNotFound
			},
			wantErr:	loanrace.ErrLeaseExpired,
		},
		{
			name:	"empty holder rejected",
			setup: func(svc *loanrace.Service, clk *testClock) {
				svc.RegisterLoan("L7", "U7")
			},
			action: func(svc *loanrace.Service,) error {
				_, err := svc.AcquireLease("L7", "U7", "")
				return err
			},
			wantErr:	loanrace.ErrActor,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newTestClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
			svc := newTestService(clk, 30*time.Second)
			tt.setup(svc, clk)
			err := tt.action(svc)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err: got %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if tt.checkAfter != nil {
				tt.checkAfter(t, svc)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：PayGate 互斥
// ──────────────────────────────────────────────────────────────

func TestPayGateMutex(t *testing.T) {
	tests := []struct {
		name	string
		setup	func(svc *loanrace.Service,) (leaseID string)
		action	func(svc *loanrace.Service, leaseID string) error
		wantErr	error
	}{
		{
			name:	"open gate on non-frozen loan",
			setup: func(svc *loanrace.Service,) string {
				svc.RegisterLoan("PG1", "U1")
				l, _ := svc.FreezeForOverdue("PG1", "U1", "scanner")
				return l.ID
			},
			action: func(svc *loanrace.Service, leaseID string) error {
				_, err := svc.OpenPayGate("PG1", leaseID, "ops", "1000")
				return err
			},
		},
		{
			name:	"pay without gate blocked",
			setup: func(svc *loanrace.Service,) string {
				svc.RegisterLoan("PG2", "U2")
				l, _ := svc.FreezeForOverdue("PG2", "U2", "scanner")
				return l.ID
			},
			action: func(svc *loanrace.Service, _ string) error {
				_, err := svc.PayThroughGate("PG2", "1000", "payer")
				return err
			},
			wantErr:	loanrace.ErrGateBlocked,
		},
		{
			name:	"pay through gate accumulates",
			setup: func(svc *loanrace.Service,) string {
				svc.RegisterLoan("PG3", "U3")
				l, _ := svc.FreezeForOverdue("PG3", "U3", "scanner")
				svc.OpenPayGate("PG3", l.ID, "ops", "5000")
				return l.ID
			},
			action: func(svc *loanrace.Service, _ string) error {
				g1, err := svc.PayThroughGate("PG3", "2000", "p")
				if err != nil {
					return err
				}
				if g1.PaidAmt != "2000" {
					t.Errorf("paid: %s", g1.PaidAmt)
				}
				g2, err := svc.PayThroughGate("PG3", "3000", "p")
				if err != nil {
					return err
				}
				if g2.State != loanrace.GateCleared {
					t.Errorf("state: %s, want cleared", g2.State)
				}
				return nil
			},
		},
		{
			name:	"block open gate",
			setup: func(svc *loanrace.Service,) string {
				svc.RegisterLoan("PG4", "U4")
				l, _ := svc.FreezeForOverdue("PG4", "U4", "scanner")
				svc.OpenPayGate("PG4", l.ID, "ops", "5000")
				return l.ID
			},
			action: func(svc *loanrace.Service, _ string) error {
				g, _ := svc.GateForLoan("PG4")
				_, err := svc.BlockPayGate(g.ID, "risk")
				if err != nil {
					return err
				}
				_, err = svc.PayThroughGate("PG4", "1000", "p")
				return err
			},
			wantErr:	loanrace.ErrGateBlocked,
		},
		{
			name:	"gate on written off loan",
			setup: func(svc *loanrace.Service,) string {
				svc.RegisterLoan("PG5", "U5")
				l, _ := svc.FreezeForOverdue("PG5", "U5", "scanner")
				wo, _ := svc.ApplyWriteoff("PG5", "1000", "50", "bad", "a")
				svc.ApproveWriteoff(wo.ID, "b")
				svc.ExecuteWriteoff(wo.ID, "c")
				return l.ID
			},
			action: func(svc *loanrace.Service, leaseID string) error {
				_, err := svc.OpenPayGate("PG5", leaseID, "ops", "1000")
				return err
			},
			wantErr:	loanrace.ErrWrittenOff,
		},
		{
			name:	"close gate before cleared fails",
			setup: func(svc *loanrace.Service,) string {
				svc.RegisterLoan("PG6", "U6")
				l, _ := svc.FreezeForOverdue("PG6", "U6", "scanner")
				svc.OpenPayGate("PG6", l.ID, "ops", "5000")
				return l.ID
			},
			action: func(svc *loanrace.Service, _ string) error {
				g, _ := svc.GateForLoan("PG6")
				_, err := svc.ClosePayGate(g.ID, "ops")
				return err
			},
			wantErr:	loanrace.ErrState,
		},
		{
			name:	"empty actor rejected",
			setup: func(svc *loanrace.Service,) string {
				svc.RegisterLoan("PG7", "U7")
				l, _ := svc.FreezeForOverdue("PG7", "U7", "scanner")
				return l.ID
			},
			action: func(svc *loanrace.Service, leaseID string) error {
				_, err := svc.OpenPayGate("PG7", leaseID, "", "1000")
				return err
			},
			wantErr:	loanrace.ErrActor,
		},
		{
			name:	"invalid amount rejected",
			setup: func(svc *loanrace.Service,) string {
				svc.RegisterLoan("PG8", "U8")
				l, _ := svc.FreezeForOverdue("PG8", "U8", "scanner")
				return l.ID
			},
			action: func(svc *loanrace.Service, leaseID string) error {
				_, err := svc.OpenPayGate("PG8", leaseID, "ops", "-100")
				return err
			},
			wantErr:	loanrace.ErrAmount,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newTestClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
			svc := newTestService(clk,
				60*time.Second)
			leaseID := tt.setup(svc)
			err := tt.action(svc, leaseID)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err: got %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：Writeoff 双控
// ──────────────────────────────────────────────────────────────

func TestWriteoffDualControl(t *testing.T) {
	tests := []struct {
		name	string
		setup	func(svc *loanrace.Service,)
		action	func(svc *loanrace.Service,) error
		wantErr	error
	}{
		{
			name:	"same actor dual-control violation",
			setup: func(svc *loanrace.Service,) {
				svc.RegisterLoan("WO1", "U1")
				svc.FreezeForOverdue("WO1", "U1", "s")
			},
			action: func(svc *loanrace.Service,) error {
				wo, _ := svc.ApplyWriteoff("WO1", "1000", "50", "bad", "same")
				_, err := svc.ApproveWriteoff(wo.ID, "same")
				return err
			},
			wantErr:	loanrace.ErrDualControl,
		},
		{
			name:	"different actors succeed",
			setup: func(svc *loanrace.Service,) {
				svc.RegisterLoan("WO2", "U2")
				svc.FreezeForOverdue("WO2", "U2", "s")
			},
			action: func(svc *loanrace.Service,) error {
				wo, _ := svc.ApplyWriteoff("WO2", "1000", "50", "bad", "alice")
				_, err := svc.ApproveWriteoff(wo.ID, "bob")
				return err
			},
		},
		{
			name:	"execute unapproved fails",
			setup: func(svc *loanrace.Service,) {
				svc.RegisterLoan("WO3", "U3")
				svc.FreezeForOverdue("WO3", "U3", "s")
			},
			action: func(svc *loanrace.Service,) error {
				wo, _ := svc.ApplyWriteoff("WO3", "1000", "50", "bad", "alice")
				_, err := svc.ExecuteWriteoff(wo.ID, "charlie")
				return err
			},
			wantErr:	loanrace.ErrWriteoffState,
		},
		{
			name:	"reject then reapply",
			setup: func(svc *loanrace.Service,) {
				svc.RegisterLoan("WO4", "U4")
				svc.FreezeForOverdue("WO4", "U4", "s")
			},
			action: func(svc *loanrace.Service,) error {
				wo, _ := svc.ApplyWriteoff("WO4", "1000", "50", "bad", "alice")
				svc.RejectWriteoff(wo.ID, "bob")
				wo2, err := svc.ApplyWriteoff("WO4", "1000", "50", "retry", "alice")
				if err != nil {
					return err
				}
				if wo2.Reason != "retry" {
					t.Errorf("reason: %s", wo2.Reason)
				}
				return nil
			},
		},
		{
			name:	"duplicate writeoff blocked",
			setup: func(svc *loanrace.Service,) {
				svc.RegisterLoan("WO5", "U5")
				svc.FreezeForOverdue("WO5", "U5", "s")
			},
			action: func(svc *loanrace.Service,) error {
				svc.ApplyWriteoff("WO5", "1000", "50", "bad", "alice")
				_, err := svc.ApplyWriteoff("WO5", "1000", "50", "dup", "bob")
				return err
			},
			wantErr:	loanrace.ErrWriteoffExists,
		},
		{
			name:	"writeoff on closed loan fails",
			setup: func(svc *loanrace.Service,) {
				svc.RegisterLoan("WO6", "U6")
				svc.store.SetRaceState("WO6", loanrace.LRClosed)
			},
			action: func(svc *loanrace.Service,) error {
				_, err := svc.ApplyWriteoff("WO6", "1000", "50", "bad", "alice")
				return err
			},
			wantErr:	loanrace.ErrState,
		},
		{
			name:	"suspense entry created",
			setup: func(svc *loanrace.Service,) {
				svc.RegisterLoan("WO7", "U7")
				svc.FreezeForOverdue("WO7", "U7", "s")
			},
			action: func(svc *loanrace.Service,) error {
				wo, _ := svc.ApplyWriteoff("WO7", "5000", "500", "bad", "alice")
				svc.ApproveWriteoff(wo.ID, "bob")
				svc.ExecuteWriteoff(wo.ID, "charlie")
				entries := svc.SuspenseForLoan("WO7")
				if len(entries) != 1 {
					t.Errorf("suspense count: %d", len(entries))
				}
				if entries[0].Amount != "5500" {
					t.Errorf("suspense amount: %s", entries[0].Amount)
				}
				return nil
			},
		},
		{
			name:	"empty reason rejected",
			setup: func(svc *loanrace.Service,) {
				svc.RegisterLoan("WO8", "U8")
				svc.FreezeForOverdue("WO8", "U8", "s")
			},
			action: func(svc *loanrace.Service,) error {
				_, err := svc.ApplyWriteoff("WO8", "1000", "50", "", "alice")
				return err
			},
			wantErr:	loanrace.ErrState,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newTestClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
			svc := newTestService(clk, 60*time.Second)
			tt.setup(svc)
			err := tt.action(svc)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err: got %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：UnfreezeProof
// ──────────────────────────────────────────────────────────────

func TestUnfreezeProofTable(t *testing.T) {
	tests := []struct {
		name	string
		setup	func(svc *loanrace.Service,)
		action	func(svc *loanrace.Service,) error
		wantErr	error
	}{
		{
			name:	"submit on unregistered loan",
			setup:	func(svc *loanrace.Service,) {},
			action: func(svc *loanrace.Service,) error {
				_, err := svc.SubmitUnfreezeProof("NONE", "full_repay", "ref", "ops", "")
				return err
			},
			wantErr:	loanrace.ErrLoanNotFound,
		},
		{
			name:	"submit with empty kind",
			setup: func(svc *loanrace.Service,) {
				svc.RegisterLoan("P1", "U1")
			},
			action: func(svc *loanrace.Service,) error {
				_, err := svc.SubmitUnfreezeProof("P1", "", "ref", "ops", "")
				return err
			},
			wantErr:	loanrace.ErrProofInvalid,
		},
		{
			name:	"validate non-existent proof",
			setup:	func(svc *loanrace.Service,) {},
			action: func(svc *loanrace.Service,) error {
				_, err := svc.ValidateUnfreezeProof("NONE", "val")
				return err
			},
			wantErr:	loanrace.ErrProofNotFound,
		},
		{
			name:	"reject proof",
			setup: func(svc *loanrace.Service,) {
				svc.RegisterLoan("P2", "U2")
			},
			action: func(svc *loanrace.Service,) error {
				p, _ := svc.SubmitUnfreezeProof("P2", "waiver", "doc-123", "ops", "")
				rej, err := svc.RejectUnfreezeProof(p.ID, "val")
				if err != nil {
					return err
				}
				if rej.State != loanrace.ProofRejected {
					t.Errorf("state: %s", rej.State)
				}
				return nil
			},
		},
		{
			name:	"double submit after valid proof",
			setup: func(svc *loanrace.Service,) {
				svc.RegisterLoan("P3", "U3")
			},
			action: func(svc *loanrace.Service,) error {
				p, _ := svc.SubmitUnfreezeProof("P3", "full_repay", "ref1", "ops", "")
				svc.ValidateUnfreezeProof(p.ID, "val")
				_, err := svc.SubmitUnfreezeProof("P3", "full_repay", "ref2", "ops", "")
				return err
			},
			wantErr:	loanrace.ErrProofExists,
		},
		{
			name:	"proof on written off loan",
			setup: func(svc *loanrace.Service,) {
				svc.RegisterLoan("P4", "U4")
				svc.store.SetRaceState("P4", loanrace.LRWrittenOff)
			},
			action: func(svc *loanrace.Service,) error {
				_, err := svc.SubmitUnfreezeProof("P4", "waiver", "ref", "ops", "")
				return err
			},
			wantErr:	loanrace.ErrState,
		},
		{
			name:	"evidence hash computed",
			setup: func(svc *loanrace.Service,) {
				svc.RegisterLoan("P5", "U5")
			},
			action: func(svc *loanrace.Service,) error {
				p, err := svc.SubmitUnfreezeProof("P5", "full_repay", "TX-HASH-001", "ops", "")
				if err != nil {
					return err
				}
				if p.EvidenceHash == "" {
					t.Error("evidence hash empty")
				}
				if len(p.EvidenceHash) != 64 {
					t.Errorf("evidence hash length: %d", len(p.EvidenceHash))
				}
				return nil
			},
		},
		{
			name:	"clear frozen without proof on not-frozen loan",
			setup: func(svc *loanrace.Service,) {
				svc.RegisterLoan("P6", "U6")
			},
			action: func(svc *loanrace.Service,) error {
				return svc.ClearFrozenWithProof("P6", "ops")
			},
			wantErr:	loanrace.ErrNotFrozen,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newTestClock(time.Date(2024,
				1, 1, 0, 0, 0, 0, time.UTC))
			svc := newTestService(clk, 60*time.Second)
			tt.setup(svc)
			err := tt.action(svc)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err: got %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：Workers
// ──────────────────────────────────────────────────────────────

func TestScanOverdueWorker(t *testing.T) {
	tests := []struct {
		name		string
		loans		[][2]string
		overdue		map[string]bool
		wantCount	int
	}{
		{
			name:		"no loans",
			loans:		nil,
			overdue:	nil,
			wantCount:	0,
		},
		{
			name:		"all overdue",
			loans:		[][2]string{{"L1", "U1"}, {"L2", "U2"}, {"L3", "U3"}},
			overdue:	map[string]bool{"L1": true, "L2": true, "L3": true},
			wantCount:	3,
		},
		{
			name:		"some overdue",
			loans:		[][2]string{{"L1", "U1"}, {"L2", "U2"}, {"L3", "U3"}},
			overdue:	map[string]bool{"L1": true, "L3": true},
			wantCount:	2,
		},
		{
			name:		"none overdue",
			loans:		[][2]string{{"L1", "U1"}, {"L2", "U2"}},
			overdue:	map[string]bool{},
			wantCount:	0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newTestClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
			svc := newTestService(clk, 60*time.Second)
			for _, l := range tt.loans {
				svc.RegisterLoan(l[0], l[1])
			}
			checker := func(id string) bool { return tt.overdue[id] }
			n, err := svc.ScanOverdueWorker(context.Background(), checker, "scanner")
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if n != tt.wantCount {
				t.Fatalf("frozen count: got %d, want %d", n, tt.wantCount)
			}
		})
	}
}

func TestFrozenDriftPatrolReleasesExpired(t *testing.T) {
	clk := newTestClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 10*time.Second)
	svc.RegisterLoan("DP1", "U1")
	svc.RegisterLoan("DP2", "U2")
	svc.FreezeForOverdue("DP1", "U1", "scanner")
	svc.FreezeForOverdue("DP2", "U2", "scanner")
	clk.Advance(15 * time.Second)
	log := zerolog.Nop()
	released, err := svc.FrozenDriftPatrol(context.Background(), log)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if released != 2 {
		t.Fatalf("released: got %d, want 2", released)
	}
}

func TestMutexGateWorker(t *testing.T) {
	clk := newTestClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 60*time.Second)
	svc.RegisterLoan("MG1", "U1")
	svc.RegisterLoan("MG2", "U2")
	svc.FreezeForOverdue("MG1", "U1", "s")
	svc.FreezeForOverdue("MG2", "U2", "s")
	log := zerolog.Nop()
	checked, err := svc.MutexGateWorker(context.Background(), log)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if checked != 2 {
		t.Fatalf("checked: got %d, want 2", checked)
	}
}

func TestWriteoffAgingWorker(t *testing.T) {
	clk := newTestClock(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 60*time.Second)
	svc.RegisterLoan("WA1", "U1")
	svc.FreezeForOverdue("WA1", "U1", "s")
	clk.Advance(100 * 24 * time.Hour)
	log := zerolog.Nop()
	candidates, err := svc.WriteoffAgingWorker(context.Background(), 90, log)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(candidates) != 1 || candidates[0] != "WA1" {
		t.Fatalf("candidates: %v", candidates)
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：Service 视图
// ──────────────────────────────────────────────────────────────

func TestServiceViewTable(t *testing.T) {
	tests := []struct {
		name	string
		setup	func(svc *loanrace.Service,)
		loanID	string
		check	func(t *testing.T, v loanrace.LoanRaceView, err error)
	}{
		{
			name:	"unregistered loan",
			setup:	func(svc *loanrace.Service,) {},
			loanID:	"NONE",
			check: func(t *testing.T, v loanrace.LoanRaceView, err error) {
				if !errors.Is(err, loanrace.ErrLoanNotFound) {
					t.Fatalf("err: %v", err)
				}
			},
		},
		{
			name:	"active loan",
			setup: func(svc *loanrace.Service,) {
				svc.RegisterLoan("V1", "U1")
			},
			loanID:	"V1",
			check: func(t *testing.T, v loanrace.LoanRaceView, err error) {
				if err != nil {
					t.Fatalf("err: %v", err)
				}
				if v.RaceState != loanrace.LRActive {
					t.Errorf("state: %s", v.RaceState)
				}
				if v.Frozen {
					t.Error("should not be frozen")
				}
			},
		},
		{
			name:	"frozen loan with lease",
			setup: func(svc *loanrace.Service,) {
				svc.RegisterLoan("V2", "U2")
				svc.FreezeForOverdue("V2", "U2", "s")
			},
			loanID:	"V2",
			check: func(t *testing.T, v loanrace.LoanRaceView, err error) {
				if err != nil {
					t.Fatalf("err: %v", err)
				}
				if v.RaceState != loanrace.LRFrozen {
					t.Errorf("state: %s", v.RaceState)
				}
				if !v.Frozen {
					t.Error("should be frozen")
				}
				if !v.HasLease {
					t.Error("should have lease")
				}
			},
		},
		{
			name:	"written off loan",
			setup: func(svc *loanrace.Service,) {
				svc.RegisterLoan("V3", "U3")
				svc.FreezeForOverdue("V3", "U3", "s")
				wo, _ := svc.ApplyWriteoff("V3", "1000", "50", "bad", "a")
				svc.ApproveWriteoff(wo.ID, "b")
				svc.ExecuteWriteoff(wo.ID, "c")
			},
			loanID:	"V3",
			check: func(t *testing.T, v loanrace.LoanRaceView, err error) {
				if err != nil {
					t.Fatalf("err: %v", err)
				}
				if v.RaceState != loanrace.LRWrittenOff {
					t.Errorf("state: %s", v.RaceState)
				}
				if !v.WrittenOff {
					t.Error("should be written off")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newTestClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
			svc := newTestService(clk, 60*time.Second)
			tt.setup(svc)
			v, err := svc.GetView(tt.loanID)
			tt.check(t, v, err)
		})
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：E2E 完整场景
// ──────────────────────────────────────────────────────────────

func TestE2EScenarios(t *testing.T) {
	tests := []struct {
		name		string
		scenario	func(t *testing.T, svc *loanrace.Service, clk *testClock)
	}{
		{
			name:	"happy path: overdue→freeze→controlled repay→proof→unfreeze",
			scenario: func(t *testing.T, svc *loanrace.Service, clk *testClock) {
				svc.RegisterLoan("E2E-1", "USER-X")
				lease, err := svc.FreezeForOverdue("E2E-1", "USER-X", "scanner")
				if err != nil {
					t.Fatalf("freeze: %v", err)
				}
				gate, err := svc.StartControlledRepay("E2E-1", lease.ID, "ops", "10000")
				if err != nil {
					t.Fatalf("start repay: %v", err)
				}
				_, err = svc.PayThroughGate("E2E-1", "10000", "payer")
				if err != nil {
					t.Fatalf("pay: %v", err)
				}
				_, err = svc.CompleteRepay(gate.ID, "ops")
				if err != nil {
					t.Fatalf("complete: %v", err)
				}
				proof, err := svc.SubmitUnfreezeProof("E2E-1", "full_repay", "TX-001", "ops", "")
				if err != nil {
					t.Fatalf("submit proof: %v", err)
				}
				_, err = svc.ValidateUnfreezeProof(proof.ID, "validator")
				if err != nil {
					t.Fatalf("validate: %v", err)
				}
				err = svc.ClearFrozenWithProof("E2E-1", "ops")
				if err != nil {
					t.Fatalf("clear: %v", err)
				}
				s, _ := svc.GetRaceState("E2E-1")
				if s != loanrace.LRClosed {
					t.Fatalf("final state: %s", s)
				}
			},
		},
		{
			name:	"freeze→writeoff→pay rejected→suspense verified",
			scenario: func(t *testing.T, svc *loanrace.Service, clk *testClock) {
				svc.RegisterLoan("E2E-2", "USER-Y")
				_, err := svc.FreezeForOverdue("E2E-2", "USER-Y", "scanner")
				if err != nil {
					t.Fatalf("freeze: %v", err)
				}
				wo, err := svc.ApplyWriteoff("E2E-2", "8000", "400", "90d overdue", "risk_a")
				if err != nil {
					t.Fatalf("apply: %v", err)
				}
				_, err = svc.ApproveWriteoff(wo.ID, "risk_b")
				if err != nil {
					t.Fatalf("approve: %v", err)
				}
				_, err = svc.ExecuteWriteoff(wo.ID, "exec_c")
				if err != nil {
					t.Fatalf("execute: %v", err)
				}
				err = svc.CheckPayAllowed("E2E-2")
				if !errors.Is(err, loanrace.ErrWrittenOff) {
					t.Fatalf("pay allowed check: %v", err)
				}
				entries := svc.SuspenseForLoan("E2E-2")
				if len(entries) != 1 {
					t.Fatalf("suspense count: %d", len(entries))
				}
				if entries[0].Amount != "8400" {
					t.Fatalf("suspense amt: %s", entries[0].Amount)
				}
			},
		},
		{
			name:	"freeze→repay fail→back to frozen→retry→success",
			scenario: func(t *testing.T, svc *loanrace.Service, clk *testClock) {
				svc.RegisterLoan("E2E-3", "USER-Z")
				lease, err := svc.FreezeForOverdue("E2E-3", "USER-Z", "scanner")
				if err != nil {
					t.Fatalf("freeze: %v", err)
				}
				_, err = svc.StartControlledRepay("E2E-3", lease.ID, "ops", "5000")
				if err != nil {
					t.Fatalf("start repay: %v", err)
				}
				err = svc.FailRepay("E2E-3", "ops")
				if err != nil {
					t.Fatalf("fail repay: %v", err)
				}
				s, _ := svc.GetRaceState("E2E-3")
				if s != loanrace.LRFrozen {
					t.Fatalf("after fail: %s", s)
				}
				gate2, err := svc.StartControlledRepay("E2E-3", lease.ID, "ops", "5000")
				if err != nil {
					t.Fatalf("retry start: %v", err)
				}
				_, err = svc.PayThroughGate("E2E-3", "5000", "payer")
				if err != nil {
					t.Fatalf("retry pay: %v", err)
				}
				_, err = svc.CompleteRepay(gate2.ID, "ops")
				if err != nil {
					t.Fatalf("retry complete: %v", err)
				}
				proof, _ := svc.SubmitUnfreezeProof("E2E-3", "full_repay", "TX-RETRY", "ops", "")
				svc.ValidateUnfreezeProof(proof.ID, "v")
				svc.ClearFrozenWithProof("E2E-3", "ops")
				s, _ = svc.GetRaceState("E2E-3")
				if s != loanrace.LRClosed {
					t.Fatalf("final: %s", s)
				}
			},
		},
		{
			name:	"lease expiry then re-acquire",
			scenario: func(t *testing.T, svc *loanrace.Service, clk *testClock) {
				svc.RegisterLoan("E2E-4", "USER-W")
				_, err := svc.FreezeForOverdue("E2E-4", "USER-W", "scanner")
				if err != nil {
					t.Fatalf("freeze: %v", err)
				}
				clk.Advance(120 * time.Second)
				svc.FrozenDriftPatrol(context.Background(), zerolog.Nop())
				lease2, err := svc.AcquireLease("E2E-4", "USER-W", "new_scanner")
				if err != nil {
					t.Fatalf("re-acquire: %v", err)
				}
				if lease2.Holder != "new_scanner" {
					t.Fatalf("holder: %s", lease2.Holder)
				}
			},
		},
		{
			name:	"concurrent scan worker idempotent",
			scenario: func(t *testing.T, svc *loanrace.Service, clk *testClock) {
				for i := 0; i < 10; i++ {
					id := "BATCH-" + string(rune('A'+i))
					svc.RegisterLoan(id, "U-BATCH")
				}
				var total int64
				var wg sync.WaitGroup
				for w := 0; w < 4; w++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						n, _ := svc.ScanOverdueWorker(context.Background(),
							func(_ string) bool { return true }, "scanner")
						atomic.AddInt64(&total, int64(n))
					}()
				}
				wg.Wait()
				if total != 10 {
					t.Fatalf("total frozen: %d, want 10", total)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := newTestClock(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
			svc := newTestService(clk, 30*time.Second)
			tt.scenario(t, svc, clk)
		})
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：Store 基本操作
// ──────────────────────────────────────────────────────────────

func TestStoreOperations(t *testing.T) {
	tests := []struct {
		name	string
		check	func(t *testing.T, st *loanrace.Store,)
	}{
		{
			name:	"register and get state",
			check: func(t *testing.T, st *loanrace.Store,) {
				st.RegisterLoan("S1", "U1")
				s, ok := st.GetRaceState("S1")
				if !ok || s != loanrace.LRActive {
					t.Fatalf("state: %s, ok: %v", s, ok)
				}
			},
		},
		{
			name:	"get unregistered",
			check: func(t *testing.T, st *loanrace.Store,) {
				_, ok := st.GetRaceState("NONE")
				if ok {
					t.Fatal("should not exist")
				}
			},
		},
		{
			name:	"frozen bit",
			check: func(t *testing.T, st *loanrace.Store,) {
				st.RegisterLoan("S2", "U2")
				if st.IsFrozen("S2") {
					t.Fatal("should not be frozen")
				}
				st.SetFrozen("S2", true)
				if !st.IsFrozen("S2") {
					t.Fatal("should be frozen")
				}
				st.SetFrozen("S2", false)
				if st.IsFrozen("S2") {
					t.Fatal("should not be frozen again")
				}
			},
		},
		{
			name:	"loans in state",
			check: func(t *testing.T, st *loanrace.Store,) {
				st.RegisterLoan("S3A", "U3")
				st.RegisterLoan("S3B", "U3")
				st.SetRaceState("S3A", loanrace.LRFrozen)
				frozen := st.LoansInState(loanrace.LRFrozen)
				if len(frozen) != 1 || frozen[0] != "S3A" {
					t.Fatalf("frozen: %v", frozen)
				}
			},
		},
		{
			name:	"build view",
			check: func(t *testing.T, st *loanrace.Store,) {
				st.RegisterLoan("S4", "U4")
				v := st.BuildView("S4")
				if v.LoanID != "S4" || v.UserID != "U4" {
					t.Fatalf("view: %+v", v)
				}
				if v.RaceState != loanrace.LRActive {
					t.Fatalf("state: %s", v.RaceState)
				}
			},
		},
		{
			name:	"expired leases",
			check: func(t *testing.T, st *loanrace.Store,) {
				now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
				l := &loanrace.OverdueLease{
					ID:	"EL1", LoanID: "S5", State: loanrace.LeaseApplied,
					ExpiresAt:	now.Add(-10 * time.Second),
				}
				st.PutLease(l)
				expired := st.ExpiredLeases(now)
				if len(expired) != 1 {
					t.Fatalf("expired: %d", len(expired))
				}
			},
		},
		{
			name:	"suspense for loan",
			check: func(t *testing.T, st *loanrace.Store,) {
				st.PutSuspense(&loanrace.SuspenseEntry{ID: "SE1", LoanID: "S6", Amount: "100"})
				st.PutSuspense(&loanrace.SuspenseEntry{ID: "SE2", LoanID: "S6", Amount: "200"})
				st.PutSuspense(&loanrace.SuspenseEntry{ID: "SE3", LoanID: "S7", Amount: "300"})
				entries := st.SuspenseForLoan("S6")
				if len(entries) != 2 {
					t.Fatalf("entries: %d", len(entries))
				}
			},
		},
		{
			name:	"double register idempotent",
			check: func(t *testing.T, st *loanrace.Store,) {
				st.RegisterLoan("S8", "U8")
				st.SetRaceState("S8", loanrace.LRFrozen)
				st.RegisterLoan("S8", "U8")
				s, _ := st.GetRaceState("S8")
				if s != loanrace.LRFrozen {
					t.Fatalf("state after re-register: %s", s)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := loanrace.NewStore()
			tt.check(t, st)
		})
	}
}

// ──────────────────────────────────────────────────────────────
// Nil service safety
// ──────────────────────────────────────────────────────────────

func TestNilServiceSafety(t *testing.T) {
	var svc *loanrace.Service
	if _, err := svc.AcquireLease("L", "U", "h"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("AcquireLease: %v", err)
	}
	if _, err := svc.ApplyLease("L"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("ApplyLease: %v", err)
	}
	if _, err := svc.ReleaseLease("L"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("ReleaseLease: %v", err)
	}
	if _, err := svc.OpenPayGate("L", "Le", "a", "1"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("OpenPayGate: %v", err)
	}
	if _, err := svc.PayThroughGate("L", "1", "a"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("PayThroughGate: %v", err)
	}
	if _, err := svc.ClosePayGate("G", "a"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("ClosePayGate: %v", err)
	}
	if _, err := svc.BlockPayGate("G", "a"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("BlockPayGate: %v", err)
	}
	if _, err := svc.ApplyWriteoff("L", "1", "1", "r", "a"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("ApplyWriteoff: %v", err)
	}
	if _, err := svc.ApproveWriteoff("W", "a"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("ApproveWriteoff: %v", err)
	}
	if _, err := svc.ExecuteWriteoff("W", "a"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("ExecuteWriteoff: %v", err)
	}
	if _, err := svc.RejectWriteoff("W", "a"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("RejectWriteoff: %v", err)
	}
	if _, err := svc.SubmitUnfreezeProof("L", "k", "r", "a", ""); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("SubmitUnfreezeProof: %v", err)
	}
	if _, err := svc.ValidateUnfreezeProof("P", "a"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("ValidateUnfreezeProof: %v", err)
	}
	if _, err := svc.RejectUnfreezeProof("P", "a"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("RejectUnfreezeProof: %v", err)
	}
	if err := svc.ClearFrozenWithProof("L", "a"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("ClearFrozenWithProof: %v", err)
	}
	if _, err := svc.FreezeForOverdue("L", "U", "a"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("FreezeForOverdue: %v", err)
	}
	if _, err := svc.StartControlledRepay("L", "Le", "a", "1"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("StartControlledRepay: %v", err)
	}
	if _, err := svc.CompleteRepay("G", "a"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("CompleteRepay: %v", err)
	}
	if err := svc.FailRepay("L", "a"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("FailRepay: %v", err)
	}
	if _, err := svc.GetRaceState("L"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("GetRaceState: %v", err)
	}
	if _, err := svc.GetView("L"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("GetView: %v", err)
	}
	if _, err := svc.GetLease("L"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("GetLease: %v", err)
	}
	if _, err := svc.GetGate("G"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("GetGate: %v", err)
	}
	if _, err := svc.GateForLoan("L"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("GateForLoan: %v", err)
	}
	if err := svc.CheckPayAllowed("L"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("CheckPayAllowed: %v", err)
	}
	if _, err := svc.GetWriteoff("W"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("GetWriteoff: %v", err)
	}
	if _, err := svc.WriteoffForLoan("L"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("WriteoffForLoan: %v", err)
	}
	if _, err := svc.GetProof("P"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("GetProof: %v", err)
	}
	if _, err := svc.ProofForLoan("L"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("ProofForLoan: %v", err)
	}
	if err := svc.ValidateLease("L"); !errors.Is(err, loanrace.ErrUnavailable) {
		t.Fatalf("ValidateLease: %v", err)
	}
	if n, err := svc.ScanOverdueWorker(context.Background(), nil, "a"); err == nil || n != 0 {
		t.Fatalf("ScanOverdueWorker: n=%d err=%v", n, err)
	}
	if n, err := svc.FrozenDriftPatrol(context.Background(), zerolog.Nop()); err == nil || n != 0 {
		t.Fatalf("FrozenDriftPatrol: n=%d err=%v", n, err)
	}
	if n, err := svc.MutexGateWorker(context.Background(), zerolog.Nop()); err == nil || n != 0 {
		t.Fatalf("MutexGateWorker: n=%d err=%v", n, err)
	}
	if cs, err := svc.WriteoffAgingWorker(context.Background(), 90, zerolog.Nop()); err == nil || cs != nil {
		t.Fatalf("WriteoffAgingWorker: cs=%v err=%v", cs, err)
	}
	svc.RegisterLoan("L", "U")	// should not panic
	if loans := svc.AllRegisteredLoans(); loans != nil {
		t.Fatalf("AllRegisteredLoans: %v", loans)
	}
	if loans := svc.LoansInState(loanrace.LRActive); loans != nil {
		t.Fatalf("LoansInState: %v", loans)
	}
	if entries := svc.SuspenseForLoan("L"); entries != nil {
		t.Fatalf("SuspenseForLoan: %v", entries)
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：FSM 终止态检查
// ──────────────────────────────────────────────────────────────

func TestFSMTerminals(t *testing.T) {
	tests := []struct {
		name	string
		state	loanrace.LoanRaceState
		want	bool
	}{
		{"closed is terminal", loanrace.LRClosed, true},
		{"written_off is terminal", loanrace.LRWrittenOff, true},
		{"active not terminal", loanrace.LRActive, false},
		{"overdue not terminal", loanrace.LROverdue, false},
		{"frozen not terminal", loanrace.LRFrozen, false},
		{"repaying not terminal", loanrace.LRRepaying, false},
	}
	m := loanrace.LoanRaceFSM()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.Terminal(tt.state); got != tt.want {
				t.Fatalf("Terminal(%s): got %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

func TestLeaseTerminals(t *testing.T) {
	m := loanrace.FreezeLeaseFSM()
	if !m.Terminal(loanrace.LeaseReleased) {
		t.Fatal("released should be terminal")
	}
	if !m.Terminal(loanrace.LeaseExpired) {
		t.Fatal("expired should be terminal")
	}
	if m.Terminal(loanrace.LeaseAcquired) {
		t.Fatal("acquired should not be terminal")
	}
}

func TestGateTerminals(t *testing.T) {
	m := loanrace.PayGateFSM()
	if !m.Terminal(loanrace.GateClosed) {
		t.Fatal("closed should be terminal")
	}
	if !m.Terminal(loanrace.GateBlocked) {
		t.Fatal("blocked should be terminal")
	}
	if m.Terminal(loanrace.GateOpen) {
		t.Fatal("open should not be terminal")
	}
}

func TestWriteoffTerminals(t *testing.T) {
	m := loanrace.WriteoffFSM()
	if !m.Terminal(loanrace.WOApplied) {
		t.Fatal("applied should be terminal")
	}
	if !m.Terminal(loanrace.WORejected) {
		t.Fatal("rejected should be terminal")
	}
	if m.Terminal(loanrace.WOPending) {
		t.Fatal("pending should not be terminal")
	}
}

func TestProofTerminals(t *testing.T) {
	m := loanrace.UnfreezeProofFSM()
	if !m.Terminal(loanrace.ProofValid) {
		t.Fatal("valid should be terminal")
	}
	if !m.Terminal(loanrace.ProofRejected) {
		t.Fatal("rejected should be terminal")
	}
	if m.Terminal(loanrace.ProofPending) {
		t.Fatal("pending should not be terminal")
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：Generation 递增
// ──────────────────────────────────────────────────────────────

func TestLeaseGenerationIncrements(t *testing.T) {
	clk := newTestClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 5*time.Second)
	svc.RegisterLoan("GEN1", "U1")
	l1, _ := svc.FreezeForOverdue("GEN1", "U1", "s1")
	if l1.Generation != 1 {
		t.Fatalf("gen1: %d", l1.Generation)
	}
	clk.Advance(10 * time.Second)
	svc.FrozenDriftPatrol(context.Background(), zerolog.Nop())
	l2, _ := svc.AcquireLease("GEN1", "U1", "s2")
	if l2.Generation != 2 {
		t.Fatalf("gen2: %d", l2.Generation)
	}
	clk.Advance(10 * time.Second)
	svc.FrozenDriftPatrol(context.Background(), zerolog.Nop())
	l3, _ := svc.AcquireLease("GEN1", "U1", "s3")
	if l3.Generation != 3 {
		t.Fatalf("gen3: %d", l3.Generation)
	}
}

// ──────────────────────────────────────────────────────────────
// 表驱动：PayGate 部分支付
// ──────────────────────────────────────────────────────────────

func TestPayGatePartialPayments(t *testing.T) {
	clk := newTestClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	svc := newTestService(clk, 60*time.Second)
	svc.RegisterLoan("PP1", "U1")
	lease, _ := svc.FreezeForOverdue("PP1", "U1", "scanner")
	svc.OpenPayGate("PP1", lease.ID, "ops", "10000")
	payments := []struct {
		amount		string
		wantState	loanrace.GateState
		wantPaid	string
	}{
		{"1000", loanrace.GateOpen, "1000"},
		{"2000", loanrace.GateOpen, "3000"},
		{"3000", loanrace.GateOpen, "6000"},
		{"4000", loanrace.GateCleared, "10000"},
	}
	for i, p := range payments {
		g, err := svc.PayThroughGate("PP1", p.amount, "payer")
		if err != nil {
			t.Fatalf("pay %d: %v", i, err)
		}
		if g.State != p.wantState {
			t.Fatalf("pay %d state: got %s, want %s", i, g.State, p.wantState)
		}
		if g.PaidAmt != p.wantPaid {
			t.Fatalf("pay %d paid: got %s, want %s", i, g.PaidAmt, p.wantPaid)
		}
	}
}

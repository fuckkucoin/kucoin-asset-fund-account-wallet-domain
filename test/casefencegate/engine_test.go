package casefencegate_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/casefencegate"
)

// ──────────────────────────────────────────
// Mock CaseQuerier
// ──────────────────────────────────────────

type mockCaseQuerier struct {
	mu	sync.RWMutex
	cases	map[string]*mockCase
	epoch	int64
}

type mockCase struct {
	snap	casefencegate.CaseSnapshot
	fence	mockFence
}

type mockFence struct {
	epoch	int64
	token	string
}

func newMockCaseQuerier() *mockCaseQuerier {
	return &mockCaseQuerier{cases: make(map[string]*mockCase)}
}

func (m *mockCaseQuerier) AddCase(cs casefencegate.CaseSnapshot, token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mc := &mockCase{
		snap:	cs,
		fence:	mockFence{epoch: cs.FenceEpoch, token: token},
	}
	m.cases[cs.ID] = mc
}

func (m *mockCaseQuerier) GetCase(id string) (casefencegate.CaseSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mc, ok := m.cases[id]
	if !ok {
		return casefencegate.CaseSnapshot{}, fmt.Errorf("case not found: %s", id)
	}
	snap := mc.snap
	snap.FenceEpoch = mc.fence.epoch
	snap.FenceToken = mc.fence.token
	return snap, nil
}

func (m *mockCaseQuerier) GetCaseByMovement(movementID string) (casefencegate.CaseSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, mc := range m.cases {
		if mc.snap.MovementID == movementID {
			snap := mc.snap
			snap.FenceEpoch = mc.fence.epoch
			snap.FenceToken = mc.fence.token
			return snap, nil
		}
	}
	return casefencegate.CaseSnapshot{}, fmt.Errorf("case not found for movement: %s", movementID)
}

func (m *mockCaseQuerier) DecideFenced(id, actor, decision string, epoch int64, token string) (casefencegate.CaseSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mc, ok := m.cases[id]
	if !ok {
		return casefencegate.CaseSnapshot{}, fmt.Errorf("case not found: %s", id)
	}
	if mc.fence.epoch != epoch || mc.fence.token != token {
		return casefencegate.CaseSnapshot{}, fmt.Errorf("stale fence: expected epoch=%d token=%s got epoch=%d token=%s",
			mc.fence.epoch, mc.fence.token, epoch, token)
	}
	if !mc.snap.IsOpen() {
		return casefencegate.CaseSnapshot{}, fmt.Errorf("case not open: %s", mc.snap.State)
	}
	mc.snap.DecidedBy = actor
	if decision == "approve" || decision == "allow" {
		mc.snap.State = "approved"
	} else {
		mc.snap.State = "denied"
	}
	newEpoch := atomic.AddInt64(&m.epoch,
		1) + mc.fence.epoch
	mc.fence.epoch = newEpoch
	mc.fence.token = fmt.Sprintf("token-%d", newEpoch)
	snap := mc.snap
	snap.FenceEpoch = mc.fence.epoch
	snap.FenceToken = mc.fence.token
	return snap, nil
}

func (m *mockCaseQuerier) ExpireOlderThan(_ time.Duration) []casefencegate.CaseSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []casefencegate.CaseSnapshot
	for _, mc := range m.cases {
		if mc.snap.IsOpen() {
			mc.snap.State = "expired"
			newEpoch := atomic.AddInt64(&m.epoch, 1) + mc.fence.epoch
			mc.fence.epoch = newEpoch
			mc.fence.token = fmt.Sprintf("token-exp-%d", newEpoch)
			snap := mc.snap
			snap.FenceEpoch = mc.fence.epoch
			snap.FenceToken = mc.fence.token
			out = append(out, snap)
		}
	}
	return out
}

// ──────────────────────────────────────────
// 辅助
// ──────────────────────────────────────────

func testEngine(cq casefencegate.CaseQuerier,) *casefencegate.Engine {
	eng := casefencegate.New(casefencegate.Config{
		Store:			casefencegate.NewMemTicketStore(),
		CaseQuerier:		cq,
		DefaultTTL:		5 * time.Minute,
		MaxTTL:			30 * time.Minute,
		BlockBareDecide:	true,
	})
	eng.SetClock(func() time.Time {
		return time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC)
	})
	return eng
}

func testEngineWithClock(cq casefencegate.CaseQuerier, clock func() time.Time) *casefencegate.Engine {
	eng := casefencegate.New(casefencegate.Config{
		Store:			casefencegate.NewMemTicketStore(),
		CaseQuerier:		cq,
		DefaultTTL:		5 * time.Minute,
		MaxTTL:			30 * time.Minute,
		BlockBareDecide:	true,
	})
	eng.SetClock(clock)
	return eng
}

func goldenPath(name string) string {
	return filepath.Join("testdata", "casefencegate", name)
}

func writeGolden(t *testing.T, name string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden %s: %v", name, err)
	}
	path := filepath.Join("..", "..", "testdata", "casefencegate", name)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir golden %s: %v", dir, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write golden %s: %v", path, err)
	}
}

func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "casefencegate", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("golden not found: %s (run with -update to create)", path)
	}
	return data
}

// ──────────────────────────────────────────
// Fixture 1: 双 reviewer 并发 DecideFenced(approve)
// ──────────────────────────────────────────

func TestDualReviewerConcurrentApprove(t *testing.T) {
	cq := newMockCaseQuerier()
	cq.AddCase(casefencegate.CaseSnapshot{
		ID:	"case-001", MovementID: "mv-001", UserID: "user-001",
		State:	"open", PolicyVer: 3, FenceEpoch: 1,
	}, "tok-1")
	eng := testEngine(cq)
	ctx := context.Background()
	result1, err1 := eng.DecideFencedOnly(ctx, casefencegate.DecideFencedReq{
		CaseID:	"case-001", MovementID: "mv-001", Actor: "reviewer-A",
		Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-1",
	})
	if err1 != nil {
		t.Fatalf("reviewer-A should succeed: %v", err1)
	}
	if result1.Ticket == nil {
		t.Fatal("reviewer-A should get a ticket")
	}
	if result1.Ticket.State != casefencegate.TicketApproved {
		t.Fatalf("ticket state want approved got %s", result1.Ticket.State)
	}
	result2, err2 := eng.DecideFencedOnly(ctx, casefencegate.DecideFencedReq{
		CaseID:	"case-001", MovementID: "mv-001", Actor: "reviewer-B",
		Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-1",
	})
	if !errors.Is(err2, casefencegate.ErrStaleFence) {
		t.Fatalf("reviewer-B should get ErrStaleFence, got %v", err2)
	}
	if result2 == nil || result2.Proof == nil {
		t.Fatal("reviewer-B should get a CaseDualDecideProof")
	}
	if !result2.Proof.Sealed {
		t.Fatal("proof must be sealed")
	}
	if result2.Proof.LoserActor != "reviewer-B" {
		t.Fatalf("proof loser want reviewer-B got %s", result2.Proof.LoserActor)
	}
	if result2.Proof.WinnerActor != "reviewer-A" {
		t.Fatalf("proof winner want reviewer-A got %s", result2.Proof.WinnerActor)
	}
	type goldenDualApprove struct {
		WinnerTicket	*casefencegate.CaseReleaseTicket	`json:"winner_ticket"`
		LoserProof	*casefencegate.CaseDualDecideProof	`json:"loser_proof"`
	}
	golden := goldenDualApprove{
		WinnerTicket:	result1.Ticket,
		LoserProof:	result2.Proof,
	}
	writeGolden(t, "dual_approve_golden.json", golden)
}

func TestDualReviewerConcurrentApprove_Parallel(t *testing.T) {
	cq := newMockCaseQuerier()
	cq.AddCase(casefencegate.CaseSnapshot{
		ID:	"case-par-001", MovementID: "mv-par-001", UserID: "user-par-001",
		State:	"open", PolicyVer: 5, FenceEpoch: 1,
	}, "tok-par-1")
	eng := testEngine(cq)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make([]*casefencegate.DecideFencedResult, 2)
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		results[0], errs[0] = eng.DecideFencedOnly(ctx, casefencegate.DecideFencedReq{
			CaseID:	"case-par-001", MovementID: "mv-par-001", Actor: "rev-X",
			Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-par-1",
		})
	}()
	go func() {
		defer wg.Done()
		results[1], errs[1] = eng.DecideFencedOnly(ctx, casefencegate.DecideFencedReq{
			CaseID:	"case-par-001", MovementID: "mv-par-001", Actor: "rev-Y",
			Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-par-1",
		})
	}()
	wg.Wait()
	var winIdx, loseIdx int
	if errs[0] == nil {
		winIdx, loseIdx = 0, 1
	} else if errs[1] == nil {
		winIdx, loseIdx = 1, 0
	} else {
		t.Fatalf("both goroutines failed: err0=%v err1=%v", errs[0], errs[1])
	}
	if results[winIdx].Ticket == nil {
		t.Fatal("winner should have ticket")
	}
	if !errors.Is(errs[loseIdx], casefencegate.ErrStaleFence) {
		t.Fatalf("loser should get ErrStaleFence, got %v", errs[loseIdx])
	}
	if results[loseIdx] == nil || results[loseIdx].Proof == nil {
		t.Fatal("loser should get a proof")
	}
	if !results[loseIdx].Proof.Sealed {
		t.Fatal("proof must be sealed")
	}
}

// ──────────────────────────────────────────
// Fixture 2: Expire 后旧 epoch/token Approve 失败
// ──────────────────────────────────────────

func TestApproveAfterExpireFails(t *testing.T) {
	cq := newMockCaseQuerier()
	cq.AddCase(casefencegate.CaseSnapshot{
		ID:	"case-exp-001", MovementID: "mv-exp-001", UserID: "user-exp-001",
		State:	"open", PolicyVer: 2, FenceEpoch: 1,
	}, "tok-exp-1")
	eng := testEngine(cq)
	ctx := context.Background()
	cq.ExpireOlderThan(0)
	result, err := eng.DecideFencedOnly(ctx, casefencegate.DecideFencedReq{
		CaseID:	"case-exp-001", MovementID: "mv-exp-001", Actor: "reviewer-C",
		Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-exp-1",
	})
	if err == nil {
		t.Fatal("approve after expire should fail")
	}
	type goldenExpireApprove struct {
		Error	string					`json:"error"`
		Proof	*casefencegate.CaseDualDecideProof	`json:"proof,omitempty"`
		Reason	string					`json:"reason"`
	}
	golden := goldenExpireApprove{
		Error:	err.Error(),
		Reason:	"case expired; old fence epoch/token rejected",
	}
	if result != nil && result.Proof != nil {
		golden.Proof = result.Proof
	}
	writeGolden(t, "approve_after_expire_golden.json", golden)
}

func TestExpireThenApproveMutex(t *testing.T) {
	cq := newMockCaseQuerier()
	cq.AddCase(casefencegate.CaseSnapshot{
		ID:	"case-mutex-001", MovementID: "mv-mutex-001", UserID: "user-mutex-001",
		State:	"open", PolicyVer: 4, FenceEpoch: 1,
	}, "tok-mutex-1")
	eng := testEngine(cq)
	ctx := context.Background()
	cq.ExpireOlderThan(0)
	err := eng.ExpireApproveInterleave(ctx, "case-mutex-001")
	if !errors.Is(err, casefencegate.ErrExpireMutex) {
		t.Fatalf("want ErrExpireMutex, got %v", err)
	}
}

// ──────────────────────────────────────────
// Fixture 3: policyVer 漂移 Submit 拒绝
// ──────────────────────────────────────────

func TestPolicyVerDriftSubmitReject(t *testing.T) {
	cq := newMockCaseQuerier()
	cq.AddCase(casefencegate.CaseSnapshot{
		ID:	"case-pol-001", MovementID: "mv-pol-001", UserID: "user-pol-001",
		State:	"open", PolicyVer: 5, FenceEpoch: 1,
	}, "tok-pol-1")
	eng := testEngine(cq)
	ctx := context.Background()
	result,
		err := eng.DecideFencedOnly(ctx, casefencegate.DecideFencedReq{
		CaseID:	"case-pol-001", MovementID: "mv-pol-001", Actor: "reviewer-D",
		Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-pol-1",
	})
	if err != nil {
		t.Fatalf("approve should succeed: %v", err)
	}
	ticket := result.Ticket
	submitResult, submitErr := eng.SubmitCheck(ctx, casefencegate.SubmitCheckReq{
		TicketID:	ticket.ID,
		MovementID:	"mv-pol-001",
		PolicyVer:	6,
	})
	if !errors.Is(submitErr, casefencegate.ErrPolicyDrift) {
		t.Fatalf("want ErrPolicyDrift, got %v", submitErr)
	}
	if submitResult.Allowed {
		t.Fatal("submit should be blocked on policy drift")
	}
	type goldenPolicyDrift struct {
		TicketID	string					`json:"ticket_id"`
		SubmitResult	*casefencegate.SubmitCheckResult	`json:"submit_result"`
		Error		string					`json:"error"`
	}
	golden := goldenPolicyDrift{
		TicketID:	ticket.ID,
		SubmitResult:	submitResult,
		Error:		submitErr.Error(),
	}
	writeGolden(t, "policy_drift_submit_golden.json", golden)
}

func TestPolicyVerMatchSubmitPass(t *testing.T) {
	cq := newMockCaseQuerier()
	cq.AddCase(casefencegate.CaseSnapshot{
		ID:	"case-polok-001", MovementID: "mv-polok-001", UserID: "user-polok-001",
		State:	"open", PolicyVer: 7, FenceEpoch: 1,
	}, "tok-polok-1")
	eng := testEngine(cq)
	ctx := context.Background()
	result, err := eng.DecideFencedOnly(ctx, casefencegate.DecideFencedReq{
		CaseID:	"case-polok-001", MovementID: "mv-polok-001", Actor: "reviewer-E",
		Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-polok-1",
	})
	if err != nil {
		t.Fatalf("approve should succeed: %v", err)
	}
	submitResult, submitErr := eng.SubmitCheck(ctx, casefencegate.SubmitCheckReq{
		TicketID:	result.Ticket.ID,
		MovementID:	"mv-polok-001",
		PolicyVer:	7,
	})
	if submitErr != nil {
		t.Fatalf("submit should succeed: %v", submitErr)
	}
	if !submitResult.Allowed {
		t.Fatalf("submit should be allowed, reason: %s", submitResult.Reason)
	}
}

// ──────────────────────────────────────────
// Fixture 4: 裸 Decide 在 fund 路径必须不可用或失败
// ──────────────────────────────────────────

func TestBareDecideBlockedOnFundPath(t *testing.T) {
	cq := newMockCaseQuerier()
	cq.AddCase(casefencegate.CaseSnapshot{
		ID:	"case-bare-001", MovementID: "mv-bare-001", UserID: "user-bare-001",
		State:	"open", PolicyVer: 1, FenceEpoch: 1,
	}, "tok-bare-1")
	eng := testEngine(cq)
	ctx := context.Background()
	alert, err := eng.BareDecideBlock(ctx, casefencegate.BareDecideReq{
		CaseID:	"case-bare-001", MovementID: "mv-bare-001",
		Actor:	"reviewer-F", Decision: "approve",
	})
	if !errors.Is(err, casefencegate.ErrBareForbidden) {
		t.Fatalf("bare decide should be forbidden, got %v", err)
	}
	if alert == nil {
		t.Fatal("should return alert")
	}
	if !alert.Blocked {
		t.Fatal("alert should be blocked=true")
	}
}

func TestBareDecideAlertModeNotBlocked(t *testing.T) {
	cq := newMockCaseQuerier()
	eng := casefencegate.New(casefencegate.Config{
		Store:			casefencegate.NewMemTicketStore(),
		CaseQuerier:		cq,
		BlockBareDecide:	false,
	})
	eng.SetClock(func() time.Time {
		return time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC)
	})
	ctx := context.Background()
	alert, err := eng.BareDecideBlock(ctx, casefencegate.BareDecideReq{
		CaseID:	"case-bare-002", MovementID: "mv-bare-002",
		Actor:	"reviewer-G", Decision: "approve",
	})
	if err != nil {
		t.Fatalf("alert mode should not error: %v", err)
	}
	if alert == nil {
		t.Fatal("should return alert")
	}
	if alert.Blocked {
		t.Fatal("alert mode should have blocked=false")
	}
}

// ──────────────────────────────────────────
// 大表驱动: DecideFencedOnly
// ──────────────────────────────────────────

func TestDecideFencedOnly_TableDriven(t *testing.T) {
	type testCase struct {
		name		string
		caseSnap	casefencegate.CaseSnapshot
		fenceToken	string
		req		casefencegate.DecideFencedReq
		wantErr		error
		checkFn		func(t *testing.T, result *casefencegate.DecideFencedResult, err error)
	}
	tests := []testCase{
		{
			name:	"正常 approve 成功",
			caseSnap: casefencegate.CaseSnapshot{
				ID:	"td-001", MovementID: "mv-td-001", UserID: "user-td-001",
				State:	"open", PolicyVer: 1, FenceEpoch: 1,
			},
			fenceToken:	"tok-td-001",
			req: casefencegate.DecideFencedReq{
				CaseID:	"td-001", MovementID: "mv-td-001", Actor: "reviewer-td-A",
				Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-td-001",
			},
			checkFn: func(t *testing.T, result *casefencegate.DecideFencedResult, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("want nil err, got %v", err)
				}
				if result.Ticket == nil {
					t.Fatal("want ticket")
				}
				if result.Ticket.State != casefencegate.TicketApproved {
					t.Fatalf("want approved, got %s", result.Ticket.State)
				}
				if result.Ticket.PolicyVer != 1 {
					t.Fatalf("want policyVer=1, got %d", result.Ticket.PolicyVer)
				}
			},
		},
		{
			name:	"正常 deny 成功",
			caseSnap: casefencegate.CaseSnapshot{
				ID:	"td-002", MovementID: "mv-td-002", UserID: "user-td-002",
				State:	"open", PolicyVer: 2, FenceEpoch: 1,
			},
			fenceToken:	"tok-td-002",
			req: casefencegate.DecideFencedReq{
				CaseID:	"td-002", MovementID: "mv-td-002", Actor: "reviewer-td-B",
				Decision:	"deny", FenceEpoch: 1, FenceToken: "tok-td-002",
			},
			checkFn: func(t *testing.T, result *casefencegate.DecideFencedResult, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("want nil err, got %v", err)
				}
				if result.Ticket == nil {
					t.Fatal("want ticket")
				}
				if result.Ticket.State != casefencegate.TicketDenied {
					t.Fatalf("want denied, got %s", result.Ticket.State)
				}
			},
		},
		{
			name:	"fence epoch 不匹配 — ErrStaleFence",
			caseSnap: casefencegate.CaseSnapshot{
				ID:	"td-003", MovementID: "mv-td-003", UserID: "user-td-003",
				State:	"open", PolicyVer: 1, FenceEpoch: 2,
			},
			fenceToken:	"tok-td-003",
			req: casefencegate.DecideFencedReq{
				CaseID:	"td-003", MovementID: "mv-td-003", Actor: "reviewer-td-C",
				Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-old",
			},
			wantErr:	casefencegate.ErrStaleFence,
			checkFn: func(t *testing.T, result *casefencegate.DecideFencedResult, err error) {
				t.Helper()
				if !errors.Is(err, casefencegate.ErrStaleFence) {
					t.Fatalf("want ErrStaleFence, got %v", err)
				}
				if result != nil && result.Proof != nil && !result.Proof.Sealed {
					t.Fatal("proof must be sealed")
				}
			},
		},
		{
			name:	"空 actor 拒绝 — ErrMakerCheck",
			caseSnap: casefencegate.CaseSnapshot{
				ID:	"td-004", MovementID: "mv-td-004", UserID: "user-td-004",
				State:	"open", PolicyVer: 1, FenceEpoch: 1,
			},
			fenceToken:	"tok-td-004",
			req: casefencegate.DecideFencedReq{
				CaseID:	"td-004", MovementID: "mv-td-004", Actor: "",
				Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-td-004",
			},
			wantErr:	casefencegate.ErrMakerCheck,
		},
		{
			name:	"system actor 拒绝 — ErrMakerCheck",
			caseSnap: casefencegate.CaseSnapshot{
				ID:	"td-005", MovementID: "mv-td-005", UserID: "user-td-005",
				State:	"open", PolicyVer: 1, FenceEpoch: 1,
			},
			fenceToken:	"tok-td-005",
			req: casefencegate.DecideFencedReq{
				CaseID:	"td-005", MovementID: "mv-td-005", Actor: "system",
				Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-td-005",
			},
			wantErr:	casefencegate.ErrMakerCheck,
		},
		{
			name:	"actor 不能审自己 — ErrActorSelf",
			caseSnap: casefencegate.CaseSnapshot{
				ID:	"td-006", MovementID: "mv-td-006", UserID: "user-td-006",
				State:	"open", PolicyVer: 1, FenceEpoch: 1,
			},
			fenceToken:	"tok-td-006",
			req: casefencegate.DecideFencedReq{
				CaseID:	"td-006", MovementID: "mv-td-006", Actor: "user-td-006",
				Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-td-006",
			},
			wantErr:	casefencegate.ErrActorSelf,
		},
		{
			name:	"无效 decision — ErrInvalidDecision",
			caseSnap: casefencegate.CaseSnapshot{
				ID:	"td-007", MovementID: "mv-td-007", UserID: "user-td-007",
				State:	"open", PolicyVer: 1, FenceEpoch: 1,
			},
			fenceToken:	"tok-td-007",
			req: casefencegate.DecideFencedReq{
				CaseID:	"td-007", MovementID: "mv-td-007", Actor: "reviewer-td-D",
				Decision:	"maybe", FenceEpoch: 1, FenceToken: "tok-td-007",
			},
			wantErr:	casefencegate.ErrInvalidDecision,
		},
		{
			name:	"已关闭案件 — ErrCaseNotOpen",
			caseSnap: casefencegate.CaseSnapshot{
				ID:	"td-008", MovementID: "mv-td-008", UserID: "user-td-008",
				State:	"approved", PolicyVer: 1, FenceEpoch: 2,
			},
			fenceToken:	"tok-td-008",
			req: casefencegate.DecideFencedReq{
				CaseID:	"td-008", MovementID: "mv-td-008", Actor: "reviewer-td-E",
				Decision:	"approve", FenceEpoch: 2, FenceToken: "tok-td-008",
			},
			wantErr:	casefencegate.ErrCaseNotOpen,
		},
		{
			name:	"assigned 状态仍可 approve",
			caseSnap: casefencegate.CaseSnapshot{
				ID:	"td-009", MovementID: "mv-td-009", UserID: "user-td-009",
				State:	"assigned", PolicyVer: 3, FenceEpoch: 1,
			},
			fenceToken:	"tok-td-009",
			req: casefencegate.DecideFencedReq{
				CaseID:	"td-009", MovementID: "mv-td-009", Actor: "reviewer-td-F",
				Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-td-009",
			},
			checkFn: func(t *testing.T, result *casefencegate.DecideFencedResult, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("want nil err, got %v", err)
				}
				if result.Ticket == nil {
					t.Fatal("want ticket")
				}
				if result.Ticket.State != casefencegate.TicketApproved {
					t.Fatalf("want approved got %s", result.Ticket.State)
				}
			},
		},
		{
			name:	"allow 等同 approve",
			caseSnap: casefencegate.CaseSnapshot{
				ID:	"td-010", MovementID: "mv-td-010", UserID: "user-td-010",
				State:	"open", PolicyVer: 1, FenceEpoch: 1,
			},
			fenceToken:	"tok-td-010",
			req: casefencegate.DecideFencedReq{
				CaseID:	"td-010", MovementID: "mv-td-010", Actor: "reviewer-td-G",
				Decision:	"allow", FenceEpoch: 1, FenceToken: "tok-td-010",
			},
			checkFn: func(t *testing.T, result *casefencegate.DecideFencedResult, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("want nil err, got %v", err)
				}
				if result.Ticket.State != casefencegate.TicketApproved {
					t.Fatalf("want approved got %s", result.Ticket.State)
				}
			},
		},
		{
			name:		"不存在的 case — ErrCaseNotFound",
			caseSnap:	casefencegate.CaseSnapshot{},
			req: casefencegate.DecideFencedReq{
				CaseID:	"td-nonexist", MovementID: "mv-xxx", Actor: "reviewer-td-H",
				Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-xxx",
			},
			wantErr:	casefencegate.ErrCaseNotFound,
		},
		{
			name:	"过期案件 fence token 不匹配",
			caseSnap: casefencegate.CaseSnapshot{
				ID:	"td-011", MovementID: "mv-td-011", UserID: "user-td-011",
				State:	"open", PolicyVer: 1, FenceEpoch: 5,
			},
			fenceToken:	"tok-td-011",
			req: casefencegate.DecideFencedReq{
				CaseID:	"td-011", MovementID: "mv-td-011", Actor: "reviewer-td-I",
				Decision:	"approve", FenceEpoch: 3, FenceToken: "tok-old-3",
			},
			wantErr:	casefencegate.ErrStaleFence,
		},
		{
			name:	"denied 案件再次 decide 拒绝",
			caseSnap: casefencegate.CaseSnapshot{
				ID:	"td-012", MovementID: "mv-td-012", UserID: "user-td-012",
				State:	"denied", PolicyVer: 1, FenceEpoch: 3,
			},
			fenceToken:	"tok-td-012",
			req: casefencegate.DecideFencedReq{
				CaseID:	"td-012", MovementID: "mv-td-012", Actor: "reviewer-td-J",
				Decision:	"approve", FenceEpoch: 3, FenceToken: "tok-td-012",
			},
			wantErr:	casefencegate.ErrCaseNotOpen,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cq := newMockCaseQuerier()
			if tc.caseSnap.ID != "" {
				cq.AddCase(tc.caseSnap, tc.fenceToken)
			}
			eng := testEngine(cq)
			ctx := context.Background()
			result, err := eng.DecideFencedOnly(ctx, tc.req)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want err %v, got %v", tc.wantErr, err)
				}
			}
			if tc.checkFn != nil {
				tc.checkFn(t, result, err)
			}
		})
	}
}

// ──────────────────────────────────────────
// 大表驱动: SubmitCheck
// ──────────────────────────────────────────

func TestSubmitCheck_TableDriven(t *testing.T) {
	type testCase struct {
		name	string
		setup	func(eng *casefencegate.Engine, cq *mockCaseQuerier) string
		req	func(ticketID string) casefencegate.SubmitCheckReq
		wantErr	error
		checkFn	func(t *testing.T, result *casefencegate.SubmitCheckResult, err error)
	}
	tests := []testCase{
		{
			name:	"正常 policyVer 匹配通过",
			setup: func(eng *casefencegate.Engine, cq *mockCaseQuerier) string {
				cq.AddCase(casefencegate.CaseSnapshot{
					ID:	"sc-001", MovementID: "mv-sc-001", UserID: "u-sc-001",
					State:	"open", PolicyVer: 5, FenceEpoch: 1,
				}, "tok-sc-1")
				r, _ := eng.DecideFencedOnly(context.Background(), casefencegate.DecideFencedReq{
					CaseID:	"sc-001", MovementID: "mv-sc-001", Actor: "rev-sc-A",
					Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-sc-1",
				})
				return r.Ticket.ID
			},
			req: func(tid string) casefencegate.SubmitCheckReq {
				return casefencegate.SubmitCheckReq{TicketID: tid, MovementID: "mv-sc-001", PolicyVer: 5}
			},
			checkFn: func(t *testing.T, result *casefencegate.SubmitCheckResult, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("want nil err, got %v", err)
				}
				if !result.Allowed {
					t.Fatal("want allowed")
				}
			},
		},
		{
			name:	"policyVer 漂移拒绝",
			setup: func(eng *casefencegate.Engine, cq *mockCaseQuerier) string {
				cq.AddCase(casefencegate.CaseSnapshot{
					ID:	"sc-002", MovementID: "mv-sc-002", UserID: "u-sc-002",
					State:	"open", PolicyVer: 5, FenceEpoch: 1,
				}, "tok-sc-2")
				r, _ := eng.DecideFencedOnly(context.Background(), casefencegate.DecideFencedReq{
					CaseID:	"sc-002", MovementID: "mv-sc-002", Actor: "rev-sc-B",
					Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-sc-2",
				})
				return r.Ticket.ID
			},
			req: func(tid string) casefencegate.SubmitCheckReq {
				return casefencegate.SubmitCheckReq{TicketID: tid, MovementID: "mv-sc-002", PolicyVer: 6}
			},
			wantErr:	casefencegate.ErrPolicyDrift,
			checkFn: func(t *testing.T, result *casefencegate.SubmitCheckResult, err error) {
				t.Helper()
				if !errors.Is(err, casefencegate.ErrPolicyDrift) {
					t.Fatalf("want ErrPolicyDrift, got %v", err)
				}
				if result.Allowed {
					t.Fatal("should not be allowed")
				}
			},
		},
		{
			name:	"ticket 不存在",
			setup: func(_ *casefencegate.Engine, _ *mockCaseQuerier) string {
				return "nonexistent-ticket"
			},
			req: func(tid string) casefencegate.SubmitCheckReq {
				return casefencegate.SubmitCheckReq{TicketID: tid, MovementID: "mv-x", PolicyVer: 1}
			},
			wantErr:	casefencegate.ErrTicketNotFound,
		},
		{
			name:	"movement 不匹配",
			setup: func(eng *casefencegate.Engine, cq *mockCaseQuerier) string {
				cq.AddCase(casefencegate.CaseSnapshot{
					ID:	"sc-003", MovementID: "mv-sc-003", UserID: "u-sc-003",
					State:	"open", PolicyVer: 1, FenceEpoch: 1,
				}, "tok-sc-3")
				r, _ := eng.DecideFencedOnly(context.Background(), casefencegate.DecideFencedReq{
					CaseID:	"sc-003", MovementID: "mv-sc-003", Actor: "rev-sc-C",
					Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-sc-3",
				})
				return r.Ticket.ID
			},
			req: func(tid string) casefencegate.SubmitCheckReq {
				return casefencegate.SubmitCheckReq{TicketID: tid, MovementID: "mv-wrong", PolicyVer: 1}
			},
			wantErr:	casefencegate.ErrMovementMismatch,
		},
		{
			name:	"denied ticket submit",
			setup: func(eng *casefencegate.Engine, cq *mockCaseQuerier) string {
				cq.AddCase(casefencegate.CaseSnapshot{
					ID:	"sc-004", MovementID: "mv-sc-004", UserID: "u-sc-004",
					State:	"open", PolicyVer: 1, FenceEpoch: 1,
				}, "tok-sc-4")
				r, _ := eng.DecideFencedOnly(context.Background(), casefencegate.DecideFencedReq{
					CaseID:	"sc-004", MovementID: "mv-sc-004", Actor: "rev-sc-D",
					Decision:	"deny", FenceEpoch: 1, FenceToken: "tok-sc-4",
				})
				return r.Ticket.ID
			},
			req: func(tid string) casefencegate.SubmitCheckReq {
				return casefencegate.SubmitCheckReq{TicketID: tid, MovementID: "mv-sc-004", PolicyVer: 1}
			},
			checkFn: func(t *testing.T, result *casefencegate.SubmitCheckResult, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("want nil err for denied check, got %v", err)
				}
				if result.Allowed {
					t.Fatal("denied ticket should not allow")
				}
			},
		},
		{
			name:	"expired ticket submit",
			setup: func(eng *casefencegate.Engine, cq *mockCaseQuerier) string {
				cq.AddCase(casefencegate.CaseSnapshot{
					ID:	"sc-005", MovementID: "mv-sc-005", UserID: "u-sc-005",
					State:	"open", PolicyVer: 1, FenceEpoch: 1,
				}, "tok-sc-5")
				r, _ := eng.DecideFencedOnly(context.Background(), casefencegate.DecideFencedReq{
					CaseID:	"sc-005", MovementID: "mv-sc-005", Actor: "rev-sc-E",
					Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-sc-5",
				})
				eng.SetClock(func() time.Time {
					return time.Date(2022,
						6, 15, 10, 30, 0, 0, time.UTC)
				})
				return r.Ticket.ID
			},
			req: func(tid string) casefencegate.SubmitCheckReq {
				return casefencegate.SubmitCheckReq{TicketID: tid, MovementID: "mv-sc-005", PolicyVer: 1}
			},
			wantErr:	casefencegate.ErrTicketExpired,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cq := newMockCaseQuerier()
			eng := testEngine(cq)
			ticketID := tc.setup(eng, cq)
			req := tc.req(ticketID)
			result, err := eng.SubmitCheck(context.Background(), req)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want err %v, got %v", tc.wantErr, err)
				}
			}
			if tc.checkFn != nil {
				tc.checkFn(t, result, err)
			}
		})
	}
}

// ──────────────────────────────────────────
// 大表驱动: BareDecideBlock
// ──────────────────────────────────────────

func TestBareDecideBlock_TableDriven(t *testing.T) {
	type testCase struct {
		name		string
		blockBare	bool
		req		casefencegate.BareDecideReq
		wantErr		error
		wantBlock	bool
	}
	tests := []testCase{
		{
			name:		"阻断模式: 裸 Decide 被拒",
			blockBare:	true,
			req: casefencegate.BareDecideReq{
				CaseID:	"bare-td-001", MovementID: "mv-bare-td-001",
				Actor:	"rev-bare-A", Decision: "approve",
			},
			wantErr:	casefencegate.ErrBareForbidden,
			wantBlock:	true,
		},
		{
			name:		"告警模式: 裸 Decide 仅记录",
			blockBare:	false,
			req: casefencegate.BareDecideReq{
				CaseID:	"bare-td-002", MovementID: "mv-bare-td-002",
				Actor:	"rev-bare-B", Decision: "approve",
			},
			wantErr:	nil,
			wantBlock:	false,
		},
		{
			name:		"阻断模式 deny 也拒",
			blockBare:	true,
			req: casefencegate.BareDecideReq{
				CaseID:	"bare-td-003", MovementID: "mv-bare-td-003",
				Actor:	"rev-bare-C", Decision: "deny",
			},
			wantErr:	casefencegate.ErrBareForbidden,
			wantBlock:	true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eng := casefencegate.New(casefencegate.Config{
				Store:			casefencegate.NewMemTicketStore(),
				CaseQuerier:		newMockCaseQuerier(),
				BlockBareDecide:	tc.blockBare,
			})
			eng.SetClock(func() time.Time {
				return time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC)
			})
			alert, err := eng.BareDecideBlock(context.Background(), tc.req)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want err %v, got %v", tc.wantErr, err)
				}
			} else if err != nil {
				t.Fatalf("want nil err, got %v", err)
			}
			if alert == nil {
				t.Fatal("want alert")
			}
			if alert.Blocked != tc.wantBlock {
				t.Fatalf("want blocked=%v got %v", tc.wantBlock, alert.Blocked)
			}
		})
	}
}

// ──────────────────────────────────────────
// 大表驱动: TicketState FSM
// ──────────────────────────────────────────

func TestTicketFSM_TableDriven(t *testing.T) {
	type testCase struct {
		name	string
		cur	casefencegate.TicketState
		event	string
		wantSt	casefencegate.TicketState
		wantErr	bool
	}
	tests := []testCase{
		{"open→approve", casefencegate.TicketOpen, casefencegate.EventApprove, casefencegate.TicketApproved, false},
		{"open→deny", casefencegate.TicketOpen, casefencegate.EventDeny, casefencegate.TicketDenied, false},
		{"open→expire", casefencegate.TicketOpen, casefencegate.EventExpire, casefencegate.TicketExpired, false},
		{"open→stale", casefencegate.TicketOpen, casefencegate.EventStale, casefencegate.TicketStale, false},
		{"approved→approve 非法", casefencegate.TicketApproved, casefencegate.EventApprove, casefencegate.TicketApproved, true},
		{"approved→deny 非法", casefencegate.TicketApproved, casefencegate.EventDeny, casefencegate.TicketApproved, true},
		{"denied→approve 非法", casefencegate.TicketDenied, casefencegate.EventApprove, casefencegate.TicketDenied, true},
		{"expired→approve 非法", casefencegate.TicketExpired, casefencegate.EventApprove, casefencegate.TicketExpired, true},
		{"stale→approve 非法", casefencegate.TicketStale, casefencegate.EventApprove, casefencegate.TicketStale, true},
		{"open→unknown 非法", casefencegate.TicketOpen, "unknown", casefencegate.TicketOpen, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			next, err := casefencegate.ApplyTicketEvent(tc.cur, tc.event)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want err for %s + %s", tc.cur, tc.event)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if next != tc.wantSt {
				t.Fatalf("want %s got %s", tc.wantSt, next)
			}
		})
	}
}

// ──────────────────────────────────────────
// 大表驱动: ExpireStaleTickets
// ──────────────────────────────────────────

func TestExpireStaleTickets_TableDriven(t *testing.T) {
	type testCase struct {
		name		string
		setup		func(eng *casefencegate.Engine, cq *mockCaseQuerier)
		advClock	time.Duration
		wantExpired	int
		wantBumped	int
	}
	tests := []testCase{
		{
			name:		"无活跃 ticket 无过期",
			setup:		func(_ *casefencegate.Engine, _ *mockCaseQuerier) {},
			wantExpired:	0,
		},
		{
			name:	"手动插入 open ticket，TTL 未到",
			setup: func(eng *casefencegate.Engine, _ *mockCaseQuerier) {
				now := eng.now()
				_ = eng.store.SaveTicket(&casefencegate.CaseReleaseTicket{
					ID:	"tkt-exp-1", CaseID: "c-exp-1", MovementID: "m-exp-1",
					State:	casefencegate.TicketOpen, ExpiresAt: now.Add(10 * time.Minute),
					IssuedAt:	now, CreatedAt: now,
				})
			},
			advClock:	1 * time.Minute,
			wantExpired:	0,
		},
		{
			name:	"手动插入 open ticket，TTL 过期",
			setup: func(eng *casefencegate.Engine, _ *mockCaseQuerier) {
				now := eng.now()
				_ = eng.store.SaveTicket(&casefencegate.CaseReleaseTicket{
					ID:	"tkt-exp-2", CaseID: "c-exp-2", MovementID: "m-exp-2",
					State:	casefencegate.TicketOpen, ExpiresAt: now.Add(5 * time.Minute),
					IssuedAt:	now, CreatedAt: now,
				})
			},
			advClock:	10 * time.Minute,
			wantExpired:	1,
		},
		{
			name:	"case 过期导致 ticket stale",
			setup: func(eng *casefencegate.Engine, cq *mockCaseQuerier) {
				now := eng.now()
				cq.AddCase(casefencegate.CaseSnapshot{
					ID:	"c-bump-1", MovementID: "m-bump-1", UserID: "u-bump-1",
					State:	"open", PolicyVer: 1, FenceEpoch: 1,
				}, "tok-bump-1")
				_ = eng.store.SaveTicket(&casefencegate.CaseReleaseTicket{
					ID:	"tkt-bump-1", CaseID: "c-bump-1", MovementID: "m-bump-1",
					State:	casefencegate.TicketOpen, ExpiresAt: now.Add(30 * time.Minute),
					IssuedAt:	now, CreatedAt: now,
				})
			},
			advClock:	0,
			wantExpired:	1,
			wantBumped:	1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cq := newMockCaseQuerier()
			baseTime := time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC)
			now := baseTime
			eng := testEngineWithClock(cq, func() time.Time { return now })
			tc.setup(eng, cq)
			now = baseTime.Add(tc.advClock)
			result, err := eng.ExpireStaleTickets(context.Background())
			if err != nil {
				t.Fatalf("expire error: %v", err)
			}
			if len(result.Expired) != tc.wantExpired {
				t.Fatalf("want %d expired, got %d (expired=%v)", tc.wantExpired, len(result.Expired), result.Expired)
			}
			if len(result.Bumped) != tc.wantBumped {
				t.Fatalf("want %d bumped, got %d", tc.wantBumped, len(result.Bumped))
			}
		})
	}
}

// ──────────────────────────────────────────
// 大表驱动: Store 操作
// ──────────────────────────────────────────

func TestMemTicketStore_TableDriven(t *testing.T) {
	type testCase struct {
		name	string
		op	func(s *casefencegate.MemTicketStore,) error
		check	func(t *testing.T, s *casefencegate.MemTicketStore,)
	}
	tests := []testCase{
		{
			name:	"SaveTicket 和 GetTicket",
			op: func(s *casefencegate.MemTicketStore,) error {
				return s.SaveTicket(&casefencegate.CaseReleaseTicket{
					ID:	"t-store-001", CaseID: "c-001", MovementID: "m-001",
					UserID:	"u-001", State: casefencegate.TicketOpen,
				})
			},
			check: func(t *testing.T, s *casefencegate.MemTicketStore,) {
				tk, err := s.GetTicket("t-store-001")
				if err != nil {
					t.Fatalf("get: %v", err)
				}
				if tk.CaseID != "c-001" {
					t.Fatalf("case_id want c-001 got %s", tk.CaseID)
				}
			},
		},
		{
			name:	"GetTicket 不存在",
			op:	func(_ *casefencegate.MemTicketStore,) error { return nil },
			check: func(t *testing.T, s *casefencegate.MemTicketStore,) {
				_, err := s.GetTicket("nonexistent")
				if !errors.Is(err, casefencegate.ErrTicketNotFound) {
					t.Fatalf("want ErrTicketNotFound, got %v", err)
				}
			},
		},
		{
			name:	"GetTicketByCase",
			op: func(s *casefencegate.MemTicketStore,) error {
				return s.SaveTicket(&casefencegate.CaseReleaseTicket{
					ID:	"t-store-002", CaseID: "c-002", MovementID: "m-002",
					UserID:	"u-002", State: casefencegate.TicketApproved,
				})
			},
			check: func(t *testing.T, s *casefencegate.MemTicketStore,) {
				tk, err := s.GetTicketByCase("c-002")
				if err != nil {
					t.Fatalf("get by case: %v", err)
				}
				if tk.ID != "t-store-002" {
					t.Fatalf("id want t-store-002 got %s", tk.ID)
				}
			},
		},
		{
			name:	"GetTicketByMovement",
			op: func(s *casefencegate.MemTicketStore,) error {
				return s.SaveTicket(&casefencegate.CaseReleaseTicket{
					ID:	"t-store-003", CaseID: "c-003", MovementID: "m-003",
					UserID:	"u-003", State: casefencegate.TicketOpen,
				})
			},
			check: func(t *testing.T, s *casefencegate.MemTicketStore,) {
				tk, err := s.GetTicketByMovement("m-003")
				if err != nil {
					t.Fatalf("get by movement: %v", err)
				}
				if tk.ID != "t-store-003" {
					t.Fatalf("id want t-store-003 got %s", tk.ID)
				}
			},
		},
		{
			name:	"ListActiveTickets 只返回 open",
			op: func(s *casefencegate.MemTicketStore,) error {
				_ = s.SaveTicket(&casefencegate.CaseReleaseTicket{ID: "t-act-1", State: casefencegate.TicketOpen, IssuedAt: time.Now()})
				_ = s.SaveTicket(&casefencegate.CaseReleaseTicket{ID: "t-act-2", State: casefencegate.TicketApproved, IssuedAt: time.Now()})
				_ = s.SaveTicket(&casefencegate.CaseReleaseTicket{ID: "t-act-3", State: casefencegate.TicketOpen, IssuedAt: time.Now()})
				return nil
			},
			check: func(t *testing.T, s *casefencegate.MemTicketStore,) {
				active, err := s.ListActiveTickets()
				if err != nil {
					t.Fatalf("list active: %v", err)
				}
				if len(active) != 2 {
					t.Fatalf("want 2 active, got %d", len(active))
				}
			},
		},
		{
			name:	"SaveProof 和 GetProof",
			op: func(s *casefencegate.MemTicketStore,) error {
				return s.SaveProof(&casefencegate.CaseDualDecideProof{
					ID:	"p-001", CaseID: "c-p-001", Sealed: true,
				})
			},
			check: func(t *testing.T, s *casefencegate.MemTicketStore,) {
				p, err := s.GetProof("p-001")
				if err != nil {
					t.Fatalf("get proof: %v", err)
				}
				if !p.Sealed {
					t.Fatal("proof must be sealed")
				}
			},
		},
		{
			name:	"ListProofs 按 case 过滤",
			op: func(s *casefencegate.MemTicketStore,) error {
				_ = s.SaveProof(&casefencegate.CaseDualDecideProof{ID: "p-f-1", CaseID: "c-f-1"})
				_ = s.SaveProof(&casefencegate.CaseDualDecideProof{ID: "p-f-2", CaseID: "c-f-2"})
				_ = s.SaveProof(&casefencegate.CaseDualDecideProof{ID: "p-f-3", CaseID: "c-f-1"})
				return nil
			},
			check: func(t *testing.T, s *casefencegate.MemTicketStore,) {
				proofs, err := s.ListProofs("c-f-1", 100)
				if err != nil {
					t.Fatalf("list proofs: %v", err)
				}
				if len(proofs) != 2 {
					t.Fatalf("want 2, got %d", len(proofs))
				}
			},
		},
		{
			name:	"SaveBareAlert 和 ListBareAlerts",
			op: func(s *casefencegate.MemTicketStore,) error {
				_ = s.SaveBareAlert(&casefencegate.BareFenceAlert{ID: "a-001", CaseID: "c-a-001", Blocked: true})
				_ = s.SaveBareAlert(&casefencegate.BareFenceAlert{ID: "a-002", CaseID: "c-a-002", Blocked: true})
				return nil
			},
			check: func(t *testing.T, s *casefencegate.MemTicketStore,) {
				alerts, err := s.ListBareAlerts("", 100)
				if err != nil {
					t.Fatalf("list alerts: %v", err)
				}
				if len(alerts) != 2 {
					t.Fatalf("want 2, got %d", len(alerts))
				}
			},
		},
		{
			name:	"SaveFenceLease 和 GetFenceLeaseByCase",
			op: func(s *casefencegate.MemTicketStore,) error {
				return s.SaveFenceLease(&casefencegate.CaseFenceLease{
					ID:	"fl-001", CaseID: "c-fl-001", FenceEpoch: 1, FenceToken: "tok-fl-1",
				})
			},
			check: func(t *testing.T, s *casefencegate.MemTicketStore,) {
				fl, err := s.GetFenceLeaseByCase("c-fl-001")
				if err != nil {
					t.Fatalf("get fence lease: %v", err)
				}
				if fl.FenceEpoch != 1 {
					t.Fatalf("want epoch 1, got %d", fl.FenceEpoch)
				}
			},
		},
		{
			name:	"ListActiveFenceLeases 排除已释放",
			op: func(s *casefencegate.MemTicketStore,) error {
				_ = s.SaveFenceLease(&casefencegate.CaseFenceLease{ID: "fl-act-1", CaseID: "c-fl-act-1", Released: false})
				_ = s.SaveFenceLease(&casefencegate.CaseFenceLease{ID: "fl-act-2", CaseID: "c-fl-act-2", Released: true})
				return nil
			},
			check: func(t *testing.T, s *casefencegate.MemTicketStore,) {
				active, err := s.ListActiveFenceLeases()
				if err != nil {
					t.Fatalf("list active leases: %v", err)
				}
				if len(active) != 1 {
					t.Fatalf("want 1, got %d", len(active))
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := casefencegate.NewMemTicketStore()
			if err := tc.op(s); err != nil {
				t.Fatalf("setup: %v", err)
			}
			tc.check(t, s)
		})
	}
}

// ──────────────────────────────────────────
// 大表驱动: AuditTrail
// ──────────────────────────────────────────

func TestAuditTrail_TableDriven(t *testing.T) {
	type testCase struct {
		name	string
		records	[]casefencegate.AuditRecord
		queryID	string
		limit	int
		wantN	int
	}
	tests := []testCase{
		{
			name:	"空审计",
			wantN:	0,
		},
		{
			name:	"插入 3 条查全部",
			records: []casefencegate.AuditRecord{
				{CaseID: "c-1", Actor: "a-1", Action: "act1"},
				{CaseID: "c-2", Actor: "a-2", Action: "act2"},
				{CaseID: "c-1", Actor: "a-3", Action: "act3"},
			},
			wantN:	3,
		},
		{
			name:	"按 case 过滤",
			records: []casefencegate.AuditRecord{
				{CaseID: "c-1", Actor: "a-1", Action: "act1"},
				{CaseID: "c-2", Actor: "a-2", Action: "act2"},
				{CaseID: "c-1", Actor: "a-3", Action: "act3"},
			},
			queryID:	"c-1",
			wantN:		2,
		},
		{
			name:	"limit 截断",
			records: []casefencegate.AuditRecord{
				{CaseID: "c-1", Actor: "a-1", Action: "act1"},
				{CaseID: "c-1", Actor: "a-2", Action: "act2"},
				{CaseID: "c-1", Actor: "a-3", Action: "act3"},
			},
			queryID:	"c-1",
			limit:		2,
			wantN:		2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			trail := casefencegate.NewAuditTrail()
			for _, r := range tc.records {
				trail.Append(r)
			}
			limit := tc.limit
			if limit == 0 {
				limit = 200
			}
			got := trail.List(tc.queryID,
				limit)
			if len(got) != tc.wantN {
				t.Fatalf("want %d, got %d", tc.wantN, len(got))
			}
		})
	}
}

// ──────────────────────────────────────────
// 大表驱动: TicketState 辅助方法
// ──────────────────────────────────────────

func TestTicketState_Helpers(t *testing.T) {
	type testCase struct {
		state		casefencegate.TicketState
		terminal	bool
		active		bool
		decided		bool
	}
	tests := []testCase{
		{casefencegate.TicketOpen, false, true, false},
		{casefencegate.TicketApproved, true, false, true},
		{casefencegate.TicketDenied, true, false, true},
		{casefencegate.TicketExpired, true, false, false},
		{casefencegate.TicketStale, true, false, false},
	}
	for _, tc := range tests {
		t.Run(string(tc.state), func(t *testing.T) {
			if tc.state.Terminal() != tc.terminal {
				t.Fatalf("Terminal: want %v got %v", tc.terminal, tc.state.Terminal())
			}
			if tc.state.Active() != tc.active {
				t.Fatalf("Active: want %v got %v", tc.active, tc.state.Active())
			}
			if tc.state.Decided() != tc.decided {
				t.Fatalf("Decided: want %v got %v", tc.decided, tc.state.Decided())
			}
		})
	}
}

// ──────────────────────────────────────────
// 大表驱动: CaseReleaseTicket 辅助方法
// ──────────────────────────────────────────

func TestCaseReleaseTicket_Helpers(t *testing.T) {
	now := time.Date(2022, 6, 15, 10, 0, 0, 0, time.UTC)
	type testCase struct {
		name		string
		ticket		casefencegate.CaseReleaseTicket
		checkNow	time.Time
		expired		bool
		alive		bool
		matchPV		int
		match		bool
	}
	tests := []testCase{
		{
			name:	"active, not expired, match policy",
			ticket: casefencegate.CaseReleaseTicket{
				State:		casefencegate.TicketOpen,
				ExpiresAt:	now.Add(5 * time.Minute), PolicyVer: 3,
			},
			checkNow:	now,
			expired:	false,
			alive:		true,
			matchPV:	3,
			match:		true,
		},
		{
			name:	"expired by TTL",
			ticket: casefencegate.CaseReleaseTicket{
				State:	casefencegate.TicketOpen, ExpiresAt: now.Add(-1 * time.Minute), PolicyVer: 3,
			},
			checkNow:	now,
			expired:	true,
			alive:		false,
			matchPV:	3,
			match:		true,
		},
		{
			name:	"terminal state, not alive",
			ticket: casefencegate.CaseReleaseTicket{
				State:	casefencegate.TicketApproved, ExpiresAt: now.Add(5 * time.Minute), PolicyVer: 3,
			},
			checkNow:	now,
			expired:	false,
			alive:		false,
			matchPV:	3,
			match:		true,
		},
		{
			name:	"policy mismatch",
			ticket: casefencegate.CaseReleaseTicket{
				State:	casefencegate.TicketOpen, ExpiresAt: now.Add(5 * time.Minute), PolicyVer: 3,
			},
			checkNow:	now,
			expired:	false,
			alive:		true,
			matchPV:	4,
			match:		false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ticket.Expired(tc.checkNow) != tc.expired {
				t.Fatalf("Expired: want %v got %v", tc.expired, tc.ticket.Expired(tc.checkNow))
			}
			if tc.ticket.Alive(tc.checkNow) != tc.alive {
				t.Fatalf("Alive: want %v got %v", tc.alive, tc.ticket.Alive(tc.checkNow))
			}
			if tc.ticket.MatchPolicy(tc.matchPV) != tc.match {
				t.Fatalf("MatchPolicy: want %v got %v", tc.match, tc.ticket.MatchPolicy(tc.matchPV))
			}
		})
	}
}

// ──────────────────────────────────────────
// 大表驱动: CaseSnapshot 辅助方法
// ──────────────────────────────────────────

func TestCaseSnapshot_Helpers(t *testing.T) {
	type testCase struct {
		state	string
		isOpen	bool
		isApp	bool
		isDen	bool
		isExp	bool
	}
	tests := []testCase{
		{"open", true, false, false, false},
		{"assigned", true, false, false, false},
		{"approved", false, true, false, false},
		{"denied", false, false, true, false},
		{"expired", false, false, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.state, func(t *testing.T) {
			cs := casefencegate.CaseSnapshot{State: tc.state}
			if cs.IsOpen() != tc.isOpen {
				t.Fatalf("IsOpen: want %v got %v", tc.isOpen, cs.IsOpen())
			}
			if cs.IsApproved() != tc.isApp {
				t.Fatalf("IsApproved: want %v got %v", tc.isApp, cs.IsApproved())
			}
			if cs.IsDenied() != tc.isDen {
				t.Fatalf("IsDenied: want %v got %v", tc.isDen, cs.IsDenied())
			}
			if cs.IsExpired() != tc.isExp {
				t.Fatalf("IsExpired: want %v got %v", tc.isExp, cs.IsExpired())
			}
		})
	}
}

// ──────────────────────────────────────────
// 大表驱动: CaseFenceLease.StillValid
// ──────────────────────────────────────────

func TestCaseFenceLease_StillValid(t *testing.T) {
	type testCase struct {
		name	string
		lease	*casefencegate.CaseFenceLease
		epoch	int64
		token	string
		valid	bool
	}
	tests := []testCase{
		{
			name:	"匹配",
			lease:	&casefencegate.CaseFenceLease{FenceEpoch: 1, FenceToken: "tok-1"},
			epoch:	1, token: "tok-1", valid: true,
		},
		{
			name:	"epoch 不匹配",
			lease:	&casefencegate.CaseFenceLease{FenceEpoch: 2, FenceToken: "tok-2"},
			epoch:	1, token: "tok-2", valid: false,
		},
		{
			name:	"token 不匹配",
			lease:	&casefencegate.CaseFenceLease{FenceEpoch: 1, FenceToken: "tok-1"},
			epoch:	1, token: "tok-wrong", valid: false,
		},
		{
			name:	"nil lease",
			lease:	nil,
			epoch:	1, token: "tok-1", valid: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.lease.StillValid(tc.epoch, tc.token) != tc.valid {
				t.Fatalf("StillValid: want %v", tc.valid)
			}
		})
	}
}

// ──────────────────────────────────────────
// 大表驱动: Duration JSON 序列化
// ──────────────────────────────────────────

func TestDuration_JSON(t *testing.T) {
	type testCase struct {
		name	string
		dur	casefencegate.Duration
		json	string
	}
	tests := []testCase{
		{"5 分钟", casefencegate.Duration(5 * time.Minute), `"5m0s"`},
		{"1 小时", casefencegate.Duration(time.Hour), `"1h0m0s"`},
		{"500 毫秒", casefencegate.Duration(500 * time.Millisecond), `"500ms"`},
		{"零值", casefencegate.Duration(0), `"0s"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data,
				err := tc.dur.MarshalJSON()
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(data) != tc.json {
				t.Fatalf("want %s got %s", tc.json, string(data))
			}
			var d casefencegate.Duration
			if err := d.UnmarshalJSON(data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if d != tc.dur {
				t.Fatalf("roundtrip mismatch: want %v got %v", tc.dur, d)
			}
		})
	}
}

// ──────────────────────────────────────────
// 整合场景: 完整审批 + Submit 流程
// ──────────────────────────────────────────

func TestEndToEnd_ApproveAndSubmit(t *testing.T) {
	cq := newMockCaseQuerier()
	cq.AddCase(casefencegate.CaseSnapshot{
		ID:	"e2e-001", MovementID: "mv-e2e-001", UserID: "user-e2e-001",
		State:	"open", PolicyVer: 10, FenceEpoch: 1,
	}, "tok-e2e-1")
	eng := testEngine(cq)
	ctx := context.Background()
	dr, err := eng.DecideFencedOnly(ctx, casefencegate.DecideFencedReq{
		CaseID:	"e2e-001", MovementID: "mv-e2e-001", Actor: "reviewer-e2e-A",
		Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-e2e-1",
	})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if dr.Ticket == nil {
		t.Fatal("want ticket")
	}
	sr, err := eng.SubmitCheck(ctx, casefencegate.SubmitCheckReq{
		TicketID:	dr.Ticket.ID,
		MovementID:	"mv-e2e-001",
		PolicyVer:	10,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if !sr.Allowed {
		t.Fatalf("want allowed, reason: %s", sr.Reason)
	}
	audit := eng.Audit("e2e-001", 100)
	if len(audit) < 2 {
		t.Fatalf("want at least 2 audit records, got %d", len(audit))
	}
}

func TestEndToEnd_DenyThenSubmitFails(t *testing.T) {
	cq := newMockCaseQuerier()
	cq.AddCase(casefencegate.CaseSnapshot{
		ID:	"e2e-deny-001", MovementID: "mv-e2e-deny-001", UserID: "user-deny-001",
		State:	"open", PolicyVer: 3, FenceEpoch: 1,
	}, "tok-deny-1")
	eng := testEngine(cq)
	ctx := context.Background()
	dr, err := eng.DecideFencedOnly(ctx, casefencegate.DecideFencedReq{
		CaseID:	"e2e-deny-001", MovementID: "mv-e2e-deny-001", Actor: "reviewer-deny",
		Decision:	"deny", FenceEpoch: 1, FenceToken: "tok-deny-1",
	})
	if err != nil {
		t.Fatalf("deny: %v", err)
	}
	sr, err := eng.SubmitCheck(ctx, casefencegate.SubmitCheckReq{
		TicketID:	dr.Ticket.ID,
		MovementID:	"mv-e2e-deny-001",
		PolicyVer:	3,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if sr.Allowed {
		t.Fatal("denied ticket should not allow submit")
	}
}

// ──────────────────────────────────────────
// 整合场景: 竞态 Expire + Approve 同围栏 epoch
// ──────────────────────────────────────────

func TestExpireAndApproveRace(t *testing.T) {
	cq := newMockCaseQuerier()
	cq.AddCase(casefencegate.CaseSnapshot{
		ID:	"race-001", MovementID: "mv-race-001", UserID: "user-race-001",
		State:	"open", PolicyVer: 1, FenceEpoch: 1,
	}, "tok-race-1")
	eng := testEngine(cq)
	ctx := context.Background()
	_, _ = eng.ExpireStaleTickets(ctx)
	_, err := eng.DecideFencedOnly(ctx, casefencegate.DecideFencedReq{
		CaseID:	"race-001", MovementID: "mv-race-001", Actor: "reviewer-race",
		Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-race-1",
	})
	if err == nil {
		t.Fatal("approve after expire should fail")
	}
	if !errors.Is(err, casefencegate.ErrStaleFence) && !strings.Contains(err.Error(), "not open") && !errors.Is(err, casefencegate.ErrCaseNotOpen) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ──────────────────────────────────────────
// : nil engine
// ──────────────────────────────────────────

func TestNilEngine(t *testing.T) {
	var eng *casefencegate.Engine
	ctx := context.Background()
	_, err := eng.DecideFencedOnly(ctx, casefencegate.DecideFencedReq{})
	if !errors.Is(err, casefencegate.ErrEngineNil) {
		t.Fatalf("want ErrEngineNil, got %v", err)
	}
	_, err = eng.BareDecideBlock(ctx, casefencegate.BareDecideReq{})
	if !errors.Is(err, casefencegate.ErrEngineNil) {
		t.Fatalf("want ErrEngineNil, got %v", err)
	}
	_, err = eng.SubmitCheck(ctx, casefencegate.SubmitCheckReq{})
	if !errors.Is(err, casefencegate.ErrEngineNil) {
		t.Fatalf("want ErrEngineNil, got %v", err)
	}
	_, err = eng.ExpireStaleTickets(ctx)
	if !errors.Is(err, casefencegate.ErrEngineNil) {
		t.Fatalf("want ErrEngineNil, got %v", err)
	}
	err = eng.ExpireApproveInterleave(ctx, "x")
	if !errors.Is(err, casefencegate.ErrEngineNil) {
		t.Fatalf("want ErrEngineNil, got %v", err)
	}
	_, err = eng.GetTicket(ctx, "x")
	if !errors.Is(err, casefencegate.ErrEngineNil) {
		t.Fatalf("want ErrEngineNil, got %v", err)
	}
	_, err = eng.GetProof(ctx, "x")
	if !errors.Is(err, casefencegate.ErrEngineNil) {
		t.Fatalf("want ErrEngineNil, got %v", err)
	}
	_, err = eng.ExportProofs(ctx, casefencegate.ProofExportReq{})
	if !errors.Is(err, casefencegate.ErrEngineNil) {
		t.Fatalf("want ErrEngineNil, got %v", err)
	}
}

// ──────────────────────────────────────────
// : nil store
// ──────────────────────────────────────────

func TestNilStore(t *testing.T) {
	var s *casefencegate.MemTicketStore
	_, err := s.GetTicket("x")
	if !errors.Is(err, casefencegate.ErrStoreNil) {
		t.Fatalf("want ErrStoreNil, got %v", err)
	}
	err = s.SaveTicket(&casefencegate.CaseReleaseTicket{})
	if !errors.Is(err, casefencegate.ErrStoreNil) {
		t.Fatalf("want ErrStoreNil, got %v", err)
	}
	_, err = s.ListActiveTickets()
	if !errors.Is(err, casefencegate.ErrStoreNil) {
		t.Fatalf("want ErrStoreNil, got %v", err)
	}
	_, err = s.GetProof("x")
	if !errors.Is(err, casefencegate.ErrStoreNil) {
		t.Fatalf("want ErrStoreNil, got %v", err)
	}
	err = s.SaveProof(&casefencegate.CaseDualDecideProof{})
	if !errors.Is(err, casefencegate.ErrStoreNil) {
		t.Fatalf("want ErrStoreNil, got %v", err)
	}
}

// ──────────────────────────────────────────
// 大表驱动: Config.withDefaults
// ──────────────────────────────────────────

func TestConfig_WithDefaults(t *testing.T) {
	type testCase struct {
		name	string
		cfg	casefencegate.Config
		chk	func(t *testing.T, c casefencegate.Config,)
	}
	tests := []testCase{
		{
			name:	"零值填充默认",
			cfg:	casefencegate.Config{},
			chk: func(t *testing.T, c casefencegate.Config,) {
				t.Helper()
				if c.DefaultTTL != 5*time.Minute {
					t.Fatalf("DefaultTTL want 5m got %v", c.DefaultTTL)
				}
				if c.MaxTTL != 30*time.Minute {
					t.Fatalf("MaxTTL want 30m got %v", c.MaxTTL)
				}
				if c.ExpireAge != 24*time.Hour {
					t.Fatalf("ExpireAge want 24h got %v", c.ExpireAge)
				}
				if c.MaxProofsExport != 500 {
					t.Fatalf("MaxProofsExport want 500 got %d", c.MaxProofsExport)
				}
			},
		},
		{
			name:	"自定义不被覆盖",
			cfg: casefencegate.Config{DefaultTTL: 10 * time.Minute,
				MaxTTL:	1 * time.Hour, MaxProofsExport: 200},
			chk: func(t *testing.T,
				c casefencegate.Config,) {
				t.Helper()
				if c.DefaultTTL != 10*time.Minute {
					t.Fatalf("DefaultTTL want 10m got %v", c.DefaultTTL)
				}
				if c.MaxTTL != 1*time.Hour {
					t.Fatalf("MaxTTL want 1h got %v", c.MaxTTL)
				}
				if c.MaxProofsExport != 200 {
					t.Fatalf("MaxProofsExport want 200 got %d", c.MaxProofsExport)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.cfg.withDefaults()
			tc.chk(t, c)
		})
	}
}

// ──────────────────────────────────────────
// ProofExport 查询
// ──────────────────────────────────────────

func TestExportProofs(t *testing.T) {
	cq := newMockCaseQuerier()
	cq.AddCase(casefencegate.CaseSnapshot{
		ID:	"ep-001", MovementID: "mv-ep-001", UserID: "user-ep-001",
		State:	"open", PolicyVer: 1, FenceEpoch: 1,
	}, "tok-ep-1")
	eng := testEngine(cq)
	ctx := context.Background()
	_, err1 := eng.DecideFencedOnly(ctx, casefencegate.DecideFencedReq{
		CaseID:	"ep-001", MovementID: "mv-ep-001", Actor: "rev-ep-A",
		Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-ep-1",
	})
	if err1 != nil {
		t.Fatalf("first approve: %v", err1)
	}
	result2, err2 := eng.DecideFencedOnly(ctx, casefencegate.DecideFencedReq{
		CaseID:	"ep-001", MovementID: "mv-ep-001", Actor: "rev-ep-B",
		Decision:	"approve", FenceEpoch: 1, FenceToken: "tok-ep-1",
	})
	if err2 == nil {
		t.Fatal("second approve should fail (stale fence or not open)")
	}
	if !errors.Is(err2, casefencegate.ErrStaleFence) && !errors.Is(err2, casefencegate.ErrCaseNotOpen) {
		t.Fatalf("second approve should get ErrStaleFence or ErrCaseNotOpen, got %v", err2)
	}
	if errors.Is(err2, casefencegate.ErrStaleFence) && result2 != nil && result2.Proof != nil {
		proofs, err := eng.ExportProofs(ctx, casefencegate.ProofExportReq{CaseID: "ep-001"})
		if err != nil {
			t.Fatalf("export: %v", err)
		}
		if len(proofs) == 0 {
			t.Fatal("want at least 1 proof")
		}
		if !proofs[0].Sealed {
			t.Fatal("proof must be sealed")
		}
	} else {
		proofs, err := eng.ExportProofs(ctx, casefencegate.ProofExportReq{CaseID: "ep-001"})
		if err != nil {
			t.Fatalf("export: %v", err)
		}
		_ = proofs
	}
}

// ──────────────────────────────────────────
// FSM guard 辅助函数
// ──────────────────────────────────────────

func TestFSMGuards(t *testing.T) {
	type testCase struct {
		name	string
		st	casefencegate.TicketState
		can	map[string]bool
	}
	tests := []testCase{
		{
			name:	"open",
			st:	casefencegate.TicketOpen,
			can:	map[string]bool{"approve": true, "deny": true, "expire": true, "stale": true},
		},
		{
			name:	"approved",
			st:	casefencegate.TicketApproved,
			can:	map[string]bool{"approve": false, "deny": false, "expire": false, "stale": false},
		},
		{
			name:	"denied",
			st:	casefencegate.TicketDenied,
			can:	map[string]bool{"approve": false, "deny": false, "expire": false, "stale": false},
		},
		{
			name:	"expired",
			st:	casefencegate.TicketExpired,
			can:	map[string]bool{"approve": false, "deny": false, "expire": false, "stale": false},
		},
		{
			name:	"stale",
			st:	casefencegate.TicketStale,
			can:	map[string]bool{"approve": false, "deny": false, "expire": false, "stale": false},
		},
	}
	guardFns := map[string]func(casefencegate.TicketState,) bool{
		"approve":	casefencegate.CanApprove,
		"deny":		casefencegate.CanDeny,
		"expire":	casefencegate.CanExpire,
		"stale":	casefencegate.CanStale,
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for ev, want := range tc.can {
				fn := guardFns[ev]
				if fn(tc.st) != want {
					t.Fatalf("%s(%s): want %v", ev, tc.st, want)
				}
			}
		})
	}
}

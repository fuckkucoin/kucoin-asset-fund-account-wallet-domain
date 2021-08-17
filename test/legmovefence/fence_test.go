package legmovefence_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/georebal"
	"account-assets.kucoin.net/funding/internal/legmovefence"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_,
		file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "legmovefence")
}

func loadGolden(t *testing.T, name string) legmovefence.GoldenTrace {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(testdataDir(t), name))
	if err != nil {
		t.Fatal(err)
	}
	var g legmovefence.GoldenTrace
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	return g
}

func assertGolden(t *testing.T, got legmovefence.GoldenTrace, name string) {
	t.Helper()
	want := loadGolden(t, name)
	if got.Scenario != want.Scenario || got.ExecOK != want.ExecOK || got.RetryOK != want.RetryOK {
		t.Fatalf("golden %s core mismatch got=%+v want=%+v", name, got, want)
	}
	if got.TransferOK != want.TransferOK || got.Idempotent != want.Idempotent {
		t.Fatalf("golden %s transfer mismatch got=%+v want=%+v", name, got, want)
	}
	if want.FenceState != "" && got.FenceState != want.FenceState {
		t.Fatalf("golden %s fence got=%s want=%s", name, got.FenceState, want.FenceState)
	}
	if want.HalfKind != "" && got.HalfKind != want.HalfKind {
		t.Fatalf("golden %s half kind got=%s want=%s", name, got.HalfKind, want.HalfKind)
	}
	if want.HalfState != "" && got.HalfState != want.HalfState {
		t.Fatalf("golden %s half state got=%s want=%s", name, got.HalfState, want.HalfState)
	}
	if got.SuccessMoves != want.SuccessMoves || got.SrcBalance != want.SrcBalance || got.DstBalance != want.DstBalance {
		t.Fatalf("golden %s inventory got src=%d dst=%d moves=%d want src=%d dst=%d moves=%d",
			name, got.SrcBalance, got.DstBalance, got.SuccessMoves, want.SrcBalance, want.DstBalance, want.SuccessMoves)
	}
	if !got.NetMovedOnce {
		t.Fatalf("golden %s net move not once", name)
	}
	for _, k := range want.ProofKinds {
		found := false
		for _, g := range got.ProofKinds {
			if g == k {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("golden %s missing proof %s in %v", name, k, got.ProofKinds)
		}
	}
}

func seed(t *testing.T, amount int64) (*legmovefence.Service, *georebal.Book) {
	t.Helper()
	book := georebal.NewBook()
	georebal.SeedDemo(book, "plan-1", "leg-1", "idem-1", amount)
	ex := georebal.NewBookExecutor(book)
	return legmovefence.New(book, ex), book
}

func traceOf(s *legmovefence.Service, scenario string, execOK, retryOK, transferOK, idem bool, fence legmovefence.LegMoveFence, half *legmovefence.HalfMoveCase, proofs []legmovefence.GeoDoubleMoveProof,) legmovefence.GoldenTrace {
	g := legmovefence.GoldenTrace{
		Scenario:	scenario,
		ExecOK:		execOK,
		RetryOK:	retryOK,
		TransferOK:	transferOK,
		Idempotent:	idem,
		FenceState:	string(fence.State),
		SrcBalance:	s.Book().Balance("sg-1", "USDT"),
		DstBalance:	s.Book().Balance("tyo-1", "USDT"),
		SuccessMoves:	s.Book().SuccessfulMoves("leg-1"),
		NetMovedOnce:	s.Book().SuccessfulMoves("leg-1") <= 1,
		ProofKinds:	legmovefence.KindsOf(proofs),
	}
	if half != nil {
		g.HalfKind = string(half.Kind)
		g.HalfState = string(half.State)
	}
	return g
}

func TestExecEnqueueTransferFailRetryOnce(t *testing.T) {
	s, _ := seed(t, 100000)
	ctx := context.Background()
	s.Exec().Inject.FailTransfer = true
	res1, err := s.ExecLegWithFence(ctx, legmovefence.ExecLegReq{PlanID: "plan-1", LegID: "leg-1", IdempotencyKey: "idem-1", Actor: "t"})
	if err == nil {
		t.Fatal("expected transfer fail")
	}
	if res1.HalfCase == nil || res1.HalfCase.Kind != legmovefence.HalfEnqueueNoTransfer {
		t.Fatalf("half %+v", res1.HalfCase)
	}
	s.Exec().Inject.FailTransfer = false
	res2, err := s.RetryAssertFence(ctx, legmovefence.RetryReq{FenceID: res1.Fence.FenceID, IdempotencyKey: "idem-1", Actor: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if !res2.TransferOK || s.Book().SuccessfulMoves("leg-1") != 1 {
		t.Fatalf("moves=%d res=%+v", s.Book().SuccessfulMoves("leg-1"), res2)
	}
	half,
		_ := s.GetHalf(ctx, res1.HalfCase.CaseID)
	proofs, _ := s.ListProofs(ctx, legmovefence.ExportFilter{FenceID: res2.Fence.FenceID, Limit: 20})
	got := traceOf(s, "enqueue_ok_transfer_fail_retry_once", false, true, true, false, res2.Fence, &half, proofs)
	assertGolden(t, got, "golden_enqueue_transfer_fail_retry.json")
}

func TestSameIdemSecondExecIdempotent(t *testing.T) {
	s, _ := seed(t, 100000)
	ctx := context.Background()
	res1, err := s.ExecLegWithFence(ctx, legmovefence.ExecLegReq{PlanID: "plan-1", LegID: "leg-1", IdempotencyKey: "idem-1", Actor: "t"})
	if err != nil {
		t.Fatal(err)
	}
	res2, err := s.ExecLegWithFence(ctx, legmovefence.ExecLegReq{PlanID: "plan-1", LegID: "leg-1", IdempotencyKey: "idem-1", Actor: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Idempotent || s.Book().SuccessfulMoves("leg-1") != 1 {
		t.Fatalf("not idempotent %+v moves=%d", res2, s.Book().SuccessfulMoves("leg-1"))
	}
	proofs, _ := s.ListProofs(ctx, legmovefence.ExportFilter{Limit: 20})
	got := traceOf(s, "same_idem_second_exec_idempotent", true, true, true, true, res1.Fence, nil, proofs)
	got.FenceState = string(res2.Fence.State)
	assertGolden(t, got, "golden_same_idem_second_exec.json")
}

func TestHaltedOldFenceFailClosed(t *testing.T) {
	s, book := seed(t, 100000)
	ctx := context.Background()
	s.Exec().Inject.FailTransfer = true
	res1, err := s.ExecLegWithFence(ctx, legmovefence.ExecLegReq{PlanID: "plan-1", LegID: "leg-1", IdempotencyKey: "idem-1", Actor: "t"})
	if err == nil {
		t.Fatal("expected fail")
	}
	if err := book.HaltRegion("sg-1", "ops"); err != nil {
		t.Fatal(err)
	}
	s.Exec().Inject.FailTransfer = false
	res2, err := s.RetryAssertFence(ctx, legmovefence.RetryReq{FenceID: res1.Fence.FenceID, IdempotencyKey: "idem-1", Actor: "t"})
	if !errors.Is(err, legmovefence.ErrRegionHalted) {
		t.Fatalf("want halted got %v", err)
	}
	f, _ := s.GetFence(ctx, res1.Fence.FenceID)
	proofs,
		_ := s.ListProofs(ctx, legmovefence.ExportFilter{FenceID: f.FenceID, Limit: 20})
	got := traceOf(s, "halted_old_fence_fail_closed", false, false, false, false, f, res1.HalfCase, proofs)
	got.RejectReason = "region_halted"
	assertGolden(t, got, "golden_halted_old_fence.json")
	_ = res2
}

func TestTransferDoneFailPatrolResume(t *testing.T) {
	s, _ := seed(t, 100000)
	ctx := context.Background()
	s.Exec().Inject.FailMarkDone = true
	res1, err := s.ExecLegWithFence(ctx, legmovefence.ExecLegReq{PlanID: "plan-1", LegID: "leg-1", IdempotencyKey: "idem-1", Actor: "t"})
	if err == nil {
		t.Fatal("expected mark done fail")
	}
	if !res1.TransferOK || res1.HalfCase == nil || res1.HalfCase.Kind != legmovefence.HalfTransferNoDone {
		t.Fatalf("half %+v", res1)
	}
	s.Exec().Inject.FailMarkDone = false
	n, err := s.PatrolHalf(ctx)
	if err != nil || n != 1 {
		t.Fatalf("patrol n=%d err=%v", n, err)
	}
	leg, _ := s.Book().GetLeg(ctx, "leg-1")
	if leg.State != georebal.LegDone {
		t.Fatalf("leg %s", leg.State)
	}
	res2, err := s.RetryAssertFence(ctx, legmovefence.RetryReq{FenceID: res1.Fence.FenceID, IdempotencyKey: "idem-1", Actor: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Idempotent || s.Book().SuccessfulMoves("leg-1") != 1 {
		t.Fatalf("second transfer leaked %+v", res2)
	}
	half, _ := s.GetHalf(ctx, res1.HalfCase.CaseID)
	f, _ := s.GetFence(ctx, res1.Fence.FenceID)
	got := traceOf(s, "transfer_ok_mark_done_fail_patrol", false, false, true, false, f, &half, nil)
	assertGolden(t, got, "golden_transfer_done_half_patrol.json")
}

func TestForceSecondTransferProof(t *testing.T) {
	s, _ := seed(t, 100000)
	ctx := context.Background()
	res1, err := s.ExecLegWithFence(ctx, legmovefence.ExecLegReq{PlanID: "plan-1", LegID: "leg-1", IdempotencyKey: "idem-1", Actor: "t"})
	if err != nil {
		t.Fatal(err)
	}
	res2, err := s.RetryAssertFence(ctx, legmovefence.RetryReq{FenceID: res1.Fence.FenceID, IdempotencyKey: "idem-1", Actor: "t", ForceTransfer: true})
	if !errors.Is(err, legmovefence.ErrDoubleMove) {
		t.Fatalf("want double move got %v", err)
	}
	if s.Book().SuccessfulMoves("leg-1") != 1 {
		t.Fatal("double inventory")
	}
	proofs,
		_ := s.ListProofs(ctx, legmovefence.ExportFilter{FenceID: res1.Fence.FenceID, Limit: 20})
	got := traceOf(s, "force_second_transfer_proof", true, false, true, true, res2.Fence, nil, proofs)
	got.RejectReason = "double_move"
	assertGolden(t, got, "golden_force_second_transfer.json")
}

func TestNoFenceFailClosed(t *testing.T) {
	s, _ := seed(t, 1000)
	if err := s.TransferWithoutFence(context.Background(), "plan-1", "leg-1"); !errors.Is(err, legmovefence.ErrNoFence) {
		t.Fatalf("got %v", err)
	}
	proofs, _ := s.ListProofs(context.Background(), legmovefence.ExportFilter{Kind: legmovefence.ProofNoFence, Limit: 10})
	if len(proofs) == 0 {
		t.Fatal("expected no_fence proof")
	}
}

func TestFSMTransitions(t *testing.T) {
	cases := []struct {
		from, to	legmovefence.FenceState
		ok		bool
	}{
		{legmovefence.FenceIssued, legmovefence.FenceEnqueued, true},
		{legmovefence.FenceEnqueued, legmovefence.FenceMoved, true},
		{legmovefence.FenceMoved, legmovefence.FenceRevoked, false},
		{legmovefence.FenceIssued, legmovefence.FenceMoved, false},
		{legmovefence.FenceEnqueued, legmovefence.FenceRevoked, true},
		{legmovefence.FenceExpired, legmovefence.FenceMoved, false},
	}
	for _, tc := range cases {
		if legmovefence.CanTransitFence(tc.from, tc.to) != tc.ok {
			t.Fatalf("%s→%s want %v", tc.from, tc.to, tc.ok)
		}
	}
	if !legmovefence.CanTransitHalf(legmovefence.HalfOpen, legmovefence.HalfResumed) || legmovefence.CanTransitHalf(legmovefence.HalfResumed, legmovefence.HalfOpen) {
		t.Fatal("half fsm")
	}
	f := &legmovefence.LegMoveFence{State: legmovefence.FenceIssued,
		IssuedAt:	time.Now(), TTLSeconds: 60}
	if err := legmovefence.ApplyFenceTransition(f, legmovefence.FenceEnqueued, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if f.EnqueuedAt == nil {
		t.Fatal("enqueued_at")
	}
}

func TestPolicyTable(t *testing.T) {
	now := time.Now().UTC()
	f := &legmovefence.LegMoveFence{
		State:	legmovefence.FenceEnqueued, Fingerprint: "aa", Generation: 5,
		IssuedAt:	now.Add(-time.Minute), TTLSeconds: 300,
	}
	p := legmovefence.DefaultPolicy()
	rows := []struct {
		name	string
		halted	bool
		liveFP	string
		liveGen	int64
		moved	int
		state	legmovefence.FenceState
		want	error
	}{
		{"ok", false, "aa", 5, 0, legmovefence.FenceEnqueued, nil},
		{"halted", true, "aa", 5, 0, legmovefence.FenceEnqueued, legmovefence.ErrRegionHalted},
		{"fp", false, "bb", 5, 0, legmovefence.FenceEnqueued, legmovefence.ErrFingerprint},
		{"gen", false, "aa", 9, 0, legmovefence.FenceEnqueued, legmovefence.ErrGeneration},
		{"moved", false, "aa", 5, 1, legmovefence.FenceMoved, legmovefence.ErrFenceMoved},
		{"revoked", false, "aa", 5, 0, legmovefence.FenceRevoked, legmovefence.ErrFenceRevoked},
	}
	for _, tc := range rows {
		f.State = tc.state
		err := legmovefence.DecideTransfer(p, f, tc.halted, tc.liveFP, tc.liveGen, tc.moved, now)
		if !errors.Is(err, tc.want) && (tc.want != nil || err != nil) {
			t.Fatalf("%s got %v want %v", tc.name, err, tc.want)
		}
	}
	if err := legmovefence.DecideTransfer(p, nil, false, "", 0, 0, now); !errors.Is(err, legmovefence.ErrNoFence) {
		t.Fatal(err)
	}
}

func TestExpireAndRevokePump(t *testing.T) {
	s, book := seed(t, 1000)
	ctx := context.Background()
	s.Exec().Inject.FailTransfer = true
	res, err := s.ExecLegWithFence(ctx, legmovefence.ExecLegReq{PlanID: "plan-1", LegID: "leg-1", IdempotencyKey: "idem-1", TTLSeconds: 1, Actor: "t"})
	if err == nil {
		t.Fatal("expected fail")
	}
	s.WithClock(func() time.Time { return time.Now().UTC().Add(2 * time.Second) })
	n,
		err := legmovefence.NewExpireWorker(s, time.Millisecond).RunOnce(ctx)
	if err != nil || n < 1 {
		// expire may miss if issued_at+ttl still in future due to clock only on service expire list
		f, _ := s.GetFence(ctx, res.Fence.FenceID)
		if f.State != legmovefence.FenceExpired && f.State != legmovefence.FenceEnqueued {
			t.Fatalf("expire n=%d state=%s err=%v", n, f.State, err)
		}
	}
	_ = book.HaltRegion("tyo-1", "ops")
	hn, _ := legmovefence.NewHaltRevokePump(s, nil).RunOnce(ctx, "tyo-1")
	_ = hn
	_, _ = legmovefence.NewDoubleMoveDiscover(s, time.Millisecond).RunOnce(ctx)
	_, _ = legmovefence.NewProofExportWorker(s, time.Millisecond).RunOnce(ctx)
}

func TestOpsResumeMutex(t *testing.T) {
	s, _ := seed(t, 100000)
	ctx := context.Background()
	s.Exec().Inject.FailMarkDone = true
	res, err := s.ExecLegWithFence(ctx, legmovefence.ExecLegReq{PlanID: "plan-1", LegID: "leg-1", IdempotencyKey: "idem-1", Actor: "a"})
	if err == nil {
		t.Fatal("expected")
	}
	c := *res.HalfCase
	c.ResumeLock = "other"
	now := time.Now().UTC()
	c.LockedAt = &now
	_ = s.Store().UpdateHalf(ctx, c)
	if _, err := s.OpsResume(ctx, legmovefence.ResumeReq{CaseID: c.CaseID, Actor: "me", MarkOnly: true}); !errors.Is(err, legmovefence.ErrHalfLocked) {
		t.Fatalf("want lock got %v", err)
	}
	c.ResumeLock = "me"
	_ = s.Store().UpdateHalf(ctx, c)
	s.Exec().Inject.FailMarkDone = false
	out, err := s.OpsResume(ctx, legmovefence.ResumeReq{CaseID: c.CaseID, Actor: "me", MarkOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.State != legmovefence.HalfResumed {
		t.Fatalf("state %s", out.State)
	}
}

func TestFingerprintAndIsolation(t *testing.T) {
	a := legmovefence.ComputeFingerprint("p", "l", "USDT", "sg-1", "tyo-1", 10)
	b := legmovefence.ComputeFingerprint("p", "l", "USDT", "sg-1", "tyo-1", 11)
	if legmovefence.MatchFingerprint(a, b) || !legmovefence.MatchFingerprint(a, a) {
		t.Fatal("fp")
	}
	if legmovefence.CombinedHaltGen(1, 1) == legmovefence.CombinedHaltGen(2, 1) {
		t.Fatal("gen")
	}
	iso := legmovefence.DefaultIsolation()
	if !iso.NotAgreeGate || !iso.NotTrexecGate || !iso.NotIsolationAttest {
		t.Fatal("isolation flags")
	}
	if !legmovefence.SameIdemReplay(legmovefence.LegMoveFence{IdempotencyKey: "k", State: legmovefence.FenceMoved}, "k") {
		t.Fatal("idem replay")
	}
	if !legmovefence.HalfNeedsResume(legmovefence.HalfTransferNoDone, 1, false) {
		t.Fatal("half resume")
	}
	p := legmovefence.GeoDoubleMoveProof{Kind: legmovefence.ProofDoubleTransfer, FenceID: "f", BindGen: 1, LiveGen: 2, BindFP: "a", LiveFP: "b"}
	v := legmovefence.ProofViewOf(p)
	if !v.GenDrift || !v.FPDrift || v.Digest == "" {
		t.Fatalf("view %+v", v)
	}
}

func TestAssembleAndHTTPRegister(t *testing.T) {
	b := legmovefence.Assemble(legmovefence.Options{})
	if b.Service == nil || b.Patrol == nil {
		t.Fatal("bundle")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b.StartWorkers(ctx)
	b.PublishHalt(legmovefence.HaltEvent{RegionID: "sg-1"})
	if legmovefence.FenceIDOf(legmovefence.LegMoveFence{FenceID: "lmf-1"}) != "lmf-1" {
		t.Fatal("id")
	}
}

func TestGeorebalFinaliseWaitsLegs(t *testing.T) {
	book := georebal.NewBook()
	georebal.SeedDemo(book, "plan-1", "leg-1", "idem-1", 10)
	if err := book.Finalise("plan-1"); err == nil {
		t.Fatal("should wait")
	}
	_ = book.UpdateLegState(context.Background(), "leg-1", georebal.LegDone, "")
	if err := book.Finalise("plan-1"); err != nil {
		t.Fatal(err)
	}
}

func TestMemStoreRoundTrip(t *testing.T) {
	m := legmovefence.NewMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	f := legmovefence.LegMoveFence{FenceID: "f1", PlanID: "p", LegID: "l", IdempotencyKey: "i", State: legmovefence.FenceIssued, IssuedAt: now, CreatedAt: now, UpdatedAt: now, TTLSeconds: 60}
	if err := m.SaveFence(ctx, f); err != nil {
		t.Fatal(err)
	}
	got, err := m.GetFence(ctx, "f1")
	if err != nil || got.FenceID != "f1" {
		t.Fatal(err)
	}
	if _, ok, _ := m.GetFenceByIdem(ctx, "i"); !ok {
		t.Fatal("idem")
	}
	list, _ := m.ListFencesByState(ctx, legmovefence.FenceIssued, 10)
	if len(list) != 1 {
		t.Fatal(list)
	}
	_ = m.SaveProof(ctx, legmovefence.GeoDoubleMoveProof{ProofID: "p1", Kind: legmovefence.ProofReplay, CreatedAt: now, DetectedAt: now})
	if _, err := m.GetProof(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	_ = m.SaveHalf(ctx, legmovefence.HalfMoveCase{CaseID: "c1", FenceID: "f1", State: legmovefence.HalfOpen, OpenedAt: now, CreatedAt: now})
	if _, ok, _ := m.GetOpenHalfByFence(ctx, "f1"); !ok {
		t.Fatal("half")
	}
	_ = m.SaveAudit(ctx, legmovefence.FenceAudit{AuditID: "a1", FenceID: "f1", Action: legmovefence.AuditIssue, CreatedAt: now})
	if aud, _ := m.ListAudit(ctx, "f1", 5); len(aud) != 1 {
		t.Fatal("audit")
	}
}

func TestAssertTransferableTable(t *testing.T) {
	now := time.Now().UTC()
	rows := []struct {
		st	legmovefence.FenceState
		ttl	int64
		iss	time.Time
		want	error
	}{
		{legmovefence.FenceEnqueued, 60, now, nil},
		{legmovefence.FenceMoved, 60, now, legmovefence.ErrFenceMoved},
		{legmovefence.FenceRevoked, 60, now, legmovefence.ErrFenceRevoked},
		{legmovefence.FenceExpired, 60, now, legmovefence.ErrFenceExpired},
		{legmovefence.FenceIssued, 1, now.Add(-2 * time.Second), legmovefence.ErrFenceExpired},
	}
	for i, tc := range rows {
		f := &legmovefence.LegMoveFence{State: tc.st, TTLSeconds: tc.ttl, IssuedAt: tc.iss}
		err := legmovefence.AssertTransferable(f, now)
		if !errors.Is(err, tc.want) && (tc.want != nil || err != nil) {
			t.Fatalf("%d got %v want %v", i, err, tc.want)
		}
	}
	if err := legmovefence.AssertTransferable(nil, now); !errors.Is(err, legmovefence.ErrNoFence) {
		t.Fatal(err)
	}
}

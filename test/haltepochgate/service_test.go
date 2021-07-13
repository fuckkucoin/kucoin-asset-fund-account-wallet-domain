package haltepochgate_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"account-assets.kucoin.net/funding/internal/haltepochgate"
)

func TestClearWindowRejectsDepositWithdraw(t *testing.T) {
	ctx := context.Background()
	svc := haltepochgate.New()
	ep, err := svc.EngageWithEpoch(ctx, haltepochgate.EngageRequest{Actor: "ops-a", Reason: "hot-wallet-anomaly"})
	if err != nil {
		t.Fatal(err)
	}
	if ep.Generation != 1 || ep.State != haltepochgate.EpochEngaged {
		t.Fatalf("engage: %+v", ep)
	}
	_, err = svc.SeedAllowed(ctx, haltepochgate.FundIntent{
		IntentID:	"fi-dep-preclear", Scope: ep.Scope, Lane: haltepochgate.LaneDeposit, Generation: ep.Generation, State: haltepochgate.IntentAllowed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VoteClear(ctx, ep.EpochID, "ops-a", "aye", true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VoteClear(ctx, ep.EpochID, "ops-b", "aye", true); err != nil {
		t.Fatal(err)
	}
	cleared, cs, err := svc.ClearWithEpoch(ctx, haltepochgate.ClearRequest{Actor: "ops-b", Reason: "quorum-clear"})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.State != haltepochgate.EpochCleared || cleared.Generation < 2 {
		t.Fatalf("clear: %+v", cleared)
	}
	if cs == nil || cs.RevokedIntents < 1 {
		t.Fatalf("expected revoked intents, case=%+v", cs)
	}
	for _, lane := range []haltepochgate.FundLane{haltepochgate.LaneDeposit, haltepochgate.LaneWithdraw} {
		_, conc, err := svc.LaneAssert(ctx, haltepochgate.LaneAssertRequest{Lane: lane, CarriedGen: cleared.Generation})
		if !errors.Is(err, haltepochgate.ErrLaneSealed) {
			t.Fatalf("lane %s: want sealed, got %v", lane, err)
		}
		if conc == nil || conc.ProofID == "" {
			t.Fatalf("lane %s: expected SlipProof", lane)
		}
	}
	_, conc, err := svc.LaneAssert(ctx, haltepochgate.LaneAssertRequest{Lane: haltepochgate.LaneDeposit, CarriedGen: 1})
	if !errors.Is(err, haltepochgate.ErrEpochStale) {
		t.Fatalf("stale: %v", err)
	}
	if conc == nil || conc.ProofID == "" {
		t.Fatal("stale must open SlipProof")
	}
	in, err := svc.AssertBroadcast(ctx, haltepochgate.BroadcastRequest{IntentID: "fi-dep-preclear", CarriedGen: 1, Actor: "pump"})
	if !errors.Is(err, haltepochgate.ErrBroadcastDenied) && !errors.Is(err, haltepochgate.ErrIntentRevoked) {
		t.Fatalf("broadcast: %v intent=%+v", err, in)
	}
	proofs, _ := svc.ListProofs(ctx, "", 50)
	if len(proofs) == 0 {
		t.Fatal("expected SlipProof after clear window")
	}
}

func TestEngageClearRaceMonotonic(t *testing.T) {
	ctx := context.Background()
	svc := haltepochgate.New()
	scope := haltepochgate.NormalizeScope("global", "", "")
	ep, err := svc.EngageWithEpoch(ctx, haltepochgate.EngageRequest{Actor: "ops-a", Reason: "initial"})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.VoteClear(ctx, ep.EpochID, "ops-a", "aye", true)
	_, _ = svc.VoteClear(ctx, ep.EpochID, "ops-b", "aye", true)
	out := svc.RunEngageClearRace(ctx, scope, "ops-c", "ops-b", "race")
	if out.FinalGen < ep.Generation {
		t.Fatalf("epoch regress: %+v", out)
	}
	if !haltepochgate.Monotonic(ep.Generation, out.FinalGen) {
		t.Fatal("not monotonic")
	}
	cur, err := svc.CurrentEpoch(ctx, "global", "", "")
	if err != nil {
		t.Fatal(err)
	}
	c := cur.LaneConclusion()
	if !haltepochgate.LaneMatchesEpoch(c, *cur) {
		t.Fatalf("lane mismatch conc=%+v ep=%+v", c, cur)
	}
	switch cur.State {
	case haltepochgate.EpochEngaged, haltepochgate.EpochReengaged, haltepochgate.EpochCleared, haltepochgate.EpochClearing, haltepochgate.EpochIdle:
	default:
		t.Fatalf("unexpected state %s", cur.State)
	}
}

func TestBridgeMustNotClear(t *testing.T) {
	ctx := context.Background()
	svc := haltepochgate.New()
	br := haltepochgate.BridgeEngage{Svc: svc}
	if err := br.Engage(ctx, "recon", "recon-drift"); err != nil {
		t.Fatal(err)
	}
	err := br.Clear(ctx, "recon", "must-fail")
	if !errors.Is(err, haltepochgate.ErrBridgeClear) {
		t.Fatalf("want bridge clear denied, got %v", err)
	}
	proofs, _ := svc.ListProofs(ctx, "", 20)
	found := false
	for _, p := range proofs {
		if p.Kind == haltepochgate.SlipBridgeClearAttempt {
			found = true
		}
	}
	if !found {
		t.Fatal("expected bridge_clear_attempt proof")
	}
}

func TestLaneAssertTable(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name	string
		setup	func(*haltepochgate.Service,) int64
		req	haltepochgate.LaneAssertRequest
		wantErr	error
	}{
		{
			name:	"missing epoch",
			setup: func(s *haltepochgate.Service,) int64 {
				_, _ = s.EngageWithEpoch(ctx, haltepochgate.EngageRequest{Actor: "a", Reason: "h"})
				return 0
			},
			req:		haltepochgate.LaneAssertRequest{Lane: haltepochgate.LaneDeposit, CarriedGen: 0},
			wantErr:	haltepochgate.ErrEpochMissing,
		},
		{
			name:	"stale epoch after engage",
			setup: func(s *haltepochgate.Service,) int64 {
				ep, _ := s.EngageWithEpoch(ctx, haltepochgate.EngageRequest{Actor: "a", Reason: "h"})
				return ep.Generation
			},
			req:		haltepochgate.LaneAssertRequest{Lane: haltepochgate.LaneWithdraw, CarriedGen: 99},
			wantErr:	haltepochgate.ErrEpochStale,
		},
		{
			name:		"unknown lane",
			setup:		func(*haltepochgate.Service,) int64 { return 1 },
			req:		haltepochgate.LaneAssertRequest{Lane: "xfer", CarriedGen: 1},
			wantErr:	haltepochgate.ErrLaneUnknown,
		},
		{
			name:	"idle open allows bind",
			setup:	func(*haltepochgate.Service,) int64 { return 0 },
			req: haltepochgate.LaneAssertRequest{Lane: haltepochgate.LaneSweep,
				CarriedGen:	0},
			wantErr:	haltepochgate.ErrEpochMissing,
		},
		{
			name:	"idempotent lane assert",
			setup: func(s *haltepochgate.Service,) int64 {
				return 0
			},
			req:		haltepochgate.LaneAssertRequest{Lane: haltepochgate.LaneDeposit, CarriedGen: 0, IdemKey: "idem-1"},
			wantErr:	haltepochgate.ErrEpochMissing,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := haltepochgate.New()
			_ = tc.setup(svc)
			_, _, err := svc.LaneAssert(ctx, tc.req)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v want %v", err, tc.wantErr)
			}
		})
	}
}

func TestIdempotentEngageClear(t *testing.T) {
	ctx := context.Background()
	svc := haltepochgate.New()
	a, err := svc.EngageWithEpoch(ctx, haltepochgate.EngageRequest{Actor: "a", Reason: "h", IdemKey: "e1"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.EngageWithEpoch(ctx, haltepochgate.EngageRequest{Actor: "a", Reason: "h", IdemKey: "e1"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Generation != b.Generation || a.EpochID != b.EpochID {
		t.Fatalf("engage not idempotent %d vs %d", a.Generation, b.Generation)
	}
	_, _ = svc.VoteClear(ctx, a.EpochID, "a", "aye", true)
	_, _ = svc.VoteClear(ctx, a.EpochID, "b", "aye", true)
	c1, _, err := svc.ClearWithEpoch(ctx, haltepochgate.ClearRequest{Actor: "b", Reason: "c", IdemKey: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	c2, _, err := svc.ClearWithEpoch(ctx, haltepochgate.ClearRequest{Actor: "b", Reason: "c", IdemKey: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if c1.Generation != c2.Generation {
		t.Fatalf("clear not idempotent")
	}
}

func TestQuorumDenied(t *testing.T) {
	ctx := context.Background()
	svc := haltepochgate.New()
	ep, _ := svc.EngageWithEpoch(ctx, haltepochgate.EngageRequest{Actor: "a", Reason: "h"})
	_, _ = svc.VoteClear(ctx, ep.EpochID, "a", "aye", true)
	_, _, err := svc.ClearWithEpoch(ctx, haltepochgate.ClearRequest{Actor: "a", Reason: "c"})
	if !errors.Is(err, haltepochgate.ErrQuorumDenied) {
		t.Fatalf("want quorum denied, got %v", err)
	}
}

func TestForceClearSkipsQuorum(t *testing.T) {
	ctx := context.Background()
	svc := haltepochgate.New()
	_, _ = svc.EngageWithEpoch(ctx, haltepochgate.EngageRequest{Actor: "a", Reason: "h"})
	ep, _, err := svc.ClearWithEpoch(ctx, haltepochgate.ClearRequest{Actor: "lead", Reason: "force", Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if ep.State != haltepochgate.EpochCleared {
		t.Fatalf("state %s", ep.State)
	}
}

func TestReadGateFailClosed(t *testing.T) {
	ctx := context.Background()
	svc := haltepochgate.New().WithHooks(haltepochgate.FailGate{Err: errors.New("kv down")}, nil, nil, nil, nil)
	_, conc, err := svc.LaneAssert(ctx, haltepochgate.LaneAssertRequest{Lane: haltepochgate.LaneDeposit, CarriedGen: 1})
	if !errors.Is(err, haltepochgate.ErrReadGateFailed) {
		t.Fatalf("got %v", err)
	}
	if conc == nil || !conc.FailClosed || conc.ProofID == "" {
		t.Fatalf("fail-closed conc=%+v", conc)
	}
}

func TestSealResumeThenBind(t *testing.T) {
	ctx := context.Background()
	svc := haltepochgate.New()
	_, _ = svc.EngageWithEpoch(ctx, haltepochgate.EngageRequest{Actor: "a", Reason: "h"})
	ep, _, err := svc.ClearWithEpoch(ctx, haltepochgate.ClearRequest{Actor: "b", Reason: "c", Force: true})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.LaneAssert(ctx, haltepochgate.LaneAssertRequest{Lane: haltepochgate.LaneDeposit, CarriedGen: ep.Generation})
	if !errors.Is(err, haltepochgate.ErrLaneSealed) {
		t.Fatalf("window must stay sealed: %v", err)
	}
	res, err := svc.SealResume(ctx, "global", "", "", "ops")
	if err != nil {
		t.Fatal(err)
	}
	if res.State != haltepochgate.EpochIdle || !res.LaneOpen {
		t.Fatalf("resume %+v", res)
	}
	in, conc, err := svc.LaneAssert(ctx, haltepochgate.LaneAssertRequest{Lane: haltepochgate.LaneDeposit, CarriedGen: res.Generation, IdemKey: "after-seal"})
	if err != nil {
		t.Fatal(err)
	}
	if in == nil || conc.Blocked {
		t.Fatalf("expected allow after seal in=%+v conc=%+v", in, conc)
	}
	in2, _, err := svc.LaneAssert(ctx, haltepochgate.LaneAssertRequest{Lane: haltepochgate.LaneDeposit, CarriedGen: res.Generation, IdemKey: "after-seal"})
	if err != nil {
		t.Fatal(err)
	}
	if in2.IntentID != in.IntentID {
		t.Fatal("idempotency broken")
	}
}

func TestCreditDeniedAfterEpochAdvance(t *testing.T) {
	ctx := context.Background()
	svc := haltepochgate.New()
	_, _ = svc.EngageWithEpoch(ctx, haltepochgate.EngageRequest{Actor: "a", Reason: "h"})
	_, _ = svc.SeedAllowed(ctx, haltepochgate.FundIntent{
		IntentID:	"fi-wd", Scope: haltepochgate.NormalizeScope("global", "", ""), Lane: haltepochgate.LaneWithdraw, Generation: 1, State: haltepochgate.IntentAllowed,
	})
	_, _, _ = svc.ClearWithEpoch(ctx, haltepochgate.ClearRequest{Actor: "b", Reason: "c", Force: true})
	_, err := svc.AssertCredit(ctx, haltepochgate.CreditRequest{IntentID: "fi-wd", CarriedGen: 1, Actor: "ledger"})
	if !errors.Is(err, haltepochgate.ErrCreditDenied) && !errors.Is(err, haltepochgate.ErrIntentRevoked) {
		t.Fatalf("got %v", err)
	}
}

func TestMutexBlocksCrossKind(t *testing.T) {
	ctx := context.Background()
	svc := haltepochgate.New()
	scope := haltepochgate.NormalizeScope("global", "", "")
	if err := svc.HoldMutex(ctx, scope, "clearer", "clear"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.EngageWithEpoch(ctx, haltepochgate.EngageRequest{Actor: "engager", Reason: "h"})
	if !errors.Is(err, haltepochgate.ErrClearInProgress) && !errors.Is(err, haltepochgate.ErrRaceMutex) {
		t.Fatalf("got %v", err)
	}
}

func TestWorkerScanAndProve(t *testing.T) {
	ctx := context.Background()
	svc := haltepochgate.New()
	_, _ = svc.EngageWithEpoch(ctx, haltepochgate.EngageRequest{Actor: "a", Reason: "h"})
	_, _ = svc.SeedAllowed(ctx, haltepochgate.FundIntent{
		IntentID:	"fi-old", Scope: haltepochgate.NormalizeScope("global", "", ""), Lane: haltepochgate.LaneSweep, Generation: 1, State: haltepochgate.IntentAllowed,
	})
	_, _, _ = svc.ClearWithEpoch(ctx, haltepochgate.ClearRequest{Actor: "b", Reason: "c", Force: true})
	w := haltepochgate.NewWorker(svc, haltepochgate.WorkerConfig{BatchSize: 20, AutoProve: true}, nil)
	if err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	open, _ := svc.ListProofs(ctx, haltepochgate.SlipOpen, 20)
	if len(open) != 0 {
		t.Fatalf("expected drain open proofs, still %d", len(open))
	}
	rep, err := svc.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Epochs < 1 {
		t.Fatal("inspect empty")
	}
}

func TestExportAndProveClose(t *testing.T) {
	ctx := context.Background()
	svc := haltepochgate.New()
	_, _ = svc.EngageWithEpoch(ctx, haltepochgate.EngageRequest{Actor: "a", Reason: "h"})
	_, _, _ = svc.LaneAssert(ctx, haltepochgate.LaneAssertRequest{Lane: haltepochgate.LaneDeposit, CarriedGen: 1})
	list, _ := svc.ListProofs(ctx, haltepochgate.SlipOpen, 10)
	if len(list) == 0 {
		t.Fatal("need proof")
	}
	p, err := svc.ProveSlip(ctx, list[0].ProofID)
	if err != nil || p.State != haltepochgate.SlipProven {
		t.Fatalf("prove %v %+v", err, p)
	}
	p, err = svc.CloseSlip(ctx, p.ProofID)
	if err != nil || p.State != haltepochgate.SlipClosed {
		t.Fatalf("close %v %+v", err, p)
	}
	ex, err := svc.ExportProofs(ctx, 10)
	if err != nil || ex.Count < 1 {
		t.Fatal(err)
	}
	if _, err := svc.ExportProofsJSON(ctx, 10); err != nil {
		t.Fatal(err)
	}
}

func TestFixturesGolden(t *testing.T) {
	dir := haltepochgate.TestdataDir()
	for _, name := range []string{"clear_window_slip.json", "engage_clear_race.json", "bridge_engage_only.json"} {
		tr, err := haltepochgate.LoadFixture(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if tr.Name == "" || len(tr.Steps) == 0 {
			t.Fatalf("%s empty", name)
		}
	}
	gdir := filepath.Join(dir, "golden")
	for _, name := range []string{"clear_window_summary.txt", "race_summary.txt", "bridge_summary.txt"} {
		b, err := os.ReadFile(filepath.Join(gdir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "trace=") {
			t.Fatalf("golden %s missing trace", name)
		}
	}
}

func TestFSMTransitions(t *testing.T) {
	st, gen, err := haltepochgate.NextOnEngage(nil)
	if err != nil || st != haltepochgate.EpochEngaged || gen != 1 {
		t.Fatalf("%s %d %v", st, gen, err)
	}
	_, _, err = haltepochgate.NextOnClear(nil)
	if !errors.Is(err, haltepochgate.ErrEpochNotFound) {
		t.Fatal(err)
	}
	if err := haltepochgate.CanAdvance(3, 2, false); !errors.Is(err, haltepochgate.ErrEpochRegress) {
		t.Fatal(err)
	}
	if err := haltepochgate.TransitionSlip(haltepochgate.SlipOpen, haltepochgate.SlipProven); err != nil {
		t.Fatal(err)
	}
	if err := haltepochgate.TransitionSlip(haltepochgate.SlipClosed, haltepochgate.SlipOpen); err == nil {
		t.Fatal("forbidden slip")
	}
	if err := haltepochgate.TransitionIntent(haltepochgate.IntentAllowed, haltepochgate.IntentRevoked); err != nil {
		t.Fatal(err)
	}
	if err := haltepochgate.CompatibleMutex("a", "clear", "b", "engage"); !errors.Is(err, haltepochgate.ErrClearInProgress) {
		t.Fatal(err)
	}
}

func TestValidateAndPolicy(t *testing.T) {
	if err := haltepochgate.ValidateEngage(haltepochgate.EngageRequest{}); !errors.Is(err, haltepochgate.ErrActorRequired) {
		t.Fatal(err)
	}
	if err := haltepochgate.ValidateClear(haltepochgate.ClearRequest{FromBridge: true}); !errors.Is(err, haltepochgate.ErrBridgeClear) {
		t.Fatal(err)
	}
	if _, err := haltepochgate.NormalizeLane("nope"); !errors.Is(err, haltepochgate.ErrLaneUnknown) {
		t.Fatal(err)
	}
	p := haltepochgate.DefaultSealPolicy()
	ep := haltepochgate.HaltEpoch{Generation: 2, State: haltepochgate.EpochCleared, LaneOpen: false}
	if err := p.MustRejectLane(ep, 0); !errors.Is(err, haltepochgate.ErrEpochMissing) {
		t.Fatal(err)
	}
	if err := p.MustRejectLane(ep, 1); !errors.Is(err, haltepochgate.ErrEpochStale) {
		t.Fatal(err)
	}
	if err := p.MustRejectLane(ep, 2); !errors.Is(err, haltepochgate.ErrLaneSealed) {
		t.Fatal(err)
	}
	if err := p.MustRejectBridge(true); !errors.Is(err, haltepochgate.ErrBridgeClear) {
		t.Fatal(err)
	}
}

func TestScopeAndLaneHelpers(t *testing.T) {
	k := haltepochgate.NormalizeScope("pair", "eth", "usdt")
	if k.String() != "pair:ETH/USDT" {
		t.Fatal(k.String())
	}
	if !haltepochgate.LaneDeposit.Valid() || haltepochgate.FundLane("x").Valid() {
		t.Fatal("lane valid")
	}
	if haltepochgate.SlipKindForLane(haltepochgate.LaneWithdraw) != haltepochgate.SlipClearWindowWithdraw {
		t.Fatal("kind")
	}
	if !haltepochgate.EpochEngaged.Halted() || haltepochgate.EpochIdle.WindowSealed() {
		t.Fatal("state flags")
	}
	if haltepochgate.EpochDigest(haltepochgate.HaltEpoch{Scope: k, Generation: 1, State: haltepochgate.EpochEngaged}) == "" {
		t.Fatal("digest")
	}
}

func TestCustodyhaltEpochView(t *testing.T) {
	// 只读挂钩编译期存在于 custodyhalt；此处校验本包映射。
	if haltepochgate.MapLane(haltepochgate.LaneSweep) == "" {
		t.Fatal("map")
	}
	if haltepochgate.ActorOf("  x  ") != "x" {
		t.Fatal("actor")
	}
}

func TestHTTPStatusMap(t *testing.T) {
	if haltepochgate.StatusOf(haltepochgate.ErrEpochMissing) != 400 {
		t.Fatal()
	}
	if haltepochgate.StatusOf(haltepochgate.ErrBridgeClear) != 409 {
		t.Fatal()
	}
	if haltepochgate.StatusOf(haltepochgate.ErrEpochNotFound) != 404 {
		t.Fatal()
	}
	if haltepochgate.StatusOf(haltepochgate.ErrReadGateFailed) != 503 {
		t.Fatal()
	}
}

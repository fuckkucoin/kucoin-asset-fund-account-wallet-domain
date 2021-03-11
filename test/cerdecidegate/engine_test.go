package cerdecidegate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/cerdecidegate"
)

func testEngine() (*cerdecidegate.Engine, *cerdecidegate.MemorySource, *cerdecidegate.MemStore,) {
	st := cerdecidegate.NewMemStore()
	src := cerdecidegate.NewMemorySource()
	eng := cerdecidegate.NewEngine(st, src, cerdecidegate.WithMetrics(cerdecidegate.NewMetrics()), cerdecidegate.WithClock(func() time.Time {
		return time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	}))
	return eng, src, st
}

func TestGoldenThresholdFlipReject(t *testing.T) {
	g, err := cerdecidegate.LoadGolden("threshold_flip_reject.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	eng, src, st := testEngine()
	ctx := context.Background()
	ln := g.line()
	pin := g.pin()
	src.PutPin(ln, pin)
	lease, err := eng.DecideWithLease(ctx, cerdecidegate.DecideReq{
		Actor:	"ops-desk-01", IdempotencyKey: "idem-th-001", Line: ln, Pin: pin,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if lease.LeaseID == "" || lease.State != cerdecidegate.LeaseIssued {
		t.Fatalf("lease %+v", lease)
	}
	src.FlipThreshold(ln, g.LiveThreshold, g.LiveShares)
	_, err = eng.AssertLease(ctx, cerdecidegate.BindReq{LeaseID: lease.LeaseID, Actor: "ops-desk-01", Line: ln, ExecRef: "exec-1"})
	if err == nil || err.Error() != g.Expected.ExecError {
		t.Fatalf("assert err=%v want %s", err, g.Expected.ExecError)
	}
	proofs, _ := st.ListProofsByLease(ctx, lease.LeaseID)
	if !hasKind(proofs, cerdecidegate.ProofKind(g.Expected.ProofKind)) {
		t.Fatalf("proofs=%+v", proofs)
	}
}

func TestGoldenDestRiskRaise(t *testing.T) {
	g, err := cerdecidegate.LoadGolden("dest_risk_raise.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	eng, src, st := testEngine()
	ctx := context.Background()
	ln := g.line()
	pin := g.pin()
	src.PutPin(ln, pin)
	lease, err := eng.DecideWithLease(ctx, cerdecidegate.DecideReq{
		Actor:	"ops-desk-02", IdempotencyKey: "idem-risk-002", Line: ln, Pin: pin,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	src.PutRisk(ln, g.LiveDestRisk)
	n, err := eng.RaiseDestRisk(ctx, ln.BatchID, g.QuotedDestRisk, g.LiveDestRisk, g.PolicyID)
	if err != nil {
		t.Fatalf("raise: %v", err)
	}
	if n < 1 {
		t.Fatalf("revoked %d", n)
	}
	_, err = eng.AssertLease(ctx, cerdecidegate.BindReq{LeaseID: lease.LeaseID, Actor: "ops-desk-02", Line: ln})
	if !errors.Is(err, cerdecidegate.ErrLeaseRevoked) && !errors.Is(err, cerdecidegate.ErrRiskRaise) {
		t.Fatalf("assert after raise: %v", err)
	}
	proofs, _ := st.ListProofsByLease(ctx, lease.LeaseID)
	if !hasKind(proofs, cerdecidegate.ProofRiskRaise) && !hasKind(proofs, cerdecidegate.ProofRevoked) {
		t.Fatalf("proofs=%+v", proofs)
	}
}

func TestGoldenExemptNoForceLease(t *testing.T) {
	g, err := cerdecidegate.LoadGolden("exempt_no_force_lease.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	eng, _, _ := testEngine()
	ctx := context.Background()
	ln := g.line()
	pin := g.pin()
	_, err = eng.DecideWithLease(ctx, cerdecidegate.DecideReq{
		Actor:	"ops-desk-03", IdempotencyKey: "idem-ex-003", Line: ln, Pin: pin,
	})
	if err == nil || err.Error() != g.Expected.IssueError {
		t.Fatalf("issue err=%v want %s", err, g.Expected.IssueError)
	}
	_, err = eng.AssertLease(ctx, cerdecidegate.BindReq{LeaseID: "", Actor: "ops-desk-03", Line: ln})
	if err == nil || err.Error() != g.Expected.NoLeaseExecError {
		t.Fatalf("no lease err=%v want %s", err, g.Expected.NoLeaseExecError)
	}
}

func TestGoldenForceColdFlip(t *testing.T) {
	g, err := cerdecidegate.LoadGolden("force_cold_flip.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	eng, src, st := testEngine()
	ctx := context.Background()
	ln := g.line()
	pin := g.pin()
	src.PutPin(ln, pin)
	lease, err := eng.DecideWithLease(ctx, cerdecidegate.DecideReq{
		Actor:	"ops-desk-04", IdempotencyKey: "idem-fc-004", Line: ln, Pin: pin,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	src.FlipForce(ln, false)
	_, err = eng.AssertLease(ctx, cerdecidegate.BindReq{LeaseID: lease.LeaseID, Actor: "ops-desk-04", Line: ln})
	if err == nil || err.Error() != g.Expected.ExecError {
		t.Fatalf("assert err=%v want %s", err, g.Expected.ExecError)
	}
	proofs, _ := st.ListProofsByLease(ctx, lease.LeaseID)
	if !hasKind(proofs, cerdecidegate.ProofKind(g.Expected.ProofKind)) {
		t.Fatalf("proofs=%+v", proofs)
	}
}

func TestGoldenBindConsumeOK(t *testing.T) {
	g, err := cerdecidegate.LoadGolden("bind_consume_ok.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	eng, src, _ := testEngine()
	ctx := context.Background()
	ln := g.line()
	pin := g.pin()
	src.PutPin(ln, pin)
	lease, err := eng.DecideWithLease(ctx, cerdecidegate.DecideReq{
		Actor:	"ops-desk-05", IdempotencyKey: "idem-ok-005", Line: ln, Pin: pin,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	bound, err := eng.AssertLease(ctx, cerdecidegate.BindReq{LeaseID: lease.LeaseID, Actor: "ops-desk-05", Line: ln, ExecRef: "exec-ok"})
	if err != nil {
		t.Fatalf("assert: %v", err)
	}
	if string(bound.State) != g.Expected.StateAfterAssert {
		t.Fatalf("state %s", bound.State)
	}
	got, err := eng.ConsumeLease(ctx, cerdecidegate.ConsumeReq{LeaseID: lease.LeaseID, Actor: "ops-desk-05", ExecRef: "exec-ok"})
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if string(got.State) != g.Expected.StateAfterConsume {
		t.Fatalf("consume state %s", got.State)
	}
}

func TestTableIdemAndTTL(t *testing.T) {
	cases := []struct {
		name	string
		ttl	int64
		want	error
	}{
		{name: "default_ttl", ttl: 0, want: nil},
		{name: "too_long", ttl: 86400, want: cerdecidegate.ErrTTL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eng, src, _ := testEngine()
			ln := cerdecidegate.LineFact{BatchID: "batch-ttl-" + tc.name, Asset: "BTC", Chain: "BTC", Amount: "1", Dest: "bc1qcerdecidegate00ttl", DestRisk: "high", Side: "cold", Purpose: "withdraw"}
			pin := cerdecidegate.DecidePin{Required: true, PolicyID: "pol-ttl", Threshold: 2, Shares: 3, BindMode: "one", DestRisk: "high"}
			src.PutPin(ln, pin)
			_, err := eng.DecideWithLease(context.Background(), cerdecidegate.DecideReq{
				Actor:	"ops-desk-06", IdempotencyKey: "idem-ttl-" + tc.name, TTLSec: tc.ttl, Line: ln, Pin: pin,
			})
			if !errors.Is(err, tc.want) && (tc.want != nil || err != nil) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
		})
	}
}

func TestIdemReplaySamePayload(t *testing.T) {
	eng, src, _ := testEngine()
	ctx := context.Background()
	ln := cerdecidegate.LineFact{BatchID: "batch-idem", Asset: "ETH", Chain: "ETH", Amount: "3", Dest: "0xcerdecidegateidem", DestRisk: "medium", Side: "cold", Purpose: "withdraw"}
	pin := cerdecidegate.DecidePin{Required: true, PolicyID: "pol-idem", Threshold: 2, Shares: 3, BindMode: "one", DestRisk: "medium"}
	src.PutPin(ln, pin)
	a, err := eng.DecideWithLease(ctx, cerdecidegate.DecideReq{Actor: "ops-desk-07", IdempotencyKey: "idem-same", Line: ln, Pin: pin})
	if err != nil {
		t.Fatal(err)
	}
	b, err := eng.DecideWithLease(ctx, cerdecidegate.DecideReq{Actor: "ops-desk-07", IdempotencyKey: "idem-same", Line: ln, Pin: pin})
	if err != nil {
		t.Fatal(err)
	}
	if a.LeaseID != b.LeaseID {
		t.Fatalf("idem mismatch %s %s", a.LeaseID, b.LeaseID)
	}
	pin.Threshold = 4
	_, err = eng.DecideWithLease(ctx, cerdecidegate.DecideReq{Actor: "ops-desk-07", IdempotencyKey: "idem-same", Line: ln, Pin: pin})
	if !errors.Is(err, cerdecidegate.ErrIdem) {
		t.Fatalf("conflict: %v", err)
	}
}

func TestWorkerExpiry(t *testing.T) {
	st := cerdecidegate.NewMemStore()
	src := cerdecidegate.NewMemorySource()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	clk := &now
	eng := cerdecidegate.NewEngine(st, src, cerdecidegate.WithClock(func() time.Time { return *clk }))
	ln := cerdecidegate.LineFact{BatchID: "batch-exp", Asset: "BTC", Chain: "BTC", Amount: "1", Dest: "bc1qcerdecidegate00exp", DestRisk: "low", Side: "cold", Purpose: "withdraw"}
	pin := cerdecidegate.DecidePin{Required: true, PolicyID: "pol-exp", Threshold: 2, Shares: 2, BindMode: "one", DestRisk: "low"}
	src.PutPin(ln, pin)
	lease, err := eng.DecideWithLease(context.Background(), cerdecidegate.DecideReq{
		Actor:	"ops-desk-08", IdempotencyKey: "idem-exp", TTLSec: 60, Line: ln, Pin: pin,
	})
	if err != nil {
		t.Fatal(err)
	}
	*clk = now.Add(2 * time.Minute)
	w := cerdecidegate.NewWorker(eng,
		st, cerdecidegate.NewMetrics(), cerdecidegate.WorkerConfig{BatchSize: 16})
	n, err := w.ScanExpiry(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("expired n=%d err=%v", n, err)
	}
	got, _ := st.GetLease(context.Background(), lease.LeaseID)
	if got.State != cerdecidegate.LeaseExpired {
		t.Fatalf("state %s", got.State)
	}
}

func TestSharesDriftTable(t *testing.T) {
	eng, src, _ := testEngine()
	ctx := context.Background()
	ln := cerdecidegate.LineFact{BatchID: "batch-sh", Asset: "ETH", Chain: "ETH", Amount: "9", Dest: "0xcerdecidegateshares", DestRisk: "high", Side: "cold", Purpose: "withdraw"}
	pin := cerdecidegate.DecidePin{Required: true, PolicyID: "pol-sh", Threshold: 3, Shares: 5, BindMode: "one", DestRisk: "high"}
	src.PutPin(ln, pin)
	lease, err := eng.DecideWithLease(ctx, cerdecidegate.DecideReq{Actor: "ops-desk-09", IdempotencyKey: "idem-sh", Line: ln, Pin: pin})
	if err != nil {
		t.Fatal(err)
	}
	src.FlipThreshold(ln, 3, 8)
	_, err = eng.AssertLease(ctx, cerdecidegate.BindReq{LeaseID: lease.LeaseID, Actor: "ops-desk-09", Line: ln})
	if !errors.Is(err, cerdecidegate.ErrSharesDrift) {
		t.Fatalf("err=%v", err)
	}
}

func hasKind(ps []cerdecidegate.CerDriftProof, k cerdecidegate.ProofKind,) bool {
	for _, p := range ps {
		if p.Kind == k {
			return true
		}
	}
	return false
}

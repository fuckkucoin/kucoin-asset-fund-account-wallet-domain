package batchhopgate_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/batchhopgate"
)

func TestAssertPartialConserve(t *testing.T) {
	cases := []struct {
		name	string
		hops	[]batchhopgate.HopSummary
		wantErr	bool
	}{
		{"empty ok", nil, false},
		{"paired hops", makeSucceededHops(2), false},
		{"same book", []batchhopgate.HopSummary{{Index: 0, From: "funding", To: "funding", Asset: "USDT", Amount: "1"}}, true},
		{"empty amount", []batchhopgate.HopSummary{{Index: 0, From: "funding", To: "trading", Asset: "USDT"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := batchhopgate.AssertPartialConserve(tc.hops)
			if tc.wantErr && err == nil {
				t.Fatal("want err")
			}
			if !tc.wantErr && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPartialConserveHold(t *testing.T) {
	e := batchhopgate.NewPartialConserveEngine()
	e.WithClock(fixedClock(epoch))
	c := &batchhopgate.BatchResidualCase{
		ID:	"c-hold", UserID: "u-h", Asset: "USDT", Generation: 1,
		SucceededHops:	makeSucceededHops(2),
	}
	h := e.PinHold(c)
	if h == nil || !e.HasHold("u-h", "USDT") {
		t.Fatal("hold missing")
	}
	if d := batchhopgate.DiffHoldVsCase(h, c); d != "" {
		t.Fatal(d)
	}
	e.ReleaseHold("u-h", "USDT")
	if e.HasHold("u-h", "USDT") {
		t.Fatal("hold should release")
	}
}

func TestIsolationGuard(t *testing.T) {
	g := batchhopgate.NewIsolationGuard()
	if !g.RejectsNeighborWrite("acctpol.PolicyVersion") {
		t.Fatal("must reject acctpol write")
	}
	if !g.AcceptsReadonlyHook("portfolio.HopResult") {
		t.Fatal("must accept hop result")
	}
	if !g.IsOutputID("batch_residual_id") {
		t.Fatal("output id")
	}
	if batchhopgate.AssertNoAcctpolScan("acctpol.policy_gen") {
		t.Fatal("acctpol scan must fail")
	}
	if !batchhopgate.AssertNoAcctpolScan("portfolio_batch_residuals") {
		t.Fatal("portfolio table should pass")
	}
}

func TestInspectorScan(t *testing.T) {
	st := batchhopgate.NewMemStore()
	ctx := context.Background()
	_ = st.SaveResidual(ctx, &batchhopgate.BatchResidualCase{
		ID:	"ins-1", UserID: "u-i", Asset: "USDT", Status: batchhopgate.ResidualOpen,
		DetectedAt:	epoch.Add(-3 * batchhopgate.ResumeLeaseTTL), UpdatedAt: epoch,
	})
	ins := batchhopgate.NewInspector(st).WithClock(fixedClock(epoch))
	rep, err := ins.Scan(ctx, 10, 2*batchhopgate.ResumeLeaseTTL)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OpenCount != 1 || len(rep.StaleOpen) != 1 {
		t.Fatalf("%+v", rep)
	}
	if batchhopgate.FormatReport(rep) == "" {
		t.Fatal("empty format")
	}
}

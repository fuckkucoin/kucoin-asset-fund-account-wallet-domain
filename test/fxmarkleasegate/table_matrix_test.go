package fxmarkleasegate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/fxmarkleasegate"
)

func TestIssueMatrix(t *testing.T) {
	type tc struct {
		name	string
		from	string
		to	string
		usd	map[string]string
		purpose	string
		ttl	string
		wantErr	error
	}
	cases := []tc{
		{"ok eth usdt", "ETH", "USDT", map[string]string{"ETH": "1", "USDT": "1"}, "convert", "30s", nil},
		{"ok btc usdc", "BTC", "USDC", map[string]string{"BTC": "2", "USDC": "1"}, "fund_page", "20s", nil},
		{"same pair", "ETH", "ETH", map[string]string{"ETH": "1"}, "convert", "20s", fxmarkleasegate.ErrPairInvalid},
		{"empty from", "", "USDT", map[string]string{"USDT": "1"}, "convert", "20s", fxmarkleasegate.ErrPairRequired},
		{"empty to", "ETH", "", map[string]string{"ETH": "1"}, "convert", "20s", fxmarkleasegate.ErrPairRequired},
		{"bad purpose", "ETH", "USDT", map[string]string{"ETH": "1", "USDT": "1"}, "listing", "20s", fxmarkleasegate.ErrPurposeInvalid},
		{"bad ttl", "ETH", "USDT", map[string]string{"ETH": "1", "USDT": "1"}, "convert", "-1s", fxmarkleasegate.ErrTTLInvalid},
		{"missing from usd", "AAA", "USDT", map[string]string{"USDT": "1"}, "convert", "20s", fxmarkleasegate.ErrRateMissing},
		{"missing to usd", "ETH", "BBB", map[string]string{"ETH": "1"}, "convert", "20s", fxmarkleasegate.ErrRateMissing},
		{"valuation ok", "ETH", "USDC", map[string]string{"ETH": "9", "USDC": "1"}, "valuation", "45s", nil},
		{"travel ok", "BTC", "USDT", map[string]string{"BTC": "3", "USDT": "1"}, "travel", "10s", nil},
		{"zero rate", "ETH", "USDT", map[string]string{"ETH": "0", "USDT": "1"}, "convert", "20s", fxmarkleasegate.ErrRateMissing},
		{"neg rate", "ETH", "USDT", map[string]string{"ETH": "-1", "USDT": "1"}, "convert", "20s", fxmarkleasegate.ErrRateMissing},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			rd := &fxmarkleasegate.StaticReader{USDMap: c.usd, Gens: map[string]int64{}}
			e := fxmarkleasegate.New(fxmarkleasegate.Config{Store: fxmarkleasegate.NewMemStore(),
				Reader:	rd})
			_, err := e.IssueMarkLease(context.Background(), fxmarkleasegate.IssueMarkLeaseReq{
				From:	c.from,
				To:	c.to, Purpose: c.purpose, TTL: c.ttl,
			})
			if c.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected %v", err)
				}
				return
			}
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("got %v want %v", err, c.wantErr)
			}
		})
	}
}

func TestDriftKindMatrix(t *testing.T) {
	kinds := []struct {
		kind	fxmarkleasegate.DriftKind
		err	error
	}{
		{fxmarkleasegate.DriftPutChange, fxmarkleasegate.ErrPutInvalidated},
		{fxmarkleasegate.DriftGeneration, fxmarkleasegate.ErrPutInvalidated},
		{fxmarkleasegate.DriftLiveOverThresh, fxmarkleasegate.ErrLiveOverThreshold},
		{fxmarkleasegate.DriftHTTPLive, fxmarkleasegate.ErrLiveOverThreshold,
		},
		{fxmarkleasegate.DriftFingerprint, fxmarkleasegate.ErrFingerprintDrift},
		{fxmarkleasegate.DriftSourceFlip, fxmarkleasegate.ErrFingerprintDrift},
		{fxmarkleasegate.DriftExpired, fxmarkleasegate.ErrLeaseExpired},
		{fxmarkleasegate.DriftMissingRate, fxmarkleasegate.ErrMissingRate},
		{fxmarkleasegate.DriftRevoked, fxmarkleasegate.ErrLeaseRevoked},
		{fxmarkleasegate.DriftBinding, fxmarkleasegate.ErrBindingMismatch},
		{fxmarkleasegate.DriftAlreadyConsumed, fxmarkleasegate.ErrLeaseConsumed},
		{fxmarkleasegate.DriftNotBound, fxmarkleasegate.ErrLeaseNotBound},
		{fxmarkleasegate.DriftMissingLease, fxmarkleasegate.ErrMissingLease,
		},
	}
	for _, k := range kinds {
		if !errors.Is(fxmarkleasegate.FailOf(k.kind), k.err) {
			t.Fatalf("%s -> %v want %v", k.kind, fxmarkleasegate.FailOf(k.kind), k.err)
		}
	}
}

func TestThresholdMatrix(t *testing.T) {
	rows := []struct {
		lease, live	string
		th		int64
		over		bool
	}{
		{"100", "100", 10, false},
		{"100", "100.05", 10, false},
		{"100", "100.2", 10, true},
		{"40000", "40000", 10, false},
		{"40000", "42000", 10, true},
		{"1", "1.0005", 10, false},
		{"1", "1.002", 10, true},
		{"bad", "1", 10, true},
		{"1", "x", 10, true},
	}
	for i, r := range rows {
		if fxmarkleasegate.OverThreshold(r.lease, r.live, r.th) != r.over {
			t.Fatalf("#%d %s vs %s over=%v", i, r.lease, r.live, r.over)
		}
	}
}

func TestPurposeNormalizeMatrix(t *testing.T) {
	rows := []struct {
		in	string
		want	string
		err	bool
	}{
		{"", fxmarkleasegate.PurposeConvert, false},
		{"convert", fxmarkleasegate.PurposeConvert, false},
		{"fund_page", fxmarkleasegate.PurposeFundPage, false},
		{"fund-page", fxmarkleasegate.PurposeFundPage, false},
		{"assets", fxmarkleasegate.PurposeFundPage, false},
		{"valuation", fxmarkleasegate.PurposeValuation, false},
		{"value", fxmarkleasegate.PurposeValuation, false},
		{"travel_nominal", fxmarkleasegate.PurposeTravel, false},
		{"travel", fxmarkleasegate.PurposeTravel, false},
		{"oracle", "", true},
		{"dust", "", true},
		{"cvfund", "", true},
	}
	for _, r := range rows {
		got, err := fxmarkleasegate.NormalizePurpose(r.in)
		if r.err && err == nil {
			t.Fatalf("%s should err", r.in)
		}
		if !r.err && (err != nil || got != r.want) {
			t.Fatalf("%s got %s %v", r.in, got, err)
		}
	}
}

func TestNeighborBoundaryUntouched(t *testing.T) {
	for _, n := range fxmarkleasegate.NeighborReadOnly() {
		if n == "fxmarkleasegate" {
			t.Fatal("self is not neighbor")
		}
	}
	if !fxmarkleasegate.SameBookPair("ETH", "USDT") || fxmarkleasegate.SameBookPair("ETH", "ETH") {
		t.Fatal("pair")
	}
	if fxmarkleasegate.DefaultThresholdFor("travel") < fxmarkleasegate.DefaultThresholdBPS {
		t.Fatal("travel thresh")
	}
}

func TestIdemIssue(t *testing.T) {
	e, _ := testEng(t)
	a, err := e.IssueMarkLease(context.Background(), fxmarkleasegate.IssueMarkLeaseReq{From: "ETH", To: "USDT", Idem: "k1"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.IssueMarkLease(context.Background(), fxmarkleasegate.IssueMarkLeaseReq{From: "ETH", To: "USDT", Idem: "k1"})
	if err != nil || a.LeaseID != b.LeaseID {
		t.Fatalf("%+v %+v %v", a, b, err)
	}
	_, err = e.IssueMarkLease(context.Background(), fxmarkleasegate.IssueMarkLeaseReq{From: "BTC", To: "USDT", Idem: "k1"})
	if !errors.Is(err, fxmarkleasegate.ErrIdemConflict) {
		t.Fatalf("got %v", err)
	}
}

func TestBindingMismatch(t *testing.T) {
	e, _ := testEng(t)
	iss, err := e.IssueMarkLease(context.Background(), fxmarkleasegate.IssueMarkLeaseReq{From: "ETH", To: "USDT"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.AssertBeforePost(context.Background(), fxmarkleasegate.AssertBeforePostReq{
		LeaseID:	iss.LeaseID, From: "BTC", To: "USDT", Consume: true,
	})
	if !errors.Is(err, fxmarkleasegate.ErrBindingMismatch) {
		t.Fatalf("got %v", err)
	}
}

func TestCatalogHelpers(t *testing.T) {
	if !fxmarkleasegate.StableAsset("usdt") || fxmarkleasegate.StableAsset("ETH") {
		t.Fatal("stable")
	}
	if !fxmarkleasegate.AllowedPurpose("convert") || fxmarkleasegate.AllowedPurpose("gateway") {
		t.Fatal("purpose")
	}
	if !fxmarkleasegate.PurposeNeedsLive("travel") || fxmarkleasegate.PurposeNeedsLive("convert") {
		t.Fatal("live purpose")
	}
}

func TestMemStoreRoundtrip(t *testing.T) {
	ctx := context.Background()
	m := fxmarkleasegate.NewMemStore()
	now := time.Now().UTC()
	l := fxmarkleasegate.MarkLease{ID: "mkl-1", From: "ETH", To: "USDT", Rate: "1", State: fxmarkleasegate.LeaseIssued, IssuedAt: now, ExpiresAt: now.Add(time.Minute)}
	if _, err := m.SaveLease(ctx, l); err != nil {
		t.Fatal(err)
	}
	got, err := m.GetLease(ctx, "mkl-1")
	if err != nil || got.From != "ETH" {
		t.Fatal(got, err)
	}
	if _, err := m.SaveProof(ctx, fxmarkleasegate.FxMarkDriftProof{ID: "p1", LeaseID: "mkl-1", Kind: fxmarkleasegate.DriftPutChange, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	ps, _ := m.ListProofsByLease(ctx, "mkl-1", 10)
	if len(ps) != 1 {
		t.Fatal(ps)
	}
	_, _ = m.SaveAudit(ctx, fxmarkleasegate.MarkLeaseAudit{ID: "a1", LeaseID: "mkl-1", At: now})
	_, _ = m.SaveGeneration(ctx, fxmarkleasegate.MarkGeneration{Asset: "ETH", Seq: 2, LastUSD: "3", UpdatedAt: now})
	if n, _ := m.CountGenerations(ctx); n != 1 {
		t.Fatal(n)
	}
}

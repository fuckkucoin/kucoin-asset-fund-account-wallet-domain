package oracletickgate_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/oracle"
	"account-assets.kucoin.net/funding/internal/oracletickgate"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "oracletickgate")
}

func loadGolden(t *testing.T, name string, dest any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(testdataDir(t), name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, dest); err != nil {
		t.Fatal(err)
	}
}

func seedFeed(t *testing.T, now time.Time, price string) *oracle.Feed {
	t.Helper()
	f := oracle.NewFeed(30 * time.Second).WithClock(func() time.Time { return now })
	if err := f.Put(oracle.Tick{Asset: "BTC", Quote: "USDT", Price: price, At: now}); err != nil {
		t.Fatal(err)
	}
	if err := f.Put(oracle.Tick{Asset: "ETH", Quote: "USDT", Price: "3000", At: now}); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestTickFlipOldMTMLeaseRefused(t *testing.T) {
	var g struct {
		Expect	struct {
			LeaseState	string	`json:"lease_state"`
			ProofKind	string	`json:"proof_kind"`
			FailClosed	bool	`json:"fail_closed"`
			FundWriteHeld	bool	`json:"fund_write_held"`
			Posted		bool	`json:"posted"`
		}	`json:"expect"`
		Input	struct {
			PriceBefore	string	`json:"price_before"`
			PriceAfter	string	`json:"price_after"`
		}	`json:"input"`
	}
	loadGolden(t, "golden_tick_flip_mtm.json", &g)
	now := time.Date(2021, 12, 18, 8, 0, 0, 0, time.UTC)
	feed := seedFeed(t, now, g.Input.PriceBefore)
	svc := oracletickgate.New(feed).WithClock(func() time.Time { return now })
	ctx := context.Background()
	lease, err := svc.IssueTickLease(ctx, oracletickgate.IssueTickLeaseRequest{
		Quote:	"USDT", Amounts: map[string]string{"BTC": "1"}, ValuationKind: oracletickgate.ValMTM, Mode: oracletickgate.ModeEnforce, Actor: "ops-mtm",
	})
	if err != nil {
		t.Fatal(err)
	}
	if lease.LeaseID == "" || lease.Generation == 0 {
		t.Fatalf("lease %+v", lease)
	}
	if err := feed.Put(oracle.Tick{Asset: "BTC", Quote: "USDT", Price: g.Input.PriceAfter, At: now}); err != nil {
		t.Fatal(err)
	}
	got, rec, err := svc.AssertBeforeFundWrite(ctx, oracletickgate.AssertFundWriteRequest{
		LeaseID:	lease.LeaseID, WriteKind: oracletickgate.WriteCollateralMTM, Actor: "ops-mtm",
	})
	if err == nil || rec != nil {
		t.Fatalf("want refuse, rec=%+v err=%v", rec, err)
	}
	if !errors.Is(err, oracletickgate.ErrLeaseRevoked) && !errors.Is(err, oracletickgate.ErrFingerprint) && !errors.Is(err, oracletickgate.ErrGeneration) && !errors.Is(err, oracletickgate.ErrPriceDrift) {
		t.Fatalf("err=%v", err)
	}
	if got == nil || string(got.State) != g.Expect.LeaseState {
		t.Fatalf("lease %+v", got)
	}
	proofs, _ := svc.store.ListProofsByLease(ctx, lease.LeaseID)
	if !oracletickgate.HasKind(proofs, oracletickgate.ProofKind(g.Expect.ProofKind)) && !oracletickgate.HasKind(proofs, oracletickgate.ProofPriceDrift) && !oracletickgate.HasKind(proofs, oracletickgate.ProofGeneration) {
		t.Fatalf("proofs %+v", proofs)
	}
	if len(proofs) == 0 || !proofs[0].FailClosed || !proofs[0].FundWriteHeld {
		t.Fatalf("proof flags %+v", proofs)
	}
	if g.Expect.Posted && rec != nil && rec.Posted {
		t.Fatal("posted")
	}
}

func TestTickAtExpiredRecheckFails(t *testing.T) {
	var g struct {
		Expect struct {
			RecheckOK	bool	`json:"recheck_ok"`
			ProofKind	string	`json:"proof_kind"`
			FailClosed	bool	`json:"fail_closed"`
		} `json:"expect"`
	}
	loadGolden(t, "golden_tick_stale.json", &g)
	now := time.Date(2021, 12, 18, 9, 0, 0, 0, time.UTC)
	cur := now
	feed := oracle.NewFeed(30 * time.Second).WithClock(func() time.Time { return cur })
	if err := feed.Put(oracle.Tick{Asset: "BTC", Quote: "USDT", Price: "60000", At: now}); err != nil {
		t.Fatal(err)
	}
	svc := oracletickgate.New(feed).WithClock(func() time.Time { return cur })
	ctx := context.Background()
	lease, err := svc.IssueTickLease(ctx, oracletickgate.IssueTickLeaseRequest{
		Asset:	"BTC", Quote: "USDT", Amount: "1", ValuationKind: oracletickgate.ValConvert, Mode: oracletickgate.ModeEnforce, Actor: "ops-stale",
	})
	if err != nil {
		t.Fatal(err)
	}
	cur = now.Add(31 * time.Second)
	rc := svc.RecheckTick(ctx, lease.LeaseID)
	if rc.OK != g.Expect.RecheckOK {
		t.Fatalf("recheck %+v", rc)
	}
	if rc.ProofKind != oracletickgate.ProofKind(g.Expect.ProofKind) && rc.ProofKind != oracletickgate.ProofLeaseExpired {
		t.Fatalf("kind %s", rc.ProofKind)
	}
	_, rec, err := svc.AssertBeforeFundWrite(ctx, oracletickgate.AssertFundWriteRequest{
		LeaseID:	lease.LeaseID, WriteKind: oracletickgate.WriteConvertPost, Actor: "ops-stale",
	})
	if err == nil || rec != nil {
		t.Fatalf("stale write rec=%+v", rec)
	}
}

func TestMissingPriceNoEnforceLease(t *testing.T) {
	var g struct {
		Expect struct {
			ProofKind	string	`json:"proof_kind"`
			FailClosed	bool	`json:"fail_closed"`
		} `json:"expect"`
	}
	loadGolden(t, "golden_missing_price.json", &g)
	now := time.Date(2021, 12, 18, 10, 0, 0, 0, time.UTC)
	feed := oracle.NewFeed(30 * time.Second).WithClock(func() time.Time { return now })
	svc := oracletickgate.New(feed).WithClock(func() time.Time { return now })
	ctx := context.Background()
	lease, err := svc.IssueTickLease(ctx, oracletickgate.IssueTickLeaseRequest{
		Asset:	"DOGE", Quote: "USDT", Amount: "1", ValuationKind: oracletickgate.ValConvert, Mode: oracletickgate.ModeEnforce, Actor: "ops-miss",
	})
	if lease != nil {
		t.Fatalf("lease issued %+v", lease)
	}
	if !errors.Is(err, oracletickgate.ErrMissingPriceIssue) {
		t.Fatalf("err=%v", err)
	}
	proofs,
		_ := svc.store.ListProofs(ctx, 20)
	if !oracletickgate.HasKind(proofs, oracletickgate.ProofKind(g.Expect.ProofKind)) {
		t.Fatalf("proofs %+v", proofs)
	}
}

func TestMarginLTVSameGeneration(t *testing.T) {
	var g struct {
		Expect struct {
			SameGeneration		bool	`json:"same_generation"`
			AfterPutMismatch	bool	`json:"after_put_mismatch"`
			ProofKind		string	`json:"proof_kind"`
		} `json:"expect"`
	}
	loadGolden(t, "golden_margin_ltv_generation.json", &g)
	now := time.Date(2021, 12, 18, 11, 0, 0, 0, time.UTC)
	feed := seedFeed(t, now, "60000")
	svc := oracletickgate.New(feed).WithClock(func() time.Time { return now })
	ctx := context.Background()
	lease, err := svc.IssueTickLease(ctx, oracletickgate.IssueTickLeaseRequest{
		Quote:	"USDT", ValuationKind: oracletickgate.ValLTV, Mode: oracletickgate.ModeEnforce, Actor: "ops-ltv",
		BorrowAsset:	"USDT", BorrowAmt: "1000", ColAsset: "BTC", ColAmt: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.BindMarginLTVGeneration(ctx, lease.LeaseID, lease.Generation); err != nil {
		t.Fatal(err)
	}
	if !g.Expect.SameGeneration {
		t.Fatal("golden")
	}
	if err := feed.Put(oracle.Tick{Asset: "BTC", Quote: "USDT", Price: "62000", At: now}); err != nil {
		t.Fatal(err)
	}
	err = svc.BindMarginLTVGeneration(ctx,
		lease.LeaseID, lease.Generation)
	if g.Expect.AfterPutMismatch && err == nil {
		t.Fatal("want generation mismatch")
	}
	if err != nil && !errors.Is(err, oracletickgate.ErrGeneration) {
		t.Fatalf("err=%v", err)
	}
	proofs, _ := svc.store.ListProofsByLease(ctx, lease.LeaseID)
	if !oracletickgate.HasKind(proofs, oracletickgate.ProofKind(g.Expect.ProofKind)) && !oracletickgate.HasKind(proofs, oracletickgate.ProofTickFlip) && !oracletickgate.HasKind(proofs, oracletickgate.ProofPriceDrift) {
		t.Fatalf("proofs %+v", proofs)
	}
}

func TestShadowCannotWrite(t *testing.T) {
	var g struct {
		Expect struct {
			ProofKind	string	`json:"proof_kind"`
			FailClosed	bool	`json:"fail_closed"`
			Posted		bool	`json:"posted"`
		} `json:"expect"`
	}
	loadGolden(t, "golden_shadow_impersonate.json", &g)
	now := time.Date(2021, 12, 18, 12, 0, 0, 0, time.UTC)
	feed := seedFeed(t, now, "60000")
	svc := oracletickgate.New(feed).WithClock(func() time.Time { return now })
	ctx := context.Background()
	lease, err := svc.ShadowValuation(ctx, oracletickgate.IssueTickLeaseRequest{
		Asset:	"BTC", Quote: "USDT", Amount: "1", ValuationKind: oracletickgate.ValConvert, Actor: "ops-shadow",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, rec, err := svc.AssertBeforeFundWrite(ctx, oracletickgate.AssertFundWriteRequest{
		LeaseID:	lease.LeaseID, WriteKind: oracletickgate.WriteConvertPost, Actor: "ops-shadow",
	})
	if !errors.Is(err, oracletickgate.ErrShadowImpersonate) {
		t.Fatalf("err=%v", err)
	}
	if rec != nil && rec.Posted != g.Expect.Posted {
		t.Fatal("posted")
	}
	proofs, _ := svc.store.ListProofsByLease(ctx, lease.LeaseID)
	if !oracletickgate.HasKind(proofs, oracletickgate.ProofKind(g.Expect.ProofKind)) {
		t.Fatalf("proofs %+v", proofs)
	}
}

func TestNoLeaseFailClosed(t *testing.T) {
	var g struct {
		Expect struct {
			ProofKind	string	`json:"proof_kind"`
			FailClosed	bool	`json:"fail_closed"`
		} `json:"expect"`
	}
	loadGolden(t, "golden_no_lease.json", &g)
	now := time.Date(2021, 12, 18, 13, 0, 0, 0, time.UTC)
	svc := oracletickgate.New(seedFeed(t, now, "60000")).WithClock(func() time.Time { return now })
	_, rec, err := svc.AssertBeforeFundWrite(context.Background(), oracletickgate.AssertFundWriteRequest{
		WriteKind:	oracletickgate.WriteConvertPost, Actor: "ops-none",
	})
	if !errors.Is(err, oracletickgate.ErrNoLease) || rec != nil {
		t.Fatalf("err=%v rec=%+v", err, rec)
	}
	proofs, _ := svc.store.ListProofs(context.Background(), 20)
	if !oracletickgate.HasKind(proofs, oracletickgate.ProofKind(g.Expect.ProofKind)) {
		t.Fatalf("proofs %+v", proofs)
	}
}

func TestHappyConvertConsumeOnce(t *testing.T) {
	now := time.Date(2021,
		12, 18, 14, 0, 0, 0, time.UTC)
	feed := seedFeed(t, now, "60000")
	svc := oracletickgate.New(feed).WithClock(func() time.Time { return now })
	ctx := context.Background()
	lease, err := svc.IssueTickLease(ctx, oracletickgate.IssueTickLeaseRequest{
		Asset:	"BTC", Quote: "USDT", Amount: "2", ValuationKind: oracletickgate.ValConvert, Mode: oracletickgate.ModeEnforce, Actor: "ops-ok", IdemKey: "idem-ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.IssueTickLease(ctx, oracletickgate.IssueTickLeaseRequest{
		Asset:	"BTC", Quote: "USDT", Amount: "2", ValuationKind: oracletickgate.ValConvert, Mode: oracletickgate.ModeEnforce, Actor: "ops-ok", IdemKey: "idem-ok",
	})
	if err != nil || again.LeaseID != lease.LeaseID {
		t.Fatalf("idem %+v %v", again, err)
	}
	got, rec, err := svc.AssertBeforeFundWrite(ctx, oracletickgate.AssertFundWriteRequest{
		LeaseID:	lease.LeaseID, WriteKind: oracletickgate.WriteConvertPost, Actor: "ops-ok", WriteRef: "post-1",
	})
	if err != nil || rec == nil || !rec.Posted || got.State != oracletickgate.LeaseConsumed {
		t.Fatalf("got=%+v rec=%+v err=%v", got, rec, err)
	}
	_, rec2, err := svc.AssertBeforeFundWrite(ctx, oracletickgate.AssertFundWriteRequest{
		LeaseID:	lease.LeaseID, WriteKind: oracletickgate.WriteConvertPost, Actor: "ops-ok",
	})
	if !errors.Is(err, oracletickgate.ErrLeaseConsumed) || rec2 != nil {
		t.Fatalf("double spend err=%v", err)
	}
}

func TestFSMEdges(t *testing.T) {
	if oracletickgate.CanTransit(oracletickgate.LeaseIssued, oracletickgate.LeaseBound) != nil {
		t.Fatal("issued→bound")
	}
	if oracletickgate.CanTransit(oracletickgate.LeaseIssued, oracletickgate.LeaseConsumed) == nil {
		t.Fatal("skip bound forbidden")
	}
	if oracletickgate.CanTransit(oracletickgate.LeaseConsumed, oracletickgate.LeaseIssued) == nil {
		t.Fatal("terminal")
	}
	if !oracletickgate.WriteAllowed(oracletickgate.ValMTM, oracletickgate.WriteCollateralMTM) || oracletickgate.WriteAllowed(oracletickgate.ValMTM, oracletickgate.WriteConvertPost) {
		t.Fatal("write allow")
	}
	if !oracletickgate.WriteAllowed(oracletickgate.ValLTV, oracletickgate.WriteLTVLiquidate) || !oracletickgate.WriteAllowed(oracletickgate.ValConvert, oracletickgate.WriteConvertPost) {
		t.Fatal("write allow 2")
	}
}

func TestWorkerExpire(t *testing.T) {
	now := time.Date(2021, 12, 18, 15, 0, 0, 0, time.UTC)
	cur := now
	feed := seedFeed(t, now, "60000")
	svc := oracletickgate.New(feed).WithClock(func() time.Time { return cur })
	ctx := context.Background()
	_, err := svc.IssueTickLease(ctx, oracletickgate.IssueTickLeaseRequest{
		Asset:	"BTC", Quote: "USDT", Amount: "1", ValuationKind: oracletickgate.ValConvert, Mode: oracletickgate.ModeEnforce, Actor: "ops-exp", TTLSeconds: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	cur = now.Add(2 * time.Second)
	w := oracletickgate.NewWorker(svc, oracletickgate.DefaultWorkerConfig())
	res := w.Tick(ctx)
	if res.ExpiredRevoked < 1 {
		t.Fatalf("expire %+v", res)
	}
}

func TestHooksAndMetrics(t *testing.T) {
	now := time.Date(2021, 12, 18, 16, 0, 0, 0, time.UTC)
	feed := seedFeed(t, now, "60000")
	svc := oracletickgate.New(feed).WithClock(func() time.Time { return now })
	ctx := context.Background()
	_, _, lease, err := svc.ConvertHook(ctx, "BTC", "1", "USDT", oracletickgate.ModeEnforce, "ops-h")
	if err != nil || lease == nil {
		t.Fatal(err)
	}
	_, lease2, err := svc.MarkToMarketHook(ctx, "USDT", map[string]string{"BTC": "1"}, oracletickgate.ModeEnforce, "ops-h")
	if err != nil || lease2 == nil {
		t.Fatal(err)
	}
	_, lease3, err := svc.MarginLTVHook(ctx, "USDT", "100", "BTC", "1", "USDT", oracletickgate.ModeEnforce, "ops-h")
	if err != nil || lease3 == nil {
		t.Fatal(err)
	}
	rec, err := svc.FundWriteHook(ctx, lease.LeaseID, oracletickgate.WriteConvertPost, "r1", "ops-h")
	if err != nil || rec == nil {
		t.Fatal(err)
	}
	var m oracletickgate.Metrics
	oracletickgate.ObserveLease(&m, lease)
	oracletickgate.ObserveProof(&m, oracletickgate.TickDriftProof{Kind: oracletickgate.ProofShadowImpersonate, FundWriteHeld: true})
	snap := m.Snapshot()
	if snap["shadow_blocked"] != 1 {
		t.Fatalf("%v", snap)
	}
	if oracletickgate.MustHaveEnforce(lease) != nil {
		t.Fatal("enforce")
	}
	if oracletickgate.MustHaveEnforce(&oracletickgate.TickLease{Mode: oracletickgate.ModeShadow}) != oracletickgate.ErrShadowImpersonate {
		t.Fatal("shadow")
	}
	if len(oracletickgate.SchemaStatements()) < 3 {
		t.Fatal("schema")
	}
	if oracletickgate.NewSQLStore("").Dialect() != "mysql" {
		t.Fatal("dialect")
	}
}

func TestPriceDriftBPS(t *testing.T) {
	if oracletickgate.PriceDriftBPS("100", "100.6") < 50 {
		t.Fatal("bps")
	}
	if !oracletickgate.DriftExceeded("100", "101", 50) {
		t.Fatal("exceed")
	}
	if oracletickgate.DriftExceeded("100", "100.1", 50) {
		t.Fatal("within")
	}
	if oracletickgate.FingerprintOf(oracle.Tick{Asset: "BTC", Quote: "USDT", Price: "1", At: time.Unix(1, 0)}) == "" {
		t.Fatal("fp")
	}
}

func TestTableDrivenIssueModes(t *testing.T) {
	now := time.Date(2021, 12, 18, 17, 0, 0, 0, time.UTC)
	cases := []struct {
		name	string
		mode	oracletickgate.LeaseMode
		kind	oracletickgate.ValuationKind
		ok	bool
	}{
		{"enforce_convert", oracletickgate.ModeEnforce, oracletickgate.ValConvert, true},
		{"probe_convert", oracletickgate.ModeProbe, oracletickgate.ValConvert, true},
		{"enforce_mtm", oracletickgate.ModeEnforce, oracletickgate.ValMTM, true},
		{"bad_kind", oracletickgate.ModeEnforce, "nope", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := oracletickgate.New(seedFeed(t, now, "60000")).WithClock(func() time.Time { return now })
			req := oracletickgate.IssueTickLeaseRequest{
				Asset:	"BTC", Quote: "USDT", Amount: "1", Amounts: map[string]string{"BTC": "1"},
				ValuationKind:	tc.kind, Mode: tc.mode, Actor: "ops-td",
			}
			_, err := svc.IssueTickLease(context.Background(), req)
			if tc.ok && err != nil {
				t.Fatal(err)
			}
			if !tc.ok && err == nil {
				t.Fatal("want err")
			}
		})
	}
}

func TestViewExportRestore(t *testing.T) {
	now := time.Date(2021, 12, 18, 18, 0, 0, 0, time.UTC)
	svc := oracletickgate.New(seedFeed(t, now, "60000")).WithClock(func() time.Time { return now })
	ctx := context.Background()
	lease, err := svc.IssueTickLease(ctx, oracletickgate.IssueTickLeaseRequest{
		Asset:	"BTC", Quote: "USDT", Amount: "1", ValuationKind: oracletickgate.ValConvert, Mode: oracletickgate.ModeEnforce, Actor: "ops-v",
	})
	if err != nil {
		t.Fatal(err)
	}
	v, err := svc.View(ctx, lease.LeaseID)
	if err != nil || v.Lease == nil {
		t.Fatal(err)
	}
	_, _ = svc.RevokeLease(ctx, oracletickgate.RevokeLeaseRequest{LeaseID: lease.LeaseID, Actor: "ops-v", Reason: "ops"})
	proofs,
		_ := svc.ListProofs(ctx, 10)
	_ = oracletickgate.SummarizeProofs(proofs, lease)
	_ = oracletickgate.FirstKind(proofs)
	_ = oracletickgate.CompactDetail("abc", 2)
	snap := svc.store.Snapshot()
	mem := oracletickgate.NewMemStore()
	mem.Restore(snap)
	if _, err := mem.GetLease(ctx, lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	g, err := svc.LiveGeneration(ctx, "BTC", "USDT")
	if err != nil || g.Generation == 0 {
		t.Fatal(err)
	}
}

func TestWriteKindMismatch(t *testing.T) {
	now := time.Date(2021, 12, 18, 19, 0, 0, 0, time.UTC)
	svc := oracletickgate.New(seedFeed(t, now, "60000")).WithClock(func() time.Time { return now })
	ctx := context.Background()
	lease, err := svc.IssueTickLease(ctx, oracletickgate.IssueTickLeaseRequest{
		Asset:	"BTC", Quote: "USDT", Amount: "1", ValuationKind: oracletickgate.ValConvert, Mode: oracletickgate.ModeEnforce, Actor: "ops-wk",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.AssertBeforeFundWrite(ctx, oracletickgate.AssertFundWriteRequest{
		LeaseID:	lease.LeaseID, WriteKind: oracletickgate.WriteLTVLiquidate, Actor: "ops-wk",
	})
	if !errors.Is(err, oracletickgate.ErrWriteKind) {
		t.Fatalf("err=%v", err)
	}
}

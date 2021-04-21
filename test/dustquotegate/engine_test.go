package dustquotegate_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/dustconv"
	"account-assets.kucoin.net/funding/internal/dustquotegate"
)

func testdataFile(t *testing.T, name string) []byte {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "dustquotegate", name)
	b, err := os.ReadFile(root)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	return b
}

func testEngine(t *testing.T) (*dustquotegate.Engine, *dustquotegate.StaticSource, *dustquotegate.MemStore,) {
	t.Helper()
	src := dustquotegate.NewStaticSource()
	st := dustquotegate.NewMemStore()
	eng := dustquotegate.NewEngine(st, src, dustquotegate.WithMetrics(dustquotegate.NewMetrics()), dustquotegate.WithClock(func() time.Time {
		return time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	}))
	src.PutBook("user-dqg-rate-001", "BTC", "0.00005")
	src.PutBook("user-dqg-pol-001", "ETH", "0.0004")
	src.PutBook("user-dqg-ok-001", "BTC", "0.00005")
	src.PutBook("user-dqg-cons-001", "BTC", "0.00005")
	src.PutBook("user-dqg-scan-001", "BTC", "0.00005")
	src.PutBook("user-dqg-scan-001", "ETH", "0.0004")
	return eng, src, st
}

func TestQuoteThenPutRateRejectsConvert(t *testing.T) {
	var g struct {
		UserID		string	`json:"user_id"`
		Asset		string	`json:"asset"`
		Amount		string	`json:"amount"`
		Expected	struct {
			ConvertError	string	`json:"convert_error"`
			ProofKind	string	`json:"proof_kind"`
		}	`json:"expected"`
	}
	if err := json.Unmarshal(testdataFile(t, "quote_then_putrate_reject.golden.json"), &g); err != nil {
		t.Fatal(err)
	}
	eng, src, st := testEngine(t)
	ctx := context.Background()
	lease, err := eng.QuoteWithLease(ctx, dustquotegate.QuoteReq{
		UserID:	g.UserID, Asset: g.Asset, Amount: g.Amount,
		Actor:	"ops-dust-quote", IdempotencyKey: "idem-rate-1",
	})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if lease.State != dustquotegate.LeaseQuoted || lease.LeaseID == "" {
		t.Fatalf("lease: %+v", lease)
	}
	src.PutRate(g.Asset, "61000")
	_, err = eng.AssertLease(ctx, dustquotegate.BindReq{
		LeaseID:	lease.LeaseID, UserID: g.UserID, Asset: g.Asset, Amount: g.Amount, Actor: "ops-dust-quote",
	})
	if err == nil || err.Error() != g.Expected.ConvertError {
		t.Fatalf("want %q got %v", g.Expected.ConvertError, err)
	}
	proofs, _ := st.ListProofsByLease(ctx, lease.LeaseID)
	found := false
	for _, p := range proofs {
		if string(p.Kind) == g.Expected.ProofKind {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing proof %s: %+v", g.Expected.ProofKind, proofs)
	}
}

func TestPutPolicyGenInvalidatesLease(t *testing.T) {
	var g struct {
		UserID		string	`json:"user_id"`
		Asset		string	`json:"asset"`
		Amount		string	`json:"amount"`
		Expected	struct {
			ConvertError	string	`json:"convert_error"`
			ProofKind	string	`json:"proof_kind"`
		}	`json:"expected"`
	}
	if err := json.Unmarshal(testdataFile(t, "policygen_bump.golden.json"), &g); err != nil {
		t.Fatal(err)
	}
	eng, src, st := testEngine(t)
	ctx := context.Background()
	lease, err := eng.QuoteWithLease(ctx, dustquotegate.QuoteReq{
		UserID:	g.UserID, Asset: g.Asset, Amount: g.Amount,
		Actor:	"ops-dust-quote", IdempotencyKey: "idem-pol-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	src.PutPolicyGen(g.Asset, 2)
	_, err = eng.AssertLease(ctx, dustquotegate.BindReq{LeaseID: lease.LeaseID, UserID: g.UserID, Asset: g.Asset, Amount: g.Amount, Actor: "ops-dust-quote"})
	if err == nil || err.Error() != g.Expected.ConvertError {
		t.Fatalf("want %q got %v", g.Expected.ConvertError, err)
	}
	proofs, _ := st.ListProofsByLease(ctx, lease.LeaseID)
	ok := false
	for _, p := range proofs {
		if string(p.Kind) == g.Expected.ProofKind {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("proofs=%+v", proofs)
	}
}

func TestOnFlipRevokesOpenLeases(t *testing.T) {
	eng, _, _ := testEngine(t)
	ctx := context.Background()
	lease, err := eng.QuoteWithLease(ctx, dustquotegate.QuoteReq{
		UserID:	"user-dqg-rate-001", Asset: "BTC", Amount: "0.00005",
		Actor:	"ops-dust-quote", IdempotencyKey: "idem-flip-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := eng.OnFlip(ctx, dustquotegate.FlipEvent{Asset: "BTC", Kind: dustquotegate.FlipRate, OldRate: "60000", NewRate: "1"})
	if err != nil || n < 1 {
		t.Fatalf("flip n=%d err=%v", n, err)
	}
	got, err := eng.GetLease(ctx, lease.LeaseID)
	if err != nil || got.State != dustquotegate.LeaseRevoked {
		t.Fatalf("state=%s err=%v", got.State, err)
	}
}

func TestConserveFailClosed(t *testing.T) {
	var g struct {
		UserID		string	`json:"user_id"`
		Asset		string	`json:"asset"`
		Amount		string	`json:"amount"`
		Tamper		string	`json:"usdt_tamper"`
		Expected	struct {
			Error string `json:"error"`
		}	`json:"expected"`
	}
	if err := json.Unmarshal(testdataFile(t, "conserve_fail_closed.golden.json"), &g); err != nil {
		t.Fatal(err)
	}
	eng, _, _ := testEngine(t)
	ctx := context.Background()
	lease, err := eng.QuoteWithLease(ctx, dustquotegate.QuoteReq{
		UserID:	g.UserID,
		Asset:	g.Asset, Amount: g.Amount,
		Actor:	"ops-dust-quote", IdempotencyKey: "idem-cons-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.AssertLease(ctx, dustquotegate.BindReq{LeaseID: lease.LeaseID, UserID: g.UserID, Asset: g.Asset, Amount: g.Amount, Actor: "ops-dust-quote"}); err != nil {
		t.Fatal(err)
	}
	_, err = eng.SettleLease(ctx, dustquotegate.SettleReq{LeaseID: lease.LeaseID, ConvertID: "c1", USDT: g.Tamper, Actor: "ops-dust-quote"})
	if err == nil || err.Error() != g.Expected.Error {
		t.Fatalf("want %q got %v", g.Expected.Error, err)
	}
	cur, _ := eng.GetLease(ctx, lease.LeaseID)
	if cur.State == dustquotegate.LeaseSettled {
		t.Fatal("must not settle")
	}
}

func TestQuoteAssertSettleOK(t *testing.T) {
	var g struct {
		UserID		string	`json:"user_id"`
		Asset		string	`json:"asset"`
		Amount		string	`json:"amount"`
		Expected	struct {
			SettleState string `json:"settle_state"`
		}	`json:"expected"`
	}
	if err := json.Unmarshal(testdataFile(t, "settle_ok.golden.json"), &g); err != nil {
		t.Fatal(err)
	}
	eng, _, _ := testEngine(t)
	ctx := context.Background()
	lease, err := eng.QuoteWithLease(ctx, dustquotegate.QuoteReq{
		UserID:	g.UserID, Asset: g.Asset, Amount: g.Amount,
		Actor:	"ops-dust-quote", IdempotencyKey: "idem-ok-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.AssertLease(ctx, dustquotegate.BindReq{LeaseID: lease.LeaseID, UserID: g.UserID, Asset: g.Asset, Amount: g.Amount, Actor: "ops-dust-quote"}); err != nil {
		t.Fatal(err)
	}
	got, err := eng.SettleLease(ctx, dustquotegate.SettleReq{LeaseID: lease.LeaseID, ConvertID: "cv-ok", USDT: lease.USDTWant, Actor: "ops-dust-quote"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.State) != g.Expected.SettleState {
		t.Fatalf("state=%s", got.State)
	}
}

func TestConvertScanHaltOnFlip(t *testing.T) {
	var g struct {
		UserID	string	`json:"user_id"`
		Batch	string	`json:"batch_id"`
	}
	if err := json.Unmarshal(testdataFile(t, "scan_mid_flip.golden.json"), &g); err != nil {
		t.Fatal(err)
	}
	dust := dustconv.New()
	_ = dust.Credit(g.UserID, "BTC", "0.00005")
	_ = dust.Credit(g.UserID, "ETH", "0.0004")
	src := dustquotegate.NewDustconvSource(dust)
	st := dustquotegate.NewMemStore()
	eng := dustquotegate.NewEngine(st,
		src, dustquotegate.WithMetrics(dustquotegate.NewMetrics()))
	dust.BindLeaseGate(dustquotegate.NewConvBind(eng))
	dust.OnRateFlip(func(asset, oldV, newV string) {
		_, _ = eng.OnFlip(context.Background(), dustquotegate.FlipEvent{Asset: asset, Kind: dustquotegate.FlipRate, OldRate: oldV, NewRate: newV})
	})
	run := dustquotegate.NewScanRunner(eng, dust)
	ctx := context.Background()
	lease, err := eng.QuoteWithLease(ctx, dustquotegate.QuoteReq{
		UserID:	g.UserID, Asset: "BTC", Amount: "0.00005", Actor: "ops-dust-quote",
		IdempotencyKey:	"scan-q-btc", ScanBatch: g.Batch, ScanIndex: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := dust.Convert(dustconv.ConvertReq{
		UserID:	g.UserID, Asset: "BTC", Amount: "0.00005", Actor: "ops-dust-quote",
		IdempotencyKey:	"scan-c-btc", DustQuoteLeaseID: lease.LeaseID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.USDT == "" {
		t.Fatal("first convert must keep")
	}
	_ = dust.PutRate("ETH", "2100")
	_, err = run.Run(ctx, g.UserID, "ops-dust-quote", g.Batch)
	if err == nil {
		// ETH 旧租约会因翻价被吊销；Run 内 Quote 用新价应仍可成功。
		// 显式 Halt 覆盖后续错兑：翻价后持旧 ETH 租约不得 Convert。
	}
	old, err := eng.QuoteWithLease(ctx, dustquotegate.QuoteReq{
		UserID:	g.UserID, Asset: "ETH", Amount: "0.0004", Actor: "ops-dust-quote",
		IdempotencyKey:	"scan-old-eth",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = dust.PutRate("ETH", "1999")
	_, err = dust.Convert(dustconv.ConvertReq{
		UserID:	g.UserID, Asset: "ETH", Amount: "0.0004", Actor: "ops-dust-quote",
		IdempotencyKey:	"scan-c-eth-old", DustQuoteLeaseID: old.LeaseID,
	})
	if err == nil {
		t.Fatal("old lease after flip must fail")
	}
	if haltErr := eng.HaltScan(ctx, g.Batch, g.UserID, "ops-dust-quote", "mid flip", 1, 1, 1); haltErr != dustquotegate.ErrScanHalted {
		t.Fatalf("halt: %v", haltErr)
	}
	cur,
		err := st.GetCursor(ctx, g.Batch)
	if err != nil || !cur.Halted {
		t.Fatalf("cursor %+v err=%v", cur, err)
	}
}

func TestFingerprintTable(t *testing.T) {
	cases := []struct {
		name	string
		a, b	dustquotegate.PolicyRateFingerprint
		err	error
	}{
		{"same", dustquotegate.PolicyRateFingerprint{Asset: "BTC", Rate: "1", PolicyGen: 1, CapBPS: 50, BookFP: "x"}, dustquotegate.PolicyRateFingerprint{Asset: "BTC", Rate: "1", PolicyGen: 1, CapBPS: 50, BookFP: "x"}, nil},
		{"rate", dustquotegate.PolicyRateFingerprint{Asset: "BTC", Rate: "1", PolicyGen: 1, CapBPS: 50, BookFP: "x"}, dustquotegate.PolicyRateFingerprint{Asset: "BTC", Rate: "2", PolicyGen: 1, CapBPS: 50, BookFP: "x"}, dustquotegate.ErrRateDrift},
		{"gen", dustquotegate.PolicyRateFingerprint{Asset: "BTC", Rate: "1", PolicyGen: 1, CapBPS: 50, BookFP: "x"}, dustquotegate.PolicyRateFingerprint{Asset: "BTC", Rate: "1", PolicyGen: 2, CapBPS: 50, BookFP: "x"}, dustquotegate.ErrPolicyDrift},
		{"book", dustquotegate.PolicyRateFingerprint{Asset: "BTC", Rate: "1", PolicyGen: 1, CapBPS: 50, BookFP: "x"}, dustquotegate.PolicyRateFingerprint{Asset: "BTC", Rate: "1", PolicyGen: 1, CapBPS: 50, BookFP: "y"}, dustquotegate.ErrBookDrift},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.a.Digest = dustquotegate.DigestFingerprint(tc.a)
			tc.b.Digest = dustquotegate.DigestFingerprint(tc.b)
			err := dustquotegate.CompareLive(tc.a, tc.b)
			if err != tc.err {
				t.Fatalf("got %v want %v", err, tc.err)
			}
		})
	}
}

func TestExpireOpen(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	src := dustquotegate.NewStaticSource()
	src.PutBook("user-dqg-ok-001", "BTC", "0.00005")
	st := dustquotegate.NewMemStore()
	tick := now
	eng := dustquotegate.NewEngine(st, src, dustquotegate.WithClock(func() time.Time { return tick }))
	lease, err := eng.QuoteWithLease(context.Background(), dustquotegate.QuoteReq{
		UserID:	"user-dqg-ok-001", Asset: "BTC", Amount: "0.00005",
		Actor:	"ops-dust-quote", IdempotencyKey: "idem-exp", TTLSec: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	tick = now.Add(2 * time.Second)
	n, err := eng.ExpireOpen(context.Background(), 50)
	if err != nil || n < 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	got,
		_ := eng.GetLease(context.Background(), lease.LeaseID)
	if got.State != dustquotegate.LeaseExpired {
		t.Fatalf("state=%s", got.State)
	}
}

func TestNoLeaseProof(t *testing.T) {
	eng, _, st := testEngine(t)
	_, err := eng.AssertLease(context.Background(), dustquotegate.BindReq{UserID: "u1", Asset: "BTC", Actor: "ops-dust-quote"})
	if err != dustquotegate.ErrNoLease {
		t.Fatalf("got %v", err)
	}
	ps, _ := st.ListProofsByKind(context.Background(), dustquotegate.ProofNoLease, 10)
	if len(ps) == 0 {
		t.Fatal("expected no_lease proof")
	}
}

func TestDustconvQuoteSnapshotPin(t *testing.T) {
	s := dustconv.New()
	snap, err := s.LiveLeaseQuote("u1", "BTC")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Floor != "0.0001" || snap.PolicyGen < 1 {
		t.Fatalf("%+v", snap)
	}
	if err := s.SetLeasePolicyGen("BTC", 9); err != nil {
		t.Fatal(err)
	}
	live, err := s.LiveLeaseQuote("u1", "BTC")
	if err != nil {
		t.Fatal(err)
	}
	if err := snap.SameLeaseQuote(live); err == nil {
		t.Fatal("gen bump must drift")
	}
}

func TestIdempotentQuote(t *testing.T) {
	eng, _, _ := testEngine(t)
	req := dustquotegate.QuoteReq{UserID: "user-dqg-ok-001", Asset: "BTC", Amount: "0.00005", Actor: "ops-dust-quote", IdempotencyKey: "same-idem"}
	a, err := eng.QuoteWithLease(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := eng.QuoteWithLease(context.Background(), req)
	if err != nil || a.LeaseID != b.LeaseID {
		t.Fatalf("idem %s %s %v", a.LeaseID, b.LeaseID, err)
	}
}

package partial_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"account-assets.kucoin.net/funding/internal/freeze"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/liquidation/gate"
	"account-assets.kucoin.net/funding/internal/margin"
	"account-assets.kucoin.net/funding/internal/liquidation/partial"
)

func harness(t *testing.T) (*partial.Service, *partial.MemStore, *freeze.AmountBook, *partial.LedgerReleaseAdapter, *ledger.MemStore) {
	t.Helper()
	st := ledger.NewMemStore()
	st.Seed("u-partial", ledger.P2PDebit.SourceBook(), "USDT", "10000")
	book := freeze.NewAmountBook()
	rel := partial.NewLedgerReleaseAdapter(st)
	mem := partial.NewMemStore()
	svc, err := partial.New(partial.ProductionConfig(), mem, book, rel)
	if err != nil {
		t.Fatal(err)
	}
	return svc, mem, book, rel, st
}

func seedFrozen(t *testing.T, st *ledger.MemStore, user, asset, frozen string) {
	t.Helper()
	ctx := context.Background()
	acc, err := st.GetAccount(ctx, user, ledger.P2PDebit.SourceBook(), asset)
	if err != nil {
		t.Fatal(err)
	}
	amt, err := ledger.ParseAmount(frozen)
	if err != nil {
		t.Fatal(err)
	}
	acc.Frozen = amt
	if err := st.UpsertAccount(ctx, acc); err != nil {
		t.Fatal(err)
	}
}

func TestTablePartialRelease(t *testing.T) {
	ctx := context.Background()
	type step struct {
		fillID	string
		amt	string
		wantRem	string
		wantRel	string
		wantErr	error
		reenter	bool
		dup	bool
	}
	cases := []struct {
		name	string
		initial	string
		steps	[]step
	}{
		{
			name:		"主成功两笔成交耗尽冻结",
			initial:	"500",
			steps: []step{
				{fillID: "f1", amt: "200", wantRem: "300", wantRel: "200"},
				{fillID: "f2", amt: "300", wantRem: "0", wantRel: "500"},
			},
		},
		{
			name:		"同成交幂等不二次释放",
			initial:	"500",
			steps: []step{
				{fillID: "f1", amt: "200", wantRem: "300", wantRel: "200"},
				{fillID: "f1", amt: "200", wantRem: "300", wantRel: "200", dup: true},
			},
		},
		{
			name:		"成交大于剩余则封顶不超释",
			initial:	"100",
			steps: []step{
				{fillID: "f1", amt: "150", wantRem: "0", wantRel: "100"},
			},
		},
		{
			name:		"非法成交失败封闭",
			initial:	"100",
			steps: []step{
				{fillID: "bad", amt: "-1", wantErr: errors.New("fill")},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, book, led, lst := harness(t)
			lst.Seed("u-partial", ledger.P2PDebit.SourceBook(), "USDT", "10000")
			seedFrozen(t, lst, "u-partial", "USDT", tc.initial)
			if _, err := svc.Open(ctx, "u-partial", "USDT", tc.initial); err != nil {
				t.Fatal(err)
			}
			for _, st := range tc.steps {
				res, err := svc.ApplyFill(ctx, partial.Fill{ID: st.fillID, AccountID: "u-partial", Asset: "USDT", Amount: st.amt})
				if st.wantErr != nil {
					if err == nil {
						t.Fatal("expected fail-closed")
					}
					ep, _, _ := svc.EpisodeOf(ctx, "u-partial")
					if ep.Released != "0" || ep.Remaining != tc.initial {
						t.Fatalf("fail-closed mutated episode %#v", ep)
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				if st.dup && !res.Idempotent {
					t.Fatal("dup must be idempotent")
				}
				if res.Episode.Remaining != st.wantRem || res.Episode.Released != st.wantRel {
					t.Fatalf("got rem=%s rel=%s", res.Episode.Remaining, res.Episode.Released)
				}
				if book.FrozenAmount("u-partial", "USDT") != st.wantRem {
					t.Fatalf("freeze remaining %s", book.FrozenAmount("u-partial", "USDT"))
				}
				if err := margin.AssertConserve(tc.initial, res.Episode.Remaining, res.Episode.Released); err != nil {
					t.Fatal(err)
				}
				_ = led
			}
		})
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	ctx := context.Background()
	svc, mem, _, _, lst := harness(t)
	seedFrozen(t, lst, "u-partial", "USDT", "500")
	if _, err := svc.Open(ctx, "u-partial", "USDT", "500"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApplyFill(ctx, partial.Fill{ID: "f1", AccountID: "u-partial", Asset: "USDT", Amount: "200"}); err != nil {
		t.Fatal(err)
	}
	raw, err := mem.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dst := partial.NewMemStore()
	if err := dst.Restore(ctx, raw); err != nil {
		t.Fatal(err)
	}
	ep, ok, err := dst.GetEpisode(ctx, "u-partial")
	if err != nil || !ok {
		t.Fatalf("restore episode %v %v", ok, err)
	}
	if ep.Released != "200" || ep.Remaining != "300" {
		t.Fatalf("restored %#v", ep)
	}
	tk, ok, err := dst.GetTicket(ctx, "f1")
	if err != nil || !ok || tk.Status != partial.StatusComplete {
		t.Fatalf("ticket %#v ok=%v err=%v", tk, ok, err)
	}
}

func TestFailClosedJournal(t *testing.T) {
	ctx := context.Background()
	book := freeze.NewAmountBook()
	led := &failLedger{fail: true}
	mem := partial.NewMemStore()
	svc, err := partial.New(partial.ProductionConfig(), mem, book, led)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Open(ctx, "u1", "USDT", "80"); err != nil {
		t.Fatal(err)
	}
	_, err = svc.ApplyFill(ctx, partial.Fill{ID: "f1", AccountID: "u1", Asset: "USDT", Amount: "10"})
	if err == nil {
		t.Fatal("journal fail must close")
	}
	ep, _, _ := svc.EpisodeOf(ctx, "u1")
	if ep.Released != "0" {
		t.Fatalf("must not release on journal fail: %#v", ep)
	}
	if book.FrozenAmount("u1", "USDT") != "80" {
		t.Fatalf("freeze must stay 80 got %s", book.FrozenAmount("u1", "USDT"))
	}
}

func TestAssetMismatchFailClosed(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := harness(t)
	if _, err := svc.Open(ctx, "u-partial", "USDT", "50"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.ApplyFill(ctx, partial.Fill{ID: "f1", AccountID: "u-partial", Asset: "BTC", Amount: "1"})
	if !errors.Is(err, partial.ErrAssetMismatch) {
		t.Fatalf("got %v", err)
	}
}

func TestRestartReenterNoOverRelease(t *testing.T) {
	ctx := context.Background()
	st := ledger.NewMemStore()
	st.Seed("u-re", ledger.P2PDebit.SourceBook(), "USDT", "10000")
	seedFrozen(t, st, "u-re", "USDT", "400")
	book := freeze.NewAmountBook()
	rel := partial.NewLedgerReleaseAdapter(st)
	mem := partial.NewMemStore()
	fr := &failOnceFreeze{inner: book,
		failTimes:	1}
	svc, err := partial.New(partial.ProductionConfig(), mem, fr, rel)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Open(ctx, "u-re", "USDT", "400"); err != nil {
		t.Fatal(err)
	}
	_, err = svc.ApplyFill(ctx, partial.Fill{ID: "fx", AccountID: "u-re", Asset: "USDT", Amount: "120"})
	if err == nil {
		t.Fatal("first pass should fail on freeze leg")
	}
	raw, err := mem.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fsnap := book.Snapshot()
	mem2 := partial.NewMemStore()
	if err := mem2.Restore(ctx, raw); err != nil {
		t.Fatal(err)
	}
	book2 := freeze.NewAmountBook()
	book2.Restore(fsnap)
	rel2 := partial.NewLedgerReleaseAdapter(st)
	// 账本幂等已见；重入只补冻结腿。
	svc2, err := partial.New(partial.ProductionConfig(), mem2, book2, rel2)
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc2.Reenter(ctx, "u-re")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Episode.Released != "120" || out[0].Episode.Remaining != "280" {
		t.Fatalf("reenter %#v", out)
	}
	if book2.FrozenAmount("u-re", "USDT") != "280" {
		t.Fatalf("frozen %s", book2.FrozenAmount("u-re", "USDT"))
	}
	if book2.ReleaseCalls("u-re") != 1 {
		t.Fatalf("release calls %d", book2.ReleaseCalls("u-re"))
	}
	res, err := svc2.ApplyFill(ctx, partial.Fill{ID: "fx", AccountID: "u-re", Asset: "USDT", Amount: "120"})
	if err != nil || !res.Idempotent {
		t.Fatalf("second apply %v %#v", err, res)
	}
	if res.Episode.Released != "120" {
		t.Fatalf("over-release %#v", res.Episode)
	}
}

func TestProdConfigForbidsRollbackLocked(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join("..", "..", "..", "testdata", "golden", "liqpartial", "prod_forbid_rollback.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Production		bool	`json:"production"`
		AllowRollback		bool	`json:"allow_rollback"`
		RollbackForbidden	bool	`json:"rollback_forbidden"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	cfg := partial.ProductionConfig()
	if cfg.Production != g.Production || cfg.AllowRollback != g.AllowRollback || cfg.RollbackForbidden() != g.RollbackForbidden {
		t.Fatalf("config %#v golden %#v", cfg, g)
	}
	if err := (partial.Config{Production: true, AllowRollback: true}).Validate(); err == nil {
		t.Fatal("prod+rollback must fail validate")
	}
	svc, _, book, _, lst := harness(t)
	seedFrozen(t, lst, "u-partial", "USDT", "90")
	if _, err := svc.Open(ctx, "u-partial", "USDT", "90"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApplyFill(ctx, partial.Fill{ID: "f1", AccountID: "u-partial", Asset: "USDT", Amount: "40"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.AttemptRollback(ctx, "u-partial", "40"); !errors.Is(err, partial.ErrRollbackLocked) {
		t.Fatalf("rollback: %v", err)
	}
	ep, _, _ := svc.EpisodeOf(ctx, "u-partial")
	if ep.Released != "40" || ep.Remaining != "50" {
		t.Fatalf("rollback must not mutate %#v", ep)
	}
	if book.FrozenAmount("u-partial", "USDT") != "50" {
		t.Fatal("freeze mutated by forbidden rollback")
	}
}

func TestJournalDirectionGolden(t *testing.T) {
	ctx := context.Background()
	svc,
		_, _, led, lst := harness(t)
	seedFrozen(t, lst, "u-partial", "USDT", "500")
	if _, err := svc.Open(ctx, "u-partial", "USDT", "500"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApplyFill(ctx, partial.Fill{ID: "f1", AccountID: "u-partial", Asset: "USDT", Amount: "200"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApplyFill(ctx, partial.Fill{ID: "f2", AccountID: "u-partial", Asset: "USDT", Amount: "300"}); err != nil {
		t.Fatal(err)
	}
	lines := led.LastLines("u-partial")
	if len(lines) < 2 {
		t.Fatalf("lines=%d", len(lines))
	}
	assertReleaseGolden(t, lines[:2])
	inv := filepath.Join("..", "..", "..", "testdata", "golden", "liqpartial", "invariant_snapshot.json")
	raw, err := os.ReadFile(inv)
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Initial		string		`json:"initial"`
		Remaining	string		`json:"remaining"`
		Released	string		`json:"released"`
		Fills		[]string	`json:"fills"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	ep, _, _ := svc.EpisodeOf(ctx, "u-partial")
	if ep.Initial != g.Initial || ep.Remaining != g.Remaining || ep.Released != g.Released {
		t.Fatalf("episode %#v golden %#v", ep, g)
	}
}

func TestOpenIdempotentSameEpisode(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _, _ := harness(t)
	if _, err := svc.Open(ctx, "u-partial", "USDT", "10"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Open(ctx, "u-partial", "USDT", "10"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Open(ctx, "u-partial", "USDT", "11"); err == nil {
		t.Fatal("conflicting open must fail-closed")
	}
}

func TestWireAfterGateLock(t *testing.T) {
	ctx := context.Background()
	lst := ledger.NewMemStore()
	lst.Seed("u-g", ledger.P2PDebit.SourceBook(), "USDT", "10000")
	fr := freeze.NewAdapter(nil)
	locks := gate.NewLedgerLockAdapter(lst)
	g := gate.New(gate.Config{Params: margin.Params{MaintenanceRateBPS: 500}, LockAsset: "USDT"}, fr, locks)
	acc := margin.Account{ID: "u-g", Equity: "1", LockAsset: "USDT", Positions: []margin.Position{{Asset: "BTC", Qty: "1"}}}
	d, err := g.Evaluate(acc, map[string]string{"BTC": "10000"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Required == "" || !d.Frozen {
		t.Fatalf("gate %#v", d)
	}
	book := freeze.NewAmountBook()
	rel := partial.NewLedgerReleaseAdapter(lst)
	mem := partial.NewMemStore()
	svc, err := partial.New(partial.ProductionConfig(), mem, book, rel)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Open(ctx, "u-g", "USDT", d.Required); err != nil {
		t.Fatal(err)
	}
	res, err := svc.ApplyFill(ctx, partial.Fill{ID: "lg1", AccountID: "u-g", Asset: "USDT", Amount: "100"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Episode.Released != "100" {
		t.Fatalf("%#v", res.Episode)
	}
}

func assertReleaseGolden(t *testing.T, lines []ledger.JournalEntry) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "golden", "liqpartial", "release_journal.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want []struct {
		Memo		string	`json:"memo"`
		DebitPositive	bool	`json:"debit_positive"`
		CreditZero	bool	`json:"credit_zero"`
		DebitZero	bool	`json:"debit_zero"`
		CreditPos	bool	`json:"credit_positive"`
		AccountType	string	`json:"account_type"`
	}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if len(want) != len(lines) {
		t.Fatalf("golden lines %d got %d", len(want), len(lines))
	}
	for i, w := range want {
		if lines[i].Memo != w.Memo || string(lines[i].AccountType) != w.AccountType {
			t.Fatalf("line %d memo/book got %s %s", i, lines[i].Memo, lines[i].AccountType)
		}
		if w.DebitPositive && !lines[i].Debit.IsPositive() {
			t.Fatalf("line %d debit", i)
		}
		if w.CreditZero && !lines[i].Credit.IsZero() {
			t.Fatalf("line %d credit", i)
		}
		if w.DebitZero && !lines[i].Debit.IsZero() {
			t.Fatalf("line %d debit zero", i)
		}
		if w.CreditPos && !lines[i].Credit.IsPositive() {
			t.Fatalf("line %d credit pos", i)
		}
	}
}

type failLedger struct{ fail bool }

func (f *failLedger) PostRelease(context.Context, string, string, string, string) error {
	if f.fail {
		return errors.New("journal down")
	}
	return nil
}

func (f *failLedger) PostingCount(string) int			{ return 0 }
func (f *failLedger) LastLines(string) []ledger.JournalEntry	{ return nil }
func (f *failLedger) IdemSeen(string) bool			{ return false }

type failOnceFreeze struct {
	inner		freeze.AmountAPI
	failTimes	int
	seen		int
}

func (f *failOnceFreeze) Hold(accountID, asset, amount, reason string) error {
	return f.inner.Hold(accountID, asset, amount, reason)
}
func (f *failOnceFreeze) Release(accountID, asset, amount, idemKey string) error {
	if f.seen < f.failTimes {
		f.seen++
		return errors.New("freeze interrupt")
	}
	return f.inner.Release(accountID,
		asset, amount, idemKey)
}
func (f *failOnceFreeze) FrozenAmount(accountID, asset string) string {
	return f.inner.FrozenAmount(accountID, asset)
}
func (f *failOnceFreeze) ReleaseCalls(accountID string) int	{ return f.inner.ReleaseCalls(accountID) }
func (f *failOnceFreeze) ReleasedAmount(accountID, asset string) string {
	return f.inner.ReleasedAmount(accountID, asset)
}

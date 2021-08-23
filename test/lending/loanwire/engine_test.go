package loanwire_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/lending/repay"
	"account-assets.kucoin.net/funding/internal/marginhook"
	"account-assets.kucoin.net/funding/internal/utabook"
	"account-assets.kucoin.net/funding/internal/lending/loanwire"
)

func TestCT01SinglePeriodPost(t *testing.T) {
	ctx := context.Background()
	for _, c := range loanwire.HourCases() {
		if c.ID != "C-T01" && c.ID != "C-T01b" && c.ID != "C-T01c" {
			continue
		}
		e := loanwire.NewEngine()
		e.OpenLoan(c.User, loanwire.DefaultRow(c), loanwire.DefaultTier(c))
		p := utabook.PeriodOf(utabook.LoanNoteID(c.User, c.Currency, c.Mode, c.Symbol), c.PeriodKind, c.AnchorMs)
		got, err := e.PostPeriod(ctx, c.User, c.Currency, c.Mode, c.Symbol, p, c.Factor)
		if err != nil {
			t.Fatalf("%s post: %v", c.ID, err)
		}
		if loanwire.MustDec(got.Interest).String() != loanwire.MustDec(c.WantInt).String() {
			t.Fatalf("%s interest got %s want %s", c.ID, got.Interest, c.WantInt)
		}
		if got.Zero || got.Existed || len(got.Lines) != 2 {
			t.Fatalf("%s flags zero=%v existed=%v lines=%d", c.ID, got.Zero, got.Existed, len(got.Lines))
		}
		if ledger.CountMatching(ledger.LoanSubjectFixture(), got.Lines, ledger.KindInterestPost, c.User) != ledger.InterestPostRuleCount() {
			t.Fatalf("%s subject mismatch %+v", c.ID, got.Lines)
		}
	}
}

func TestCT02IdempotentRepost(t *testing.T) {
	ctx := context.Background()
	c := loanwire.HourCases()[0]
	e := loanwire.NewEngine()
	e.OpenLoan(c.User, loanwire.DefaultRow(c), loanwire.DefaultTier(c))
	p := utabook.PeriodOf(utabook.LoanNoteID(c.User,
		c.Currency, c.Mode, c.Symbol), c.PeriodKind, c.AnchorMs)
	a, err := e.PostPeriod(ctx, c.User, c.Currency, c.Mode, c.Symbol, p, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.PostPeriod(ctx, c.User, c.Currency, c.Mode, c.Symbol, p, "")
	if err != nil {
		t.Fatal(err)
	}
	if !b.Existed || a.EntryID != b.EntryID {
		t.Fatalf("idempotent: first=%s second=%s existed=%v", a.EntryID, b.EntryID, b.Existed)
	}
	row := e.Liability(c.User,
		c.Currency, c.Mode, c.Symbol)
	if loanwire.MustDec(row.AccruedInterest).String() != loanwire.MustDec(c.WantInt).String() {
		t.Fatalf("double accrued %s", row.AccruedInterest)
	}
}

func TestCT03MultiPeriodSum(t *testing.T) {
	ctx := context.Background()
	c := loanwire.HourCases()[0]
	e := loanwire.NewEngine()
	e.OpenLoan(c.User, loanwire.DefaultRow(c), loanwire.DefaultTier(c))
	loan := utabook.LoanNoteID(c.User, c.Currency, c.Mode, c.Symbol)
	seq := utabook.SequenceFrom(loan, "HOUR", c.AnchorMs, 3)
	sum := loanwire.MustDec("0")
	prev := int64(0)
	for i, k := range seq {
		if i > 0 && k.PeriodStart != prev {
			t.Fatalf("period not monotonic %d -> %d", prev, k.PeriodStart)
		}
		if i > 0 && k.PeriodStart <= seq[i-1].PeriodStart {
			t.Fatalf("period key not increasing")
		}
		prev = k.PeriodEnd
		r, err := e.PostPeriod(ctx, c.User, c.Currency, c.Mode, c.Symbol, k, "")
		if err != nil {
			t.Fatal(err)
		}
		sum = sum.Add(loanwire.MustDec(r.Interest))
	}
	want := loanwire.MustDec(c.WantInt).Mul(loanwire.MustDec("3"))
	if !sum.Equal(want) {
		t.Fatalf("sum %s want %s", sum, want)
	}
}

func TestCT04ZeroPeriodMarked(t *testing.T) {
	ctx := context.Background()
	for _, c := range loanwire.HourCases() {
		if !c.Zero {
			continue
		}
		e := loanwire.NewEngine()
		e.OpenLoan(c.User, loanwire.DefaultRow(c), loanwire.DefaultTier(c))
		p := utabook.PeriodOf(utabook.LoanNoteID(c.User, c.Currency, c.Mode, c.Symbol), c.PeriodKind, c.AnchorMs)
		got, err := e.PostPeriod(ctx, c.User, c.Currency, c.Mode, c.Symbol, p, "")
		if err != nil {
			t.Fatalf("%s: %v", c.ID, err)
		}
		if !got.Zero || loanwire.MustDec(got.Interest).IsPositive() {
			t.Fatalf("%s want zero got %+v", c.ID, got)
		}
		if !e.Book.PeriodProcessed(p) {
			t.Fatalf("%s period not marked", c.ID)
		}
		if len(got.Lines) != 0 {
			t.Fatalf("%s unexpected lines %v", c.ID, got.Lines)
		}
		again, err := e.PostPeriod(ctx, c.User, c.Currency, c.Mode, c.Symbol, p, "")
		if err != nil || !again.Existed {
			t.Fatalf("%s zero idempotent %v %+v", c.ID, err, again)
		}
	}
}

func TestCT05RepayInterestOnly(t *testing.T) {
	ctx := context.Background()
	c := caseByID(t, "C-T05")
	e := primed(t, ctx, c)
	e.SeedCash(c.User, c.Currency, "10000")
	res, err := e.Offset(ctx, c.User, c.Currency, c.Mode, c.Symbol, c.Repay, "rp-"+c.ID, repay.PolicyReject)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Split.Interest.Equal(loanwire.MustDec(c.WantInt)) || !res.Split.Principal.IsZero() {
		t.Fatalf("split %+v", res.Split)
	}
	row := e.Liability(c.User, c.Currency, c.Mode, c.Symbol)
	if loanwire.MustDec(row.AccruedInterest).String() != loanwire.MustDec(c.WantAccr).String() || loanwire.MustDec(row.Principal).String() != loanwire.MustDec(c.WantPrin).String() {
		t.Fatalf("liab acc=%s prin=%s", row.AccruedInterest, row.Principal)
	}
}

func TestCT06RepayInterestAndPrincipal(t *testing.T) {
	ctx := context.Background()
	c := caseByID(t, "C-T06")
	e := primed(t, ctx, c)
	e.SeedCash(c.User, c.Currency, "10000")
	res, err := e.Offset(ctx, c.User, c.Currency, c.Mode, c.Symbol, c.Repay, "rp-"+c.ID, repay.PolicyReject)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Split.Interest.Equal(loanwire.MustDec("0.1")) || !res.Split.Principal.Equal(loanwire.MustDec("100")) {
		t.Fatalf("split %+v", res.Split)
	}
	row := e.Liability(c.User, c.Currency, c.Mode, c.Symbol)
	if loanwire.MustDec(row.Principal).String() != loanwire.MustDec(c.WantPrin).String() {
		t.Fatalf("prin %s", row.Principal)
	}
	want := ledger.RepayRuleCount(true,
		true)
	if ledger.CountMatching(ledger.LoanSubjectFixture(), res.Lines, ledger.KindRepayOffset, c.User) != want {
		t.Fatalf("subjects %d want %d lines=%+v", ledger.CountMatching(ledger.LoanSubjectFixture(), res.Lines, ledger.KindRepayOffset, c.User), want, res.Lines)
	}
}

func TestCT07ExcessRejectOrSurplus(t *testing.T) {
	ctx := context.Background()
	c := caseByID(t, "C-T07")
	e := primed(t, ctx, c)
	e.SeedCash(c.User, c.Currency, "10000")
	_, err := e.Offset(ctx, c.User, c.Currency, c.Mode, c.Symbol, c.Repay, "rp-"+c.ID, repay.PolicyReject)
	if err != repay.ErrExcess {
		t.Fatalf("want excess got %v", err)
	}
	cs := caseByID(t, "C-T07s")
	es := primed(t, ctx, cs)
	es.SeedCash(cs.User, cs.Currency, "10000")
	res, err := es.Offset(ctx, cs.User, cs.Currency, cs.Mode, cs.Symbol, cs.Repay, "rp-"+cs.ID, repay.PolicySurplus)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Split.Surplus.IsPositive() {
		t.Fatalf("want surplus %+v", res.Split)
	}
	row := es.Liability(cs.User, cs.Currency, cs.Mode, cs.Symbol)
	if loanwire.MustDec(row.Principal).IsPositive() || loanwire.MustDec(row.AccruedInterest).IsPositive() {
		t.Fatalf("cleared? %+v", row)
	}
}

func TestCT08PostRepayPostNewBase(t *testing.T) {
	ctx := context.Background()
	c := caseByID(t, "C-T08")
	e := primed(t, ctx, c)
	e.SeedCash(c.User, c.Currency, "10000")
	if _, err := e.Offset(ctx, c.User, c.Currency, c.Mode, c.Symbol, c.Repay, "rp-"+c.ID, repay.PolicyReject); err != nil {
		t.Fatal(err)
	}
	loan := utabook.LoanNoteID(c.User, c.Currency, c.Mode, c.Symbol)
	next := utabook.NextPeriod(utabook.PeriodOf(loan, c.PeriodKind, c.AnchorMs))
	got, err := e.PostPeriod(ctx, c.User, c.Currency, c.Mode, c.Symbol, next, "")
	if err != nil {
		t.Fatal(err)
	}
	// 新基数 800 × 0.0001 = 0.08
	if loanwire.MustDec(got.Interest).String() != loanwire.MustDec("0.08").String() {
		t.Fatalf("new base interest %s", got.Interest)
	}
}

func TestCT09MarginhookCalls(t *testing.T) {
	ctx := context.Background()
	c := caseByID(t, "C-T06")
	e := primed(t, ctx, c)
	e.SeedCash(c.User, c.Currency, "10000")
	if e.HookCount(marginhook.ChangeInterestPost) != 1 {
		t.Fatalf("hook after post %d", e.HookCount(marginhook.ChangeInterestPost))
	}
	if _, err := e.Offset(ctx, c.User, c.Currency, c.Mode, c.Symbol, c.Repay, "rp-"+c.ID, repay.PolicyReject); err != nil {
		t.Fatal(err)
	}
	if e.HookCount(marginhook.ChangeRepayOffset) != 1 {
		t.Fatalf("hook after repay %d", e.HookCount(marginhook.ChangeRepayOffset))
	}
	// 重复还款不二次通知
	if _, err := e.Offset(ctx, c.User, c.Currency, c.Mode, c.Symbol, c.Repay, "rp-"+c.ID, repay.PolicyReject); err != nil {
		t.Fatal(err)
	}
	if e.HookCount(marginhook.ChangeRepayOffset) != 1 {
		t.Fatalf("hook dup %d", e.HookCount(marginhook.ChangeRepayOffset))
	}
}

func TestCT10SubjectDirections(t *testing.T) {
	ctx := context.Background()
	c := caseByID(t, "C-T01")
	e := primed(t, ctx, c)
	p := utabook.PeriodOf(utabook.LoanNoteID(c.User, c.Currency, c.Mode, c.Symbol), c.PeriodKind, c.AnchorMs)
	got, _ := e.PostPeriod(ctx, c.User, c.Currency, c.Mode, c.Symbol, p, "")
	if ledger.CountMatching(ledger.LoanSubjectFixture(), got.Lines, ledger.KindInterestPost, c.User) != 2 {
		t.Fatalf("interest subjects %+v", got.Lines)
	}
	e.SeedCash(c.User, c.Currency, "10000")
	res, err := e.Offset(ctx, c.User, c.Currency, c.Mode, c.Symbol, "0.1", "rp-sub", repay.PolicyReject)
	if err != nil {
		t.Fatal(err)
	}
	if ledger.CountMatching(ledger.LoanSubjectFixture(), res.Lines, ledger.KindRepayOffset, c.User) != 2 {
		t.Fatalf("repay-interest-only subjects %+v", res.Lines)
	}
}

func primed(t *testing.T, ctx context.Context, c loanwire.Case,) *loanwire.Engine {
	t.Helper()
	e := loanwire.NewEngine()
	e.OpenLoan(c.User, loanwire.DefaultRow(c), loanwire.DefaultTier(c))
	p := utabook.PeriodOf(utabook.LoanNoteID(c.User, c.Currency, c.Mode, c.Symbol), c.PeriodKind, c.AnchorMs)
	if _, err := e.PostPeriod(ctx, c.User, c.Currency, c.Mode, c.Symbol, p, c.Factor); err != nil {
		t.Fatal(err)
	}
	return e
}

func caseByID(t *testing.T, id string) loanwire.Case {
	t.Helper()
	for _, c := range loanwire.AllCases() {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("missing case %s", id)
	return loanwire.Case{}
}

package loanwire_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/utabook"
	"account-assets.kucoin.net/funding/internal/lending/loanwire"
)

func TestExtraHourAndDay(t *testing.T) {
	ctx := context.Background()
	for _, c := range append(loanwire.ExtraHourCases(), loanwire.ExtraDayCases()...) {
		e := loanwire.NewEngine()
		e.OpenLoan(c.User, loanwire.DefaultRow(c), loanwire.DefaultTier(c))
		p := utabook.PeriodOf(utabook.LoanNoteID(c.User, c.Currency, c.Mode, c.Symbol), c.PeriodKind, c.AnchorMs)
		got, err := e.PostPeriod(ctx, c.User, c.Currency, c.Mode, c.Symbol, p, "")
		if err != nil {
			t.Fatalf("%s: %v", c.ID, err)
		}
		if loanwire.MustDec(got.Interest).String() != loanwire.MustDec(c.WantInt).String() {
			t.Fatalf("%s got %s want %s", c.ID, got.Interest, c.WantInt)
		}
		if len(got.Lines) != 2 {
			t.Fatalf("%s lines %d", c.ID, len(got.Lines))
		}
	}
}

func TestThreeCycleThenPartialRepay(t *testing.T) {
	ctx := context.Background()
	c := loanwire.ExtraHourCases()[1]
	e := loanwire.NewEngine()
	e.OpenLoan(c.User, loanwire.DefaultRow(c), loanwire.DefaultTier(c))
	e.SeedCash(c.User, c.Currency, "100000")
	loan := utabook.LoanNoteID(c.User, c.Currency, c.Mode, c.Symbol)
	seq := utabook.SequenceFrom(loan, c.PeriodKind, c.AnchorMs, 3)
	for _, k := range seq {
		if _, err := e.PostPeriod(ctx, c.User, c.Currency, c.Mode, c.Symbol, k, ""); err != nil {
			t.Fatal(err)
		}
	}
	row := e.Liability(c.User, c.Currency, c.Mode, c.Symbol)
	accr := loanwire.MustDec(row.AccruedInterest)
	want := loanwire.MustDec(c.WantInt).Mul(loanwire.MustDec("3"))
	if !accr.Equal(want) {
		t.Fatalf("accr %s want %s", accr, want)
	}
	_, err := e.Offset(ctx, c.User, c.Currency, c.Mode, c.Symbol, want.String(), "rp-x3", "")
	if err != nil {
		t.Fatal(err)
	}
	row = e.Liability(c.User, c.Currency, c.Mode, c.Symbol)
	if loanwire.MustDec(row.AccruedInterest).IsPositive() {
		t.Fatalf("interest left %s", row.AccruedInterest)
	}
	if loanwire.MustDec(row.Principal).String() != loanwire.MustDec(c.Principal).String() {
		t.Fatalf("prin changed %s", row.Principal)
	}
}

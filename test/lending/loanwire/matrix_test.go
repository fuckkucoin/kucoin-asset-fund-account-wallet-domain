package loanwire_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/utabook"
	"account-assets.kucoin.net/funding/internal/lending/loanwire"
)

func TestHourMatrixPosts(t *testing.T) {
	ctx := context.Background()
	for _, c := range loanwire.HourCases() {
		if c.Repay != "" || c.Zero {
			continue
		}
		e := loanwire.NewEngine()
		e.OpenLoan(c.User, loanwire.DefaultRow(c), loanwire.DefaultTier(c))
		p := utabook.PeriodOf(utabook.LoanNoteID(c.User, c.Currency, c.Mode, c.Symbol), c.PeriodKind, c.AnchorMs)
		got,
			err := e.PostPeriod(ctx, c.User, c.Currency, c.Mode, c.Symbol, p, c.Factor)
		if err != nil {
			t.Fatalf("%s: %v", c.ID, err)
		}
		if loanwire.MustDec(got.Interest).String() != loanwire.MustDec(c.WantInt).String() {
			t.Fatalf("%s got %s want %s", c.ID, got.Interest, c.WantInt)
		}
		if !e.Book.AccrualAgrees(c.User, c.Currency, c.Mode, c.Symbol) {
			t.Fatalf("%s book disagree", c.ID)
		}
	}
}

func TestDayMatrixPosts(t *testing.T) {
	ctx := context.Background()
	for _, c := range loanwire.DayCases() {
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
	}
}

func TestRepayIdempotent(t *testing.T) {
	ctx := context.Background()
	c := caseByID(t, "C-T06")
	e := primed(t, ctx, c)
	e.SeedCash(c.User, c.Currency, "10000")
	a, err := e.Offset(ctx, c.User, c.Currency, c.Mode, c.Symbol, c.Repay, "same", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.Offset(ctx, c.User, c.Currency, c.Mode, c.Symbol, c.Repay, "same", "")
	if err != nil || !b.Existed || a.EntryID != b.EntryID {
		t.Fatalf("repay idempotent %+v %+v %v", a, b, err)
	}
}

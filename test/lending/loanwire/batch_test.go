package loanwire_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/lending/accrual"
	"account-assets.kucoin.net/funding/internal/utabook"
)

func TestBatchSkipsProcessed(t *testing.T) {
	ctx := context.Background()
	c := caseByID(t, "C-T01")
	e := primed(t, ctx, c)
	loan := utabook.LoanNoteID(c.User, c.Currency, c.Mode, c.Symbol)
	from := utabook.PeriodOf(loan, "HOUR", c.AnchorMs).PeriodStart
	to := from + utabook.PeriodSpanMs("HOUR")*3
	p := e.Params(c.User, c.Currency, c.Mode, c.Symbol, "HOUR")
	res, err := e.Accrual.RunBatch(ctx, accrual.BatchReq{
		UserID:	c.User, Currency: c.Currency, MarginMode: c.Mode, Symbol: c.Symbol,
		Params:	p, FromMs: from, ToMs: to, Kind: "HOUR",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Posted+res.Zero+res.Skip < 1 {
		t.Fatalf("batch %+v", res)
	}
}

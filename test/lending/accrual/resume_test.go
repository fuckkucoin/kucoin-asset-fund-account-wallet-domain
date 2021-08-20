package accrual_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/marginhook"
	"account-assets.kucoin.net/funding/internal/utabook"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/lending/accrual"
)

func TestResumeFromLast(t *testing.T) {
	ctx := context.Background()
	book := utabook.New()
	book.PutRateTier(utabook.RateTier{Currency: "USDT", RateHourly: "0.0001", RateDaily: "0.0024"})
	book.UpsertLiability("u", utabook.LiabilityRow{Currency: "USDT", Principal: "1000"})
	st := ledger.NewMemStore()
	led := ledger.NewService(st, nil, ledger.MemLocker{}, nil, nil, zerolog.Nop(), config.TransferConfig{DisableDriftBlock: true})
	p := accrual.New(led, book, marginhook.New())
	loan := utabook.LoanNoteID("u", "USDT", "CROSS", "")
	k := utabook.PeriodOf(loan, "HOUR", 1_700_000_000_000)
	if _, err := p.Post(ctx, accrual.PostReq{UserID: "u", Currency: "USDT", Params: book.ParamsFromCatalog("USDT", "HOUR", loan, "CROSS", ""), Period: k}); err != nil {
		t.Fatal(err)
	}
	res, err := p.Resume(ctx, accrual.ResumeReq{
		UserID:	"u", Currency: "USDT", Params: book.ParamsFromCatalog("USDT", "HOUR", loan, "CROSS", ""),
		NowMs:	k.PeriodEnd + utabook.PeriodSpanMs("HOUR"), Lookback: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Posted < 1 {
		t.Fatalf("resume posted=%d skip=%d items=%d", res.Posted, res.Skip, len(res.Items))
	}
}

func TestPostDoesNotDoubleLiabilityWhenLedgerExists(t *testing.T) {
	ctx := context.Background()
	book := utabook.New()
	book.PutRateTier(utabook.RateTier{Currency: "USDT", RateHourly: "0.0001", RateDaily: "0.0024"})
	book.UpsertLiability("u", utabook.LiabilityRow{Currency: "USDT", Principal: "1000"})
	st := ledger.NewMemStore()
	led := ledger.NewService(st, nil, ledger.MemLocker{}, nil, nil, zerolog.Nop(), config.TransferConfig{DisableDriftBlock: true})
	p := accrual.New(led, book, marginhook.New())
	loan := utabook.LoanNoteID("u", "USDT", "CROSS", "")
	k := utabook.PeriodOf(loan, "HOUR", 1_700_000_000_000)
	params := book.ParamsFromCatalog("USDT", "HOUR", loan, "CROSS", "")
	if _, err := led.PostLoanJournal(ctx, ledger.LoanPostReq{
		UserID:	"u", Asset: "USDT", LoanID: loan, Kind: ledger.KindInterestPost,
		Amount:	decimal.RequireFromString("0.1"), PeriodStart: k.PeriodStart, PeriodEnd: k.PeriodEnd,
		IdempotencyKey:	accrual.IdemKey(k),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := p.Post(ctx, accrual.PostReq{UserID: "u", Currency: "USDT", Params: params, Period: k})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Existed {
		t.Fatalf("want ledger existed %+v", got)
	}
	row, _ := book.LiabilityLoan("u", "USDT", "CROSS", "")
	if row.AccruedInterest != "0" && row.AccruedInterest != "" {
		t.Fatalf("liability double-posted %q", row.AccruedInterest)
	}
}

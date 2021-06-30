package fxmark_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
	"account-assets.kucoin.net/funding/internal/quotemultigate"
	"account-assets.kucoin.net/funding/internal/fxmark"
)

func fxBundle(now time.Time, ra, rb string) quotemultigate.Bundle {
	mk := func(v, rate string) quotemultigate.Tick {
		return quotemultigate.Tick{
			Venue:	v, Pair: "BTC/USDT", Kind: quotemultigate.KindRate,
			Rate:	decimal.RequireFromString(rate), AsOf: now, TTL: quote.DefaultFXTTL,
			Source:	quote.SourceMarket,
		}
	}
	return quotemultigate.Bundle{Ticks: []quotemultigate.Tick{mk("mark_alpha", ra), mk("mark_beta", rb)}}
}

func TestConfirmConvertMultiOkReentry(t *testing.T) {
	now := fxNow()
	ld := &fxmark.RecConvertLedger{}
	f := fxmark.NewFresh(quote.FixedClock{T: now}, quote.DefaultFXTTL)
	p := quotemultigate.NewPanel(quotemultigate.DefaultPolicy(), quotemultigate.NewJournal())
	f.BindMulti(p)
	req := fxmark.ConvertReq{
		UserID:	"u1", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("2"),
		Idem:	"fx-1", Now: now, Multi: fxBundle(now, "64000", "64010"),
	}
	got,
		err := f.ConfirmConvert(context.Background(), ld, req)
	if err != nil || !got.Applied {
		t.Fatalf("%v %+v", err, got)
	}
	if ld.Count() != 1 {
		t.Fatal(ld.Count())
	}
	if _, err := f.ConfirmConvert(context.Background(), ld, req); err != nil {
		t.Fatal(err)
	}
	if ld.Count() != 1 {
		t.Fatal("reentry write")
	}
}

func TestConfirmConvertMultiDiverge(t *testing.T) {
	now := fxNow()
	ld := &fxmark.RecConvertLedger{}
	f := fxmark.NewFresh(quote.FixedClock{T: now},
		quote.DefaultFXTTL)
	_, err := f.ConfirmConvert(context.Background(), ld, fxmark.ConvertReq{
		UserID:	"u1", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("1"),
		Now:	now, Multi: fxBundle(now, "64000", "70000"),
	})
	if !errors.Is(err, fxmark.ErrQuoteDiverge) {
		t.Fatal(err)
	}
	if ld.Count() != 0 {
		t.Fatal("writes")
	}
}

func TestConfirmConvertProdNoBundle(t *testing.T) {
	now := fxNow()
	ld := &fxmark.RecConvertLedger{}
	f := fxmark.NewFresh(quote.FixedClock{T: now}, quote.DefaultFXTTL)
	f.BindMulti(quotemultigate.NewProdPanel())
	if err := f.Put(fxmark.Mark{From: "BTC", To: "USDT", Rate: decimal.RequireFromString("64000"), AsOf: now, TTL: quote.DefaultFXTTL, Source: quote.SourceMarket}); err != nil {
		t.Fatal(err)
	}
	_, err := f.ConfirmConvert(context.Background(), ld, fxmark.ConvertReq{
		UserID:	"u1", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("1"), Now: now,
	})
	if !errors.Is(err, fxmark.ErrQuoteTimeout) {
		t.Fatal(err)
	}
	if ld.Count() != 0 {
		t.Fatal("writes")
	}
}

func TestConfirmConvertMultiTimeoutStatus(t *testing.T) {
	now := fxNow()
	ld := &fxmark.RecConvertLedger{}
	f := fxmark.NewFresh(quote.FixedClock{T: now},
		quote.DefaultFXTTL)
	b := fxBundle(now, "64000", "64000")
	b.Ticks[1].Status = quotemultigate.StatusTimeout
	_, err := f.ConfirmConvert(context.Background(), ld, fxmark.ConvertReq{
		UserID:	"u", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("1"),
		Now:	now, Multi: b,
	})
	if !errors.Is(err, fxmark.ErrQuoteTimeout) {
		t.Fatal(err)
	}
	_ = time.Second
}

package fxmark_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
	"account-assets.kucoin.net/funding/internal/fxmark"
)

func TestFXStepExpire(t *testing.T) {
	asOf := fxNow()
	sc := &quote.StepClock{Base: asOf}
	f := fxmark.NewFresh(sc,
		quote.DefaultFXTTL)
	_ = f.Put(fxmark.Mark{From: "ETH", To: "USDT", Rate: decimal.RequireFromString("3200"), AsOf: asOf, TTL: quote.DefaultFXTTL, Source: quote.SourceMarket})
	ld := &fxmark.RecConvertLedger{}
	if _, err := f.ConfirmConvert(context.Background(), ld, fxmark.ConvertReq{
		UserID:	"u", From: "ETH", To: "USDT", Amount: decimal.RequireFromString("1"), Idem: "1",
	}); err != nil {
		t.Fatal(err)
	}
	sc.Advance(quote.DefaultFXTTL + time.Second)
	if _, err := f.ConfirmConvert(context.Background(), ld, fxmark.ConvertReq{
		UserID:	"u", From: "ETH", To: "USDT", Amount: decimal.RequireFromString("1"), Idem: "2",
	}); !errors.Is(err, fxmark.ErrQuoteStale) {
		t.Fatal(err)
	}
	if ld.Count() != 1 {
		t.Fatal(ld.Count())
	}
}

func TestFXDelete(t *testing.T) {
	now := fxNow()
	f := fxmark.NewFresh(quote.FixedClock{T: now}, quote.DefaultFXTTL)
	_ = f.Put(fxmark.Mark{From: "BTC", To: "USDT", Rate: decimal.RequireFromString("1"), AsOf: now, TTL: quote.DefaultFXTTL, Source: quote.SourceMarket})
	f.Delete("BTC", "USDT")
	if _, err := f.MarkOf("BTC", "USDT", now); !errors.Is(err, fxmark.ErrQuoteEmpty) {
		t.Fatal(err)
	}
}

func TestFXNilFresh(t *testing.T) {
	var f *fxmark.FreshBook
	if _, err := f.ConfirmConvert(context.Background(), nil, fxmark.ConvertReq{UserID: "u", Amount: decimal.RequireFromString("1")}); !errors.Is(err, fxmark.ErrQuoteEmpty) {
		t.Fatal(err)
	}
	if _, err := f.MarkOf("A", "B", fxNow()); !errors.Is(err, fxmark.ErrQuoteEmpty) {
		t.Fatal(err)
	}
}

func TestFXBadAmount(t *testing.T) {
	now := fxNow()
	f := fxmark.NewFresh(quote.FixedClock{T: now}, quote.DefaultFXTTL)
	if _, err := f.ConfirmConvert(context.Background(), &fxmark.RecConvertLedger{}, fxmark.ConvertReq{UserID: "", Amount: decimal.RequireFromString("1")}); err != fxmark.ErrAmount {
		t.Fatal(err)
	}
}

package fxmark_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
	"account-assets.kucoin.net/funding/internal/fxmark"
)

func testdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "quoterefuse", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func fxNow() time.Time	{ return time.Date(2022, 5, 18, 9, 0, 0, 0, time.UTC) }

func TestG_T04_EmptyMark(t *testing.T) {
	raw := testdata(t, "golden_gt04_fx_empty.json")
	var doc struct {
		WantErr string `json:"want_err"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	ld := &fxmark.RecConvertLedger{}
	f := fxmark.NewFresh(quote.FixedClock{T: fxNow()}, quote.DefaultFXTTL)
	_, err := f.ConfirmConvert(context.Background(), ld, fxmark.ConvertReq{
		UserID:	"u1", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("1"), Now: fxNow(),
	})
	if err == nil || err.Error() != doc.WantErr {
		t.Fatalf("%v", err)
	}
	if ld.Count() != 0 {
		t.Fatal("writes")
	}
}

func TestG_T05_StaleMark(t *testing.T) {
	raw := testdata(t, "golden_gt05_fx_stale.json")
	var doc struct {
		Rate	string	`json:"rate"`
		AgeNS	int64	`json:"age_ns"`
		TTLNS	int64	`json:"ttl_ns"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	asOf := fxNow()
	now := asOf.Add(time.Duration(doc.AgeNS))
	ld := &fxmark.RecConvertLedger{}
	f := fxmark.NewFresh(quote.FixedClock{T: now},
		time.Duration(doc.TTLNS))
	if err := f.Put(fxmark.Mark{From: "BTC", To: "USDT", Rate: decimal.RequireFromString(doc.Rate), AsOf: asOf, TTL: time.Duration(doc.TTLNS), Source: quote.SourceMarket}); err != nil {
		t.Fatal(err)
	}
	_, err := f.ConfirmConvert(context.Background(), ld, fxmark.ConvertReq{
		UserID:	"u1", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("1"), Now: now,
	})
	if !errors.Is(err, fxmark.ErrQuoteStale) {
		t.Fatalf("%v", err)
	}
	if ld.Count() != 0 {
		t.Fatal("writes")
	}
}

func TestG_T12_FXIsolation(t *testing.T) {
	now := fxNow()
	f := fxmark.NewFresh(quote.FixedClock{T: now}, quote.DefaultFXTTL)
	if err := f.Put(fxmark.Mark{From: "ETH", To: "USDT", Rate: decimal.RequireFromString("3200"), AsOf: now, TTL: quote.DefaultFXTTL, Source: quote.SourceMarket}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.MarkOf("SOL", "USDT", now); !errors.Is(err, fxmark.ErrQuoteEmpty) {
		t.Fatal(err)
	}
	if _, err := f.RateFresh("ETH", "USDT", now); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmConvertFresh(t *testing.T) {
	now := fxNow()
	ld := &fxmark.RecConvertLedger{}
	f := fxmark.NewFresh(quote.FixedClock{T: now}, quote.DefaultFXTTL)
	_ = f.Put(fxmark.Mark{From: "BTC", To: "USDT", Rate: decimal.RequireFromString("64000"), AsOf: now, TTL: quote.DefaultFXTTL, Source: quote.SourceMarket})
	got, err := f.ConfirmConvert(context.Background(), ld, fxmark.ConvertReq{
		UserID:	"u1", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("0.5"), Idem: "c1",
	})
	if err != nil || !got.Applied || ld.Count() != 1 {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestFXHouseDebug(t *testing.T) {
	now := fxNow()
	f := fxmark.NewFresh(quote.FixedClock{T: now}, quote.DefaultFXTTL)
	_ = f.Put(fxmark.HouseDebugMark("BTC", "USDT", decimal.RequireFromString("1"), now))
	ld := &fxmark.RecConvertLedger{}
	_, err := f.ConfirmConvert(context.Background(), ld, fxmark.ConvertReq{
		UserID:	"u", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("1"),
	})
	if !errors.Is(err, quote.ErrHouseOnConfirm) {
		t.Fatal(err)
	}
}

func TestFXIllegalRaw(t *testing.T) {
	now := fxNow()
	f := fxmark.NewFresh(quote.FixedClock{T: now},
		quote.DefaultFXTTL)
	_ = f.Put(fxmark.Mark{From: "BTC", To: "USDT", Rate: decimal.RequireFromString("1"), AsOf: now, TTL: quote.DefaultFXTTL, Source: quote.SourceMarket, Raw: "NaN"})
	if _, err := f.MarkOf("BTC", "USDT", now); !errors.Is(err, fxmark.ErrQuoteIllegal) {
		t.Fatal(err)
	}
}

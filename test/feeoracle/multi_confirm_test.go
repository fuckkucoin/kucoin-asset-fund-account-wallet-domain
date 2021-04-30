package feeoracle_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
	"account-assets.kucoin.net/funding/internal/quotemultigate"
	"account-assets.kucoin.net/funding/internal/feeoracle"
)

func feeMulti(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "quotemulti", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestConfirmWithdrawFeeMultiOk(t *testing.T) {
	raw := feeMulti(t, "golden_ok.json")
	var doc struct {
		Venues	[]string	`json:"venues"`
		Flat	string		`json:"flat"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	now := baseNow()
	var ticks []quotemultigate.Tick
	for _, v := range doc.Venues {
		ticks = append(ticks, quotemultigate.Tick{
			Venue:	v, Asset: "USDT", Kind: quotemultigate.KindFee,
			FeeAmount:	decimal.RequireFromString(doc.Flat),
			Rate:		decimal.RequireFromString(doc.Flat),
			AsOf:		now,
			TTL:		quote.DefaultFeeTTL, Source: quote.SourceMarket,
		})
	}
	ld := &feeoracle.RecLedger{}
	b := feeoracle.NewFeeBook(quote.FixedClock{T: now}, quote.DefaultFeeTTL, ld)
	p := quotemultigate.NewPanel(quotemultigate.DefaultPolicy(), quotemultigate.NewJournal())
	b.BindMulti(p)
	req := feeoracle.ConfirmReq{
		UserID:	"u1", Asset: "USDT", Chain: "ETH", Gross: decimal.RequireFromString("100"),
		IdemKey:	"fee-1", Now: now, Multi: quotemultigate.Bundle{Kind: quotemultigate.KindFee, Ticks: ticks},
	}
	got, err := b.ConfirmWithdrawFee(context.Background(), req)
	if err != nil || !got.Applied {
		t.Fatalf("%v %+v", err, got)
	}
	if ld.Count() != 1 {
		t.Fatal(ld.Count())
	}
	if _, err := b.ConfirmWithdrawFee(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if ld.Count() != 1 {
		t.Fatal("reentry")
	}
}

func TestConfirmWithdrawFeeMultiTimeout(t *testing.T) {
	now := baseNow()
	ld := &feeoracle.RecLedger{}
	bk := feeoracle.NewFeeBook(quote.FixedClock{T: now}, quote.DefaultFeeTTL, ld)
	ticks := []quotemultigate.Tick{
		{Venue: "fee_alpha", Asset: "USDT", Kind: quotemultigate.KindFee, FeeAmount: decimal.RequireFromString("1"), Rate: decimal.RequireFromString("1"), AsOf: now, TTL: quote.DefaultFeeTTL, Source: quote.SourceMarket, Status: quotemultigate.StatusTimeout},
		{Venue: "fee_beta", Asset: "USDT", Kind: quotemultigate.KindFee, FeeAmount: decimal.RequireFromString("1"), Rate: decimal.RequireFromString("1"), AsOf: now, TTL: quote.DefaultFeeTTL, Source: quote.SourceMarket},
	}
	_, err := bk.ConfirmWithdrawFee(context.Background(), feeoracle.ConfirmReq{
		UserID:	"u1", Asset: "USDT", Chain: "ETH", Gross: decimal.RequireFromString("100"),
		Now:	now, Multi: quotemultigate.Bundle{Ticks: ticks},
	})
	if !errors.Is(err, feeoracle.ErrQuoteTimeout) {
		t.Fatal(err)
	}
	if ld.Count() != 0 {
		t.Fatal("writes")
	}
}

func TestConfirmWithdrawFeeProdSilentSingle(t *testing.T) {
	now := baseNow()
	ld := &feeoracle.RecLedger{}
	bk := feeoracle.NewFeeBook(quote.FixedClock{T: now},
		quote.DefaultFeeTTL, ld)
	bk.BindMulti(quotemultigate.NewProdPanel())
	if err := bk.Put(feeoracle.FeeQuote{Asset: "USDT", Chain: "ETH", Flat: decimal.RequireFromString("1"), AsOf: now, TTL: quote.DefaultFeeTTL, Source: quote.SourceMarket}); err != nil {
		t.Fatal(err)
	}
	_, err := bk.ConfirmWithdrawFee(context.Background(), feeoracle.ConfirmReq{
		UserID:	"u1", Asset: "USDT", Chain: "ETH", Gross: decimal.RequireFromString("100"), Now: now,
	})
	if !errors.Is(err, feeoracle.ErrQuoteTimeout) {
		t.Fatal(err)
	}
	if ld.Count() != 0 {
		t.Fatal("writes")
	}
}

func TestConfirmWithdrawFeeDiverge(t *testing.T) {
	now := baseNow()
	ld := &feeoracle.RecLedger{}
	bk := feeoracle.NewFeeBook(quote.FixedClock{T: now}, quote.DefaultFeeTTL, ld)
	ticks := []quotemultigate.Tick{
		{Venue: "fee_alpha", Asset: "USDT", Kind: quotemultigate.KindFee, FeeAmount: decimal.RequireFromString("1"), Rate: decimal.RequireFromString("1"), AsOf: now, TTL: quote.DefaultFeeTTL, Source: quote.SourceMarket},
		{Venue: "fee_beta", Asset: "USDT", Kind: quotemultigate.KindFee, FeeAmount: decimal.RequireFromString("8"), Rate: decimal.RequireFromString("8"), AsOf: now, TTL: quote.DefaultFeeTTL, Source: quote.SourceMarket},
	}
	_, err := bk.ConfirmWithdrawFee(context.Background(), feeoracle.ConfirmReq{
		UserID:	"u1", Asset: "USDT", Gross: decimal.RequireFromString("100"),
		Now:	now,
		Multi:	quotemultigate.Bundle{Kind: quotemultigate.KindFee, Ticks: ticks},
	})
	if !errors.Is(err, feeoracle.ErrQuoteDiverge) {
		t.Fatal(err)
	}
	if ld.Count() != 0 {
		t.Fatal("writes")
	}
}

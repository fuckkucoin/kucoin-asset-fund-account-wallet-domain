package quote_test

import (
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
)

func TestG_T12_PairIsolation(t *testing.T) {
	asOf := time.Date(2022, 5, 6, 0, 0, 0, 0, time.UTC)
	clk := quote.FixedClock{T: asOf}
	b := quote.NewBook(quote.DefaultSwapTTL, clk)
	if err := b.Put("ETH/USDT", quote.Quote{
		Pair:	"ETH/USDT", Rate: decimal.RequireFromString("3200"),
		AsOf:	asOf, TTL: quote.DefaultSwapTTL, Source: quote.SourceMarket,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.GetGuarded("SOL/USDT", asOf); err != quote.ErrQuoteEmpty {
		t.Fatalf("empty pair: %v", err)
	}
	q, err := b.GetGuarded("ETH/USDT", asOf)
	if err != nil || !q.Rate.Equal(decimal.RequireFromString("3200")) {
		t.Fatalf("fresh pair %v %v", q, err)
	}
	if len(quote.PairIsolationKeys()) < 2 {
		t.Fatal("keys")
	}
}

func TestBookMissingAndDelete(t *testing.T) {
	b := quote.NewBook(quote.DefaultFeeTTL, quote.FixedClock{T: time.Date(2022, 5, 6, 1, 0, 0, 0, time.UTC)})
	if _, err := b.Get(""); err != quote.ErrPairRequired {
		t.Fatal(err)
	}
	if err := b.Put("", quote.Quote{}); err != quote.ErrPairRequired {
		t.Fatal(err)
	}
	_ = b.Put("BTC/USDT", quote.Quote{Pair: "BTC/USDT", Rate: decimal.RequireFromString("1"), AsOf: time.Date(2022, 5, 6, 1, 0, 0, 0, time.UTC), TTL: time.Second})
	if len(b.Keys()) != 1 {
		t.Fatal(b.Keys())
	}
	b.Delete("BTC/USDT")
	if _, err := b.Get("BTC/USDT"); err != quote.ErrQuoteEmpty {
		t.Fatal(err)
	}
}

func TestNilBook(t *testing.T) {
	var b *quote.Book
	if _, err := b.Get("X"); err != quote.ErrQuoteEmpty {
		t.Fatal(err)
	}
	b.Delete("X")
	if b.Keys() != nil {
		t.Fatal("keys")
	}
}

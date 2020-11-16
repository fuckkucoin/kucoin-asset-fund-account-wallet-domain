package swap_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
	"account-assets.kucoin.net/funding/internal/swap"
)

func TestConfirmExplicitStaleQuote(t *testing.T) {
	now := swNow()
	g := swap.NewGate(quote.FixedClock{T: now}, quote.DefaultSwapTTL, &swap.RecSwapLedger{})
	q := quote.Quote{
		Pair:	"BTC/USDT", Rate: decimal.RequireFromString("64000"),
		AsOf:	now.Add(-time.Hour), TTL: time.Second, Source: quote.SourceMarket,
	}
	_, err := g.ConfirmSwap(context.Background(), swap.ConfirmReq{
		UserID:	"u", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("1"), Quote: &q,
	})
	if !errors.Is(err, quote.ErrQuoteStale) {
		t.Fatal(err)
	}
}

func TestConfirmMissingLedger(t *testing.T) {
	now := swNow()
	g := swap.NewGate(quote.FixedClock{T: now}, quote.DefaultSwapTTL, nil)
	q := quote.Quote{Pair: "BTC/USDT", Rate: decimal.RequireFromString("1"), AsOf: now, TTL: time.Second, Source: quote.SourceMarket}
	_, err := g.ConfirmSwap(context.Background(), swap.ConfirmReq{
		UserID:	"u", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("1"), Quote: &q,
	})
	if !errors.Is(err, swap.ErrSwapLedger) {
		t.Fatal(err)
	}
}

func TestConfirmMissingClock(t *testing.T) {
	g := swap.NewGate(nil, quote.DefaultSwapTTL, &swap.RecSwapLedger{})
	_, err := g.ConfirmSwap(context.Background(), swap.ConfirmReq{
		UserID:	"u", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("1"),
	})
	if !errors.Is(err, quote.ErrClockRequired) {
		t.Fatal(err)
	}
}

func TestNilRecSwapLedger(t *testing.T) {
	var r *swap.RecSwapLedger
	if r.Count() != 0 {
		t.Fatal("count")
	}
	if err := r.WriteSwap(context.Background(), "", "", "", "", decimal.Zero, decimal.Zero); !errors.Is(err, swap.ErrSwapLedger) {
		t.Fatal(err)
	}
}

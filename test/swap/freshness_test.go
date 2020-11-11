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

func TestFreshPricerMissingInner(t *testing.T) {
	p := swap.FreshPricer{AsOf: swNow(),
		TTL:	time.Second}
	if _, ok := p.Rate("BTC", "USDT"); ok {
		t.Fatal("empty inner")
	}
}

func TestFreshPricerUninitializedTime(t *testing.T) {
	p := swap.FreshPricer{Inner: swap.DefaultPricer(), TTL: time.Second}
	if _, ok := p.Rate("BTC", "USDT"); ok {
		t.Fatal("asof zero")
	}
	p.AsOf = swNow()
	p.TTL = 0
	if _, ok := p.Rate("BTC", "USDT"); ok {
		t.Fatal("ttl zero")
	}
}

func TestGatePutRequiresPair(t *testing.T) {
	g := swap.NewGate(quote.FixedClock{T: swNow()}, quote.DefaultSwapTTL, &swap.RecSwapLedger{})
	if err := g.Put(quote.Quote{}); err != quote.ErrPairRequired {
		t.Fatal(err)
	}
	g.Delete("BTC/USDT")
}

func TestConfirmBadAmount(t *testing.T) {
	g := swap.NewGate(quote.FixedClock{T: swNow()}, quote.DefaultSwapTTL, &swap.RecSwapLedger{})
	if _, err := g.ConfirmSwap(context.Background(), swap.ConfirmReq{UserID: "u", Amount: decimal.Zero}); !errors.Is(err, quote.ErrQuoteIllegal) {
		t.Fatal(err)
	}
}

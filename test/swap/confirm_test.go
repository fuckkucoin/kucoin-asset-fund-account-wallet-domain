package swap_test

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
	"account-assets.kucoin.net/funding/internal/swap"
)

func testdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "quoterefuse", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func swNow() time.Time	{ return time.Date(2022, 5, 24, 8, 0, 0, 0, time.UTC) }

func TestG_T06_EmptySwap(t *testing.T) {
	raw := testdata(t, "golden_gt06_swap_empty.json")
	var doc struct {
		WantErr string `json:"want_err"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	ld := &swap.RecSwapLedger{}
	g := swap.NewGate(quote.FixedClock{T: swNow()}, quote.DefaultSwapTTL, ld)
	_, err := g.ConfirmSwap(context.Background(), swap.ConfirmReq{
		UserID:	"u1", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("1"),
	})
	if err == nil || err.Error() != doc.WantErr {
		t.Fatalf("%v", err)
	}
	if ld.Count() != 0 {
		t.Fatal("writes")
	}
}

func TestG_T07_StaleSwap(t *testing.T) {
	raw := testdata(t, "golden_gt07_swap_stale.json")
	var doc struct {
		Rate	string	`json:"rate"`
		AgeNS	int64	`json:"age_ns"`
		TTLNS	int64	`json:"ttl_ns"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	asOf := swNow()
	now := asOf.Add(time.Duration(doc.AgeNS))
	ld := &swap.RecSwapLedger{}
	g := swap.NewGate(quote.FixedClock{T: now}, time.Duration(doc.TTLNS), ld)
	q := quote.Quote{Pair: "BTC/USDT", Rate: decimal.RequireFromString(doc.Rate), AsOf: asOf, TTL: time.Duration(doc.TTLNS), Source: quote.SourceMarket}
	if err := g.Put(q); err != nil {
		t.Fatal(err)
	}
	_, err := g.ConfirmSwap(context.Background(), swap.ConfirmReq{
		UserID:	"u1", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("1"),
	})
	if !errors.Is(err, quote.ErrQuoteStale) {
		t.Fatalf("%v", err)
	}
	if ld.Count() != 0 {
		t.Fatal("writes")
	}
}

func TestG_T11_SwapBeforeAfter(t *testing.T) {
	asOf := swNow()
	sc := &quote.StepClock{Base: asOf}
	ld := &swap.RecSwapLedger{}
	g := swap.NewGate(sc, quote.DefaultSwapTTL, ld)
	_ = g.Put(quote.Quote{Pair: "BTC/USDT", Rate: decimal.RequireFromString("64000"), AsOf: asOf, TTL: quote.DefaultSwapTTL, Source: quote.SourceSpotTicker})
	if _, err := g.ConfirmSwap(context.Background(), swap.ConfirmReq{
		UserID:	"u1", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("0.1"), Idem: "1",
	}); err != nil {
		t.Fatal(err)
	}
	sc.Advance(quote.DefaultSwapTTL + time.Nanosecond)
	if _, err := g.ConfirmSwap(context.Background(), swap.ConfirmReq{
		UserID:	"u1", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("0.1"), Idem: "2",
	}); !errors.Is(err, quote.ErrQuoteStale) {
		t.Fatal(err)
	}
	if ld.Count() != 1 {
		t.Fatalf("writes %d", ld.Count())
	}
}

func TestG_T12_SwapIsolation(t *testing.T) {
	now := swNow()
	g := swap.NewGate(quote.FixedClock{T: now}, quote.DefaultSwapTTL, &swap.RecSwapLedger{})
	_ = g.Put(quote.Quote{Pair: "ETH/USDT", Rate: decimal.RequireFromString("3200"), AsOf: now, TTL: quote.DefaultSwapTTL, Source: quote.SourceMarket})
	_, err := g.ConfirmSwap(context.Background(), swap.ConfirmReq{
		UserID:	"u", From: "SOL", To: "USDT", Amount: decimal.RequireFromString("1"),
	})
	if !errors.Is(err, quote.ErrQuoteEmpty) {
		t.Fatal(err)
	}
	if _, err := g.ConfirmSwap(context.Background(), swap.ConfirmReq{
		UserID:	"u", From: "ETH", To: "USDT", Amount: decimal.RequireFromString("1"),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmExplicitQuote(t *testing.T) {
	now := swNow()
	ld := &swap.RecSwapLedger{}
	g := swap.NewGate(quote.FixedClock{T: now}, quote.DefaultSwapTTL, ld)
	q := quote.Quote{Pair: "BTC/USDT", Rate: decimal.RequireFromString("64000"), AsOf: now, TTL: quote.DefaultSwapTTL, Source: quote.SourceMarket}
	got, err := g.ConfirmSwap(context.Background(), swap.ConfirmReq{
		UserID:	"u", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("1"), Quote: &q,
	})
	if err != nil || !got.Applied {
		t.Fatal(err)
	}
}

func TestFreshPricerEmpty(t *testing.T) {
	p := swap.FreshPricer{Inner: swap.StaticPricer{Rates: map[string]decimal.Decimal{}}, AsOf: swNow(), TTL: time.Second}
	if _, err := p.Snapshot("BTC", "USDT"); !errors.Is(err, quote.ErrQuoteEmpty) {
		t.Fatal(err)
	}
	p.Inner = swap.DefaultPricer()
	q, err := p.SnapshotAt("BTC", "USDT", swNow())
	if err != nil || !q.Rate.IsPositive() {
		t.Fatal(err, q)
	}
	if _, err := p.SnapshotAt("BTC", "USDT", swNow().Add(2*time.Second)); !errors.Is(err, quote.ErrQuoteStale) {
		t.Fatal(err)
	}
}

func TestHouseDebugRateNotConfirm(t *testing.T) {
	now := swNow()
	q := swap.HouseDebugRate("BTC", "USDT", decimal.RequireFromString("1"), now)
	g := swap.NewGate(quote.FixedClock{T: now}, quote.DefaultSwapTTL, &swap.RecSwapLedger{})
	_, err := g.ConfirmSwap(context.Background(), swap.ConfirmReq{
		UserID:	"u", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("1"), Quote: &q,
	})
	if !errors.Is(err, quote.ErrHouseOnConfirm) {
		t.Fatal(err)
	}
}

func TestNilGate(t *testing.T) {
	var g *swap.Gate
	if _, err := g.ConfirmSwap(context.Background(), swap.ConfirmReq{UserID: "u", Amount: decimal.RequireFromString("1")}); !errors.Is(err, quote.ErrQuoteEmpty) {
		t.Fatal(err)
	}
}

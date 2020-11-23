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
	"account-assets.kucoin.net/funding/internal/quotemultigate"
	"account-assets.kucoin.net/funding/internal/swap"
)

func multiData(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "quotemulti", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func rateBundle(now time.Time, a, b string) quotemultigate.Bundle {
	mk := func(v, rate string) quotemultigate.Tick {
		return quotemultigate.Tick{
			Venue:	v, Pair: "BTC/USDT", Kind: quotemultigate.KindRate,
			Rate:	decimal.RequireFromString(rate), AsOf: now, TTL: quote.DefaultSwapTTL,
			Source:	quote.SourceMarket,
			Status:	quotemultigate.StatusOK,
		}
	}
	return quotemultigate.Bundle{Key: "BTC/USDT", Kind: quotemultigate.KindRate, Ticks: []quotemultigate.Tick{mk("spot_alpha", a), mk("spot_beta", b)}}
}

func TestConfirmSwapMultiOkAndReentry(t *testing.T) {
	now := swNow()
	ld := &swap.RecSwapLedger{}
	g := swap.NewGate(quote.FixedClock{T: now},
		quote.DefaultSwapTTL, ld)
	p := quotemultigate.NewPanel(quotemultigate.DefaultPolicy(), quotemultigate.NewJournal())
	g.BindMulti(p)
	req := swap.ConfirmReq{
		UserID:	"u1", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("1"),
		Idem:	"swap-1", Now: now, Multi: rateBundle(now, "64000", "64000"),
	}
	got, err := g.ConfirmSwap(context.Background(), req)
	if err != nil || !got.Applied {
		t.Fatalf("%v %+v", err, got)
	}
	if ld.Count() != 1 {
		t.Fatal(ld.Count())
	}
	got2, err := g.ConfirmSwap(context.Background(), req)
	if err != nil || !got2.Applied {
		t.Fatal(err)
	}
	if ld.Count() != 1 {
		t.Fatal("reentry wrote")
	}
}

func TestConfirmSwapMultiDivergeNoLedger(t *testing.T) {
	raw := multiData(t, "golden_diverge.json")
	var doc struct {
		Ticks	[]struct {
			Venue	string	`json:"venue"`
			Rate	string	`json:"rate"`
			Pair	string	`json:"pair"`
		}	`json:"ticks"`
		WantErr	string	`json:"want_err"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	now := swNow()
	var ticks []quotemultigate.Tick
	for _, x := range doc.Ticks {
		ticks = append(ticks, quotemultigate.Tick{
			Venue:	x.Venue,
			Pair:	x.Pair, Kind: quotemultigate.KindRate,
			Rate:	decimal.RequireFromString(x.Rate), AsOf: now, TTL: quote.DefaultSwapTTL,
			Source:	quote.SourceMarket,
		})
	}
	ld := &swap.RecSwapLedger{}
	g := swap.NewGate(quote.FixedClock{T: now},
		quote.DefaultSwapTTL, ld)
	_, err := g.ConfirmSwap(context.Background(), swap.ConfirmReq{
		UserID:	"u1", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("1"),
		Idem:	"swap-div", Now: now, Multi: quotemultigate.Bundle{Ticks: ticks},
	})
	if !errors.Is(err, quote.ErrQuoteDiverge) || err.Error() != doc.WantErr {
		t.Fatalf("%v", err)
	}
	if ld.Count() != 0 {
		t.Fatal("writes")
	}
}

func TestConfirmSwapProdSilentSingleLocked(t *testing.T) {
	now := swNow()
	ld := &swap.RecSwapLedger{}
	g := swap.NewGate(quote.FixedClock{T: now}, quote.DefaultSwapTTL, ld)
	g.BindMulti(quotemultigate.NewProdPanel())
	q := quote.Quote{Pair: "BTC/USDT", Rate: decimal.RequireFromString("64000"), AsOf: now, TTL: quote.DefaultSwapTTL, Source: quote.SourceMarket}
	if err := g.Put(q); err != nil {
		t.Fatal(err)
	}
	_, err := g.ConfirmSwap(context.Background(), swap.ConfirmReq{
		UserID:	"u1", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("1"),
		Now:	now, Quote: &q,
	})
	if !errors.Is(err, quote.ErrQuoteTimeout) {
		t.Fatalf("silent single must fail: %v", err)
	}
	if ld.Count() != 0 {
		t.Fatal("writes")
	}
	one := rateBundle(now, "64000", "64000")
	one.Ticks = one.Ticks[:1]
	_, err = g.ConfirmSwap(context.Background(), swap.ConfirmReq{
		UserID:	"u1", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("1"),
		Now:	now, Multi: one,
	})
	if !errors.Is(err, quote.ErrQuoteTimeout) {
		t.Fatalf("%v", err)
	}
}

func TestConfirmSwapMultiTimeoutElapsed(t *testing.T) {
	now := swNow()
	ld := &swap.RecSwapLedger{}
	g := swap.NewGate(quote.FixedClock{T: now}, quote.DefaultSwapTTL, ld)
	b := rateBundle(now, "64000", "64000")
	b.Elapsed = time.Second
	b.Deadline = 100 * time.Millisecond
	_, err := g.ConfirmSwap(context.Background(), swap.ConfirmReq{
		UserID:	"u", From: "BTC", To: "USDT", Amount: decimal.RequireFromString("1"),
		Multi:	b,
	})
	if !errors.Is(err, quote.ErrQuoteTimeout) {
		t.Fatal(err)
	}
	if ld.Count() != 0 {
		t.Fatal("writes")
	}
}

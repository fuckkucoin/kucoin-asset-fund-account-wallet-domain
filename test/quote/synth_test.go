package quote_test

import (
	"errors"
	"fmt"
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
)

func TestSynthWindowsAndPairs(t *testing.T) {
	origin := time.Date(2022, 7, 19, 0, 0, 0, 0, time.UTC)
	ttls := []time.Duration{time.Second, 5 * time.Second, quote.DefaultSwapTTL, quote.DefaultFeeTTL, quote.DefaultFXTTL}
	pairs := quote.PairIsolationKeys()
	for _, pair := range pairs {
		for _, ttl := range ttls {
			q := quote.Quote{Pair: pair, Rate: decimal.RequireFromString("1.25"), AsOf: origin, TTL: ttl, Source: quote.SourceMarket}
			if err := quote.GuardConfirm(q, origin); err != nil {
				t.Fatalf("%s ttl=%s %v", pair, ttl, err)
			}
			if err := quote.GuardConfirm(q, origin.Add(ttl)); err != nil {
				t.Fatalf("eq bound %s %s %v", pair, ttl, err)
			}
			if err := quote.GuardConfirm(q, origin.Add(ttl+time.Nanosecond)); !errors.Is(err, quote.ErrQuoteStale) {
				t.Fatalf("stale %s %s %v", pair, ttl, err)
			}
		}
	}
}

func TestSynthEmptyKeysDoNotBleed(t *testing.T) {
	origin := time.Date(2022, 7, 19, 1, 0, 0, 0, time.UTC)
	b := quote.NewBook(quote.DefaultSwapTTL, quote.FixedClock{T: origin})
	for i, pair := range quote.PairIsolationKeys() {
		if i%2 == 0 {
			continue
		}
		if err := b.Put(pair, quote.Quote{Pair: pair, Rate: decimal.NewFromInt(int64(i + 2)), AsOf: origin, TTL: quote.DefaultSwapTTL, Source: quote.SourceSpotTicker}); err != nil {
			t.Fatal(err)
		}
	}
	for i, pair := range quote.PairIsolationKeys() {
		_, err := b.GetGuarded(pair, origin)
	}
}

func TestSynthSources(t *testing.T) {
	origin := time.Date(2022, 7, 19, 2, 0, 0, 0, time.UTC)
	srcs := []string{quote.SourceMarket, quote.SourceSpotTicker, quote.SourceStatic, quote.SourceUnset,
	}
	for _, src := range srcs {
		q := quote.Quote{Pair: "AAA/BBB", Rate: decimal.RequireFromString("9"), AsOf: origin, TTL: time.Second, Source: src}
		if err := quote.GuardConfirm(q, origin); err != nil {
			t.Fatalf("%s %v", src, err)
		}
	}
	q := quote.HouseDebugMid("AAA/BBB", decimal.RequireFromString("9"), origin)
	if err := quote.GuardConfirm(q, origin); !errors.Is(err, quote.ErrHouseOnConfirm) {
		t.Fatal(err)
	}
}

func TestSynthIllegalFeeBPS(t *testing.T) {
	origin := time.Date(2022, 7, 19, 3, 0, 0, 0, time.UTC)
	q := quote.Quote{Asset: "USDT", FeeBPS: -1, AsOf: origin, TTL: time.Second}
	if quote.Classify(q, origin) != quote.ClassIllegal {
		t.Fatal(quote.Classify(q, origin))
	}
	q.FeeBPS = 0
	q.FeeAmount = decimal.RequireFromString("-0.1")
	if quote.Classify(q, origin) != quote.ClassIllegal {
		t.Fatal(quote.Classify(q, origin))
	}
}

func TestSynthBookClockFallback(t *testing.T) {
	origin := time.Date(2022, 7, 19, 4, 0, 0, 0, time.UTC)
	b := quote.NewBook(quote.DefaultSwapTTL, quote.FixedClock{T: origin})
	_ = b.Put("X/Y", quote.Quote{Pair: "X/Y", Rate: decimal.RequireFromString("1"), AsOf: origin, TTL: quote.DefaultSwapTTL, Source: quote.SourceMarket})
	if _, err := b.GetGuarded("X/Y", time.Time{}); err != nil {
		t.Fatal(err)
	}
}

func TestSynthAgeNegative(t *testing.T) {
	if quote.Age(time.Time{}, time.Now()) != -1 {
		t.Fatal("age")
	}
	if !quote.StaleByTTL(time.Time{}, time.Second, time.Date(2022, 7, 19, 5, 0, 0, 0, time.UTC)) {
		t.Fatal("stale missing asof")
	}
}

func TestSynthPairKey(t *testing.T) {
	if quote.PairKey(" btc ", " usdt ") != "BTC/USDT" {
		t.Fatal(quote.PairKey(" btc ", " usdt "))
	}
	if quote.PairKey("", "USDT") != "" {
		t.Fatal("empty")
	}
	if quote.AssetKey(" usdt ") != "USDT" {
		t.Fatal(quote.AssetKey(" usdt "))
	}
}

func TestSynthDigestClass(t *testing.T) {
	origin := time.Date(2022, 7, 19, 6, 0, 0, 0, time.UTC)
	q := quote.Quote{Pair: "P/Q", Rate: decimal.RequireFromString("1"), AsOf: origin, TTL: time.Second}
	d := q.Digest(origin.Add(2 * time.Second))
	if d.Class != quote.ClassStale.String() {
		t.Fatal(d.Class)
	}
}

func TestSynthManyRates(t *testing.T) {
	origin := time.Date(2022, 7, 19, 7, 0, 0, 0, time.UTC)
	for i := 1; i <= 32; i++ {
		q := quote.Quote{Pair: fmt.Sprintf("A%d/USDT", i), Rate: decimal.NewFromInt(int64(i)), AsOf: origin, TTL: time.Second, Source: quote.SourceMarket}
		if q.IsEmpty() || q.IsStale(origin) {
			t.Fatalf("%d", i)
		}
	}
}

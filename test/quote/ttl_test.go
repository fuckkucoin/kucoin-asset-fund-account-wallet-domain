package quote_test

import (
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
)

func TestDefaultBoundaryTableReplay(t *testing.T) {
	origin := time.Date(2022, 8, 23, 0, 0, 0, 0, time.UTC)
	for _, bc := range quote.DefaultBoundaryTable() {
		q := quote.Quote{Pair: "BTC/USDT", Rate: decimal.RequireFromString("64000"), TTL: bc.TTL, Source: quote.SourceMarket}
		now := origin
		if bc.Age < 0 {
			// AsOf 为零
		} else {
			q.AsOf = origin
			now = origin.Add(bc.Age)
		}
		got := quote.Classify(q, now).String()
		if got != bc.Want {
			t.Fatalf("%s got %s want %s (%s)", bc.Name, got, bc.Want, bc.Comment)
		}
	}
}

func TestNormalizeTTL(t *testing.T) {
	if quote.NormalizeTTL(0) != 0 || quote.NormalizeTTL(-time.Second) != 0 {
		t.Fatal("zero")
	}
	if quote.NormalizeTTL(quote.MinTTL) != quote.MinTTL {
		t.Fatal("min")
	}
	if quote.DefaultFeeTTL != 30*time.Second || quote.DefaultFXTTL != 15*time.Second || quote.DefaultSwapTTL != 15*time.Second {
		t.Fatal("defaults")
	}
}

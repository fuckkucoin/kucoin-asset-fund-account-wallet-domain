package quote_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
)

func TestPairsSeedIsolation(t *testing.T) {
	raw := testdata(t, "pairs_seed.json")
	var doc struct {
		Pairs	[]struct {
			Pair	string	`json:"pair"`
			Rate	string	`json:"rate"`
			Seed	bool	`json:"seed"`
		}	`json:"pairs"`
		ExpectEmpty	[]string	`json:"expect_empty"`
		ExpectOK	[]string	`json:"expect_ok"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	origin := time.Date(2022, 8, 9, 0, 0, 0, 0, time.UTC)
	b := quote.NewBook(quote.DefaultSwapTTL, quote.FixedClock{T: origin})
	for _, p := range doc.Pairs {
		if !p.Seed {
			continue
		}
		q := quote.Quote{Pair: p.Pair, Rate: decimal.RequireFromString(p.Rate), AsOf: origin, TTL: quote.DefaultSwapTTL, Source: quote.SourceMarket}
		if err := b.Put(p.Pair, q); err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range doc.ExpectEmpty {
		if _, err := b.GetGuarded(k, origin); !errors.Is(err, quote.ErrQuoteEmpty) {
			t.Fatalf("%s %v", k, err)
		}
	}
	for _, k := range doc.ExpectOK {
		if _, err := b.GetGuarded(k, origin); err != nil {
			t.Fatalf("%s %v", k, err)
		}
	}
}

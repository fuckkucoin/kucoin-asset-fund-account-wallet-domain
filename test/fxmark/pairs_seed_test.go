package fxmark_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
	"account-assets.kucoin.net/funding/internal/fxmark"
)

func TestFXPairsSeedFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "quoterefuse", "pairs_seed.json"))
	if err != nil {
		t.Fatal(err)
	}
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
	now := fxNow()
	f := fxmark.NewFresh(quote.FixedClock{T: now}, quote.DefaultFXTTL)
	for _, p := range doc.Pairs {
		if !p.Seed {
			continue
		}
		from, to := splitFXPair(p.Pair)
		if err := f.Put(fxmark.Mark{From: from, To: to, Rate: decimal.RequireFromString(p.Rate), AsOf: now, TTL: quote.DefaultFXTTL, Source: quote.SourceMarket}); err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range doc.ExpectEmpty {
		from, to := splitFXPair(k)
		if _, err := f.MarkOf(from, to, now); !errors.Is(err, fxmark.ErrQuoteEmpty) {
			t.Fatalf("%s %v", k, err)
		}
	}
	for _, k := range doc.ExpectOK {
		from, to := splitFXPair(k)
		if _, err := f.RateFresh(from, to, now); err != nil {
			t.Fatalf("%s %v", k, err)
		}
	}
}

func splitFXPair(pair string) (string, string) {
	i := strings.IndexByte(pair, '/')
	if i <= 0 {
		return pair, ""
	}
	return pair[:i], pair[i+1:]
}

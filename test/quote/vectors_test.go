package quote_test

import (
	"errors"
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
)

func TestIllegalLiteralTable(t *testing.T) {
	asOf := time.Date(2022, 6, 9, 0, 0, 0, 0, time.UTC)
	now := asOf
	rows := []struct {
		raw	string
		rate	string
		want	error
	}{
		{"NaN", "1", quote.ErrQuoteIllegal},
		{"nan", "1", quote.ErrQuoteIllegal},
		{"Inf", "1", quote.ErrQuoteIllegal},
		{"+Inf", "1", quote.ErrQuoteIllegal},
		{"-Inf", "1", quote.ErrQuoteIllegal},
		{"infinity", "1", quote.ErrQuoteIllegal},
		{"1e2", "1", quote.ErrQuoteIllegal},
		{"1E-3", "1", quote.ErrQuoteIllegal},
		{"abc", "1", quote.ErrQuoteIllegal},
		{"", "-2", quote.ErrQuoteIllegal},
		{"", "0", quote.ErrSourceMasked},
		{"", "3", nil},
	}
	for _, row := range rows {
		q := quote.Quote{Pair: "BTC/USDT", AsOf: asOf, TTL: time.Second, Source: quote.SourceMarket}
		if row.rate != "" {
			q.Rate = decimal.RequireFromString(row.rate)
		}
		q.RawRate = row.raw
		err := quote.GuardConfirm(q, now)
		if row.want == nil {
			if err != nil {
				t.Fatalf("raw=%q rate=%s %v", row.raw, row.rate, err)
			}
			continue
		}
		if !errors.Is(err, row.want) {
			t.Fatalf("raw=%q got %v want %v", row.raw, err, row.want)
		}
	}
}

func TestLevelTables(t *testing.T) {
	asOf := time.Date(2022, 6, 9, 1, 0, 0, 0, time.UTC)
	cases := []struct {
		name	string
		lv	[]quote.Level
		want	quote.Class
	}{
		{"one", []quote.Level{{Px: decimal.RequireFromString("2"), Sz: decimal.RequireFromString("3")}}, quote.ClassOK},
		{"zero_sz", []quote.Level{{Px: decimal.RequireFromString("2"), Sz: decimal.Zero}}, quote.ClassEmpty},
		{"neg_px", []quote.Level{{Px: decimal.RequireFromString("-1"), Sz: decimal.RequireFromString("1")}}, quote.ClassIllegal},
		{"mixed", []quote.Level{{Px: decimal.Zero, Sz: decimal.RequireFromString("1")}, {Px: decimal.RequireFromString("2"), Sz: decimal.RequireFromString("1")}}, quote.ClassOK},
	}
	for _, tc := range cases {
		q := quote.Quote{Pair: "A/B", AsOf: asOf, TTL: time.Second, Levels: tc.lv}
		if got := quote.Classify(q, asOf); got != tc.want {
			t.Fatalf("%s %s want %s", tc.name, got, tc.want)
		}
	}
}

func TestStepClockConfirmWindow(t *testing.T) {
	asOf := time.Date(2022, 6, 9, 2, 0, 0, 0, time.UTC)
	sc := &quote.StepClock{Base: asOf}
	q := quote.Quote{Pair: "BTC/USDT", Rate: decimal.RequireFromString("10"), AsOf: asOf, TTL: 2 * time.Second, Source: quote.SourceMarket}
	if err := quote.GuardConfirm(q, sc.Now()); err != nil {
		t.Fatal(err)
	}
	sc.Advance(2 * time.Second)
	if err := quote.GuardConfirm(q, sc.Now()); err != nil {
		t.Fatal(err)
	}
	sc.Advance(time.Nanosecond)
	if err := quote.GuardConfirm(q, sc.Now()); !errors.Is(err, quote.ErrQuoteStale) {
		t.Fatal(err)
	}
}

func TestClassString(t *testing.T) {
	if quote.Class(99).String() != "unknown" {
		t.Fatal("unknown")
	}
}

func TestJoinKeys(t *testing.T) {
	if quote.JoinKeys([]string{"b", "a"}) != "a,b" {
		t.Fatal(quote.JoinKeys([]string{"b", "a"}))
	}
	if quote.FormatDurationNS(time.Second) != "1000000000" {
		t.Fatal(quote.FormatDurationNS(time.Second))
	}
}

func TestFuncClock(t *testing.T) {
	ts := time.Date(2022, 6, 9, 3, 0, 0, 0, time.UTC)
	c := quote.FuncClock(func() time.Time { return ts })
	if !c.Now().Equal(ts) {
		t.Fatal(c.Now())
	}
	var n quote.FuncClock
	if !n.Now().IsZero() {
		t.Fatal("zero")
	}
}

func TestQuoteWithSourceClone(t *testing.T) {
	q := quote.Quote{Pair: "X/Y", Rate: decimal.RequireFromString("1"), AsOf: time.Date(2022, 6, 9, 4, 0, 0, 0, time.UTC), TTL: time.Second, Levels: []quote.Level{{Px: decimal.RequireFromString("1"), Sz: decimal.RequireFromString("1")}}}
	c := q.WithSource(quote.SourceMarket).Clone()
	if c.Source != quote.SourceMarket || len(c.Levels) != 1 {
		t.Fatal(c)
	}
}
// temporary guard removed after the production check

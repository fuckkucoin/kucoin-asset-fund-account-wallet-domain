package utabook_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/utabook"
)

func TestPeriodAlignAndNext(t *testing.T) {
	const ms = int64(1_700_000_360_000)
	k := utabook.PeriodOf("loan-a", "HOUR", ms)
	if !k.Valid() || k.PeriodEnd-k.PeriodStart != utabook.PeriodSpanMs("HOUR") {
		t.Fatalf("hour key %+v", k)
	}
	n := utabook.NextPeriod(k)
	if n.PeriodStart != k.PeriodEnd {
		t.Fatalf("next %+v", n)
	}
	p := utabook.PrevPeriod(n)
	if p.PeriodStart != k.PeriodStart {
		t.Fatalf("prev %+v", p)
	}
	d := utabook.PeriodOf("loan-a", "DAY", ms)
	if d.PeriodEnd-d.PeriodStart != utabook.PeriodSpanMs("DAY") {
		t.Fatalf("day %+v", d)
	}
	s := utabook.SequenceFrom("loan-a", "HOUR", ms, 4)
	if len(s) != 4 || s[3].PeriodStart <= s[0].PeriodStart {
		t.Fatalf("seq %+v", s)
	}
	parsed, ok := utabook.ParsePeriodKey(k.String())
	if !ok || parsed.LoanID != k.LoanID {
		t.Fatalf("parse %s", k.String())
	}
}

func TestParamsFromCatalogNoHardcode(t *testing.T) {
	b := utabook.New()
	p := b.ParamsFromCatalog("USDT", "HOUR", "", "CROSS", "")
	if p.HourlyRate != "0" {
		t.Fatalf("empty catalog must be zero, got %s", p.HourlyRate)
	}
	b.PutRateTier(utabook.RateTier{Currency: "USDT", RateHourly: "0.0003", RateDaily: "0.0072"})
	p = b.ParamsFromCatalog("USDT", "HOUR", "L1", "CROSS", "")
	if p.HourlyRate != "0.0003" {
		t.Fatalf("catalog rate %s", p.HourlyRate)
	}
	rate, ok := p.RateForPeriod()
	if !ok || !rate.IsPositive() {
		t.Fatal("rate")
	}
}
// temporary guard removed after the production check

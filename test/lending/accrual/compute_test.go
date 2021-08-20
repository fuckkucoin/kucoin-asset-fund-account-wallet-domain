package accrual_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/utabook"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/lending/accrual"
)

func TestComputeMatrix(t *testing.T) {
	rows := []struct {
		name	string
		prin	string
		hourly	string
		daily	string
		kind	string
		free	string
		factor	string
		want	string
		zero	bool
	}{
		{"hour", "1000", "0.0001", "0.0024", "HOUR", "0", "1", "0.1", false},
		{"day", "1000", "0.0001", "0.0024", "DAY", "0", "1", "2.4", false},
		{"free_cover", "10", "0.0001", "0.0024", "HOUR", "10", "1", "0", true},
		{"zero_rate", "1000", "0", "0", "HOUR", "0", "1", "0", true},
		{"factor2", "1000", "0.0001", "0.0024", "HOUR", "0", "2", "0.2", false},
		{"partial_free", "1000", "0.0001", "0.0024", "HOUR", "200", "1", "0.08", false},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			prin, _ := decimal.NewFromString(r.prin)
			fac, _ := decimal.NewFromString(r.factor)
			out := accrual.Compute(accrual.ComputeIn{
				Principal:	prin,
				Params: utabook.AccrualParams{
					Currency:	"USDT", HourlyRate: r.hourly, DailyRate: r.daily,
					PeriodKind:	r.kind, InterestFree: r.free,
				},
				Factor:	fac,
			})
			if out.Zero != r.zero {
				t.Fatalf("zero=%v want %v reason=%s", out.Zero, r.zero, out.Reason)
			}
			if !out.Interest.Equal(must(r.want)) {
				t.Fatalf("got %s want %s", out.Interest, r.want)
			}
		})
	}
}

func must(s string) decimal.Decimal {
	v, _ := decimal.NewFromString(s)
	return v
}

package quotemultigate_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
	"account-assets.kucoin.net/funding/internal/quotemultigate"
)

func testdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "quotemulti", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func gateNow() time.Time {
	return time.Date(2022, 8, 18, 10, 0, 0, 0, time.UTC)
}

func liveTick(venue, pair, rate string, now time.Time) quotemultigate.Tick {
	return quotemultigate.Tick{
		Venue:	venue,
		Pair:	pair,
		Kind:	quotemultigate.KindRate,
		Rate:	decimal.RequireFromString(rate),
		AsOf:	now,
		TTL:	quote.DefaultSwapTTL,
		Source:	quote.SourceMarket,
		Status:	quotemultigate.StatusOK,
	}
}

func feeTick(venue, asset, flat string, now time.Time) quotemultigate.Tick {
	return quotemultigate.Tick{
		Venue:		venue,
		Asset:		asset,
		Kind:		quotemultigate.KindFee,
		FeeAmount:	decimal.RequireFromString(flat),
		Rate:		decimal.RequireFromString(flat),
		AsOf:		now,
		TTL:		quote.DefaultFeeTTL,
		Source:		quote.SourceMarket,
		Status:		quotemultigate.StatusOK,
	}
}

func TestGuardTableFromGolden(t *testing.T) {
	now := gateNow()
	type row struct {
		name	string
		b	quotemultigate.Bundle
		pol	quotemultigate.Policy
		want	error
	}
	div := struct {
		Ticks	[]struct {
			Venue	string	`json:"venue"`
			Rate	string	`json:"rate"`
			Pair	string	`json:"pair"`
		}	`json:"ticks"`
		Want	string	`json:"want_err"`
	}{}
	if err := json.Unmarshal(testdata(t, "golden_diverge.json"), &div); err != nil {
		t.Fatal(err)
	}
	var divTicks []quotemultigate.Tick
	for _, x := range div.Ticks {
		divTicks = append(divTicks, liveTick(x.Venue, x.Pair, x.Rate, now))
	}
	to := struct {
		ElapsedMS	int	`json:"elapsed_ms"`
		DeadlineMS	int	`json:"deadline_ms"`
		Ticks		[]struct {
			Venue	string	`json:"venue"`
			Status	string	`json:"status"`
			Rate	string	`json:"rate"`
			Pair	string	`json:"pair"`
		}	`json:"ticks"`
	}{}
	if err := json.Unmarshal(testdata(t, "golden_timeout.json"), &to); err != nil {
		t.Fatal(err)
	}
	var toTicks []quotemultigate.Tick
	for _, x := range to.Ticks {
		tk := liveTick(x.Venue, x.Pair, x.Rate, now)
		tk.Status = x.Status
		toTicks = append(toTicks, tk)
	}
	silent := struct {
		Profile	string	`json:"profile"`
		Ticks	[]struct {
			Venue	string	`json:"venue"`
			Pair	string	`json:"pair"`
			Rate	string	`json:"rate"`
			Status	string	`json:"status"`
		}	`json:"ticks"`
	}{}
	if err := json.Unmarshal(testdata(t, "golden_silent_single.json"), &silent); err != nil {
		t.Fatal(err)
	}
	var one []quotemultigate.Tick
	for _, x := range silent.Ticks {
		tk := liveTick(x.Venue,
			x.Pair, x.Rate, now)
		tk.Status = x.Status
		one = append(one, tk)
	}
	rows := []row{
		{
			name:	"ok_two_source",
			b: quotemultigate.Bundle{Key: "BTC/USDT", Kind: quotemultigate.KindRate, Ticks: []quotemultigate.Tick{
				liveTick("spot_alpha", "BTC/USDT", "64000", now),
				liveTick("spot_beta", "BTC/USDT", "64000", now),
			}},
			pol:	quotemultigate.DefaultPolicy(),
		},
		{
			name:	"diverge",
			b:	quotemultigate.Bundle{Key: "BTC/USDT", Kind: quotemultigate.KindRate, Ticks: divTicks},
			pol:	quotemultigate.DefaultPolicy(),
			want:	quotemultigate.ErrQuoteDiverge,
		},
		{
			name:	"timeout_mark",
			b: quotemultigate.Bundle{
				Ticks:		toTicks,
				Elapsed:	time.Duration(to.ElapsedMS) * time.Millisecond,
				Deadline:	time.Duration(to.DeadlineMS) * time.Millisecond,
			},
			pol:	quotemultigate.DefaultPolicy(),
			want:	quotemultigate.ErrQuoteTimeout,
		},
		{
			name:	"prod_silent_single",
			b:	quotemultigate.Bundle{Ticks: one},
			pol:	quotemultigate.ProdPolicy(),
			want:	quotemultigate.ErrQuoteTimeout,
		},
		{
			name:	"missing_venue",
			b: quotemultigate.Bundle{Ticks: []quotemultigate.Tick{
				liveTick("spot_alpha", "BTC/USDT", "64000", now),
				{Venue: "spot_beta", Status: quotemultigate.StatusMissing, Pair: "BTC/USDT"},
			}},
			pol:	quotemultigate.DefaultPolicy(),
			want:	quotemultigate.ErrQuoteTimeout,
		},
		{
			name:	"empty_status",
			b: quotemultigate.Bundle{Ticks: []quotemultigate.Tick{
				{Venue: "spot_alpha", Status: quotemultigate.StatusEmpty, Pair: "BTC/USDT"},
				liveTick("spot_beta", "BTC/USDT", "64000", now),
			}},
			pol:	quotemultigate.DefaultPolicy(),
			want:	quotemultigate.ErrQuoteEmpty,
		},
		{
			name:	"kind_mismatch",
			b: quotemultigate.Bundle{Ticks: []quotemultigate.Tick{
				liveTick("spot_alpha", "BTC/USDT", "64000", now),
				feeTick("fee_alpha", "USDT", "1.2", now),
			}},
			pol:	quotemultigate.DefaultPolicy(),
			want:	quotemultigate.ErrQuoteDiverge,
		},
		{
			name:	"prod_no_bundle",
			b:	quotemultigate.Bundle{},
			pol:	quotemultigate.ProdPolicy(),
			want:	quotemultigate.ErrQuoteTimeout,
		},
		{
			name:	"zero_clock",
			b: quotemultigate.Bundle{Ticks: []quotemultigate.Tick{
				liveTick("a", "BTC/USDT", "1", now),
				liveTick("b", "BTC/USDT", "1", now),
			}},
			pol:	quotemultigate.DefaultPolicy(),
			want:	quotemultigate.ErrClockRequired,
		},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			nowIn := now
			if r.name == "zero_clock" {
				nowIn = time.Time{}
			}
			_, err := quotemultigate.Guard(r.pol, r.b, nowIn)
			if r.want == nil {
				if err != nil {
					t.Fatalf("err %v", err)
				}
				return
			}
			if !errors.Is(err, r.want) {
				t.Fatalf("got %v want %v", err, r.want)
			}
		})
	}
}

func TestGuardStaleFromGolden(t *testing.T) {
	raw := testdata(t, "golden_stale.json")
	var doc struct {
		AgeNS	int64	`json:"age_ns"`
		TTLNS	int64	`json:"ttl_ns"`
		Rate	string	`json:"rate"`
		Pair	string	`json:"pair"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	asOf := gateNow()
	now := asOf.Add(time.Duration(doc.AgeNS))
	tk := func(v string) quotemultigate.Tick {
		t0 := liveTick(v,
			doc.Pair, doc.Rate, asOf)
		t0.TTL = time.Duration(doc.TTLNS)
		t0.AsOf = asOf
		return t0
	}
	_, err := quotemultigate.Guard(quotemultigate.DefaultPolicy(), quotemultigate.Bundle{Ticks: []quotemultigate.Tick{tk("spot_alpha"), tk("spot_beta")}}, now)
	if !errors.Is(err, quotemultigate.ErrQuoteStale) {
		t.Fatalf("%v", err)
	}
}

func TestGuardFeeOkGolden(t *testing.T) {
	raw := testdata(t, "golden_ok.json")
	var doc struct {
		Venues	[]string	`json:"venues"`
		Flat	string		`json:"flat"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	now := gateNow()
	var ticks []quotemultigate.Tick
	for _, v := range doc.Venues {
		ticks = append(ticks, feeTick(v, "USDT", doc.Flat, now))
	}
	d, err := quotemultigate.Guard(quotemultigate.DefaultPolicy(), quotemultigate.Bundle{Kind: quotemultigate.KindFee, Ticks: ticks}, now)
	if err != nil {
		t.Fatal(err)
	}
	if d.OKTicks != 2 || !d.Quote.FeeAmount.Equal(decimal.RequireFromString(doc.Flat)) {
		t.Fatalf("%+v", d)
	}
}

func TestPolicyHelpers(t *testing.T) {
	if quotemultigate.DefaultPolicy().Production() || !quotemultigate.ProdPolicy().Production() {
		t.Fatal("profile")
	}
	if !quotemultigate.ProdPolicy().RefuseSilentSingle() {
		t.Fatal("prod refuse")
	}
	p := quotemultigate.Policy{Profile: "production"}
	if !p.Production() || !p.RefuseSilentSingle() {
		t.Fatal("production alias")
	}
	z := quotemultigate.Policy{}
	if z.maxSpread() != quotemultigate.DefaultMaxSpreadBPS || z.minVenues() != quotemultigate.DefaultMinVenues {
		t.Fatal("defaults")
	}
	if z.deadline() != quotemultigate.DefaultDeadline || z.roundPlaces() != quotemultigate.DefaultRoundPlaces {
		t.Fatal("d2")
	}
}

func TestSpreadAndFingerprint(t *testing.T) {
	if quotemultigate.SpreadBPS(decimal.Zero, decimal.Zero) != 0 {
		t.Fatal("zero")
	}
	if quotemultigate.SpreadBPS(decimal.NewFromInt(1), decimal.Zero) == 0 {
		t.Fatal("mid0")
	}
	a := decimal.RequireFromString("100")
	b := decimal.RequireFromString("101")
	if quotemultigate.MaxSpreadAmong([]decimal.Decimal{a, b}) <= 0 {
		t.Fatal("spread")
	}
	now := gateNow()
	fps := quotemultigate.UniqueFP([]quotemultigate.Tick{liveTick("a", "BTC/USDT", "1", now), liveTick("b", "BTC/USDT", "1", now)}, 8)
	if len(fps) != 1 {
		t.Fatal(fps)
	}
	if quotemultigate.FormatPlaces(8) != "8" {
		t.Fatal("places")
	}
	if quotemultigate.TickFromQuote("v", quote.Quote{Pair: "ETH/USDT", Rate: a, FeeBPS: 0}).kind("") != quotemultigate.KindRate {
		t.Fatal("kind")
	}
}

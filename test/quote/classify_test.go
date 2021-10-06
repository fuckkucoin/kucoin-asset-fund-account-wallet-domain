package quote_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
)

func testdata(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join("..", "..", "testdata", "quoterefuse", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAcceptCatalogMatchesFixture(t *testing.T) {
	raw := testdata(t, "acceptance_catalog.json")
	var doc struct {
		Cases []struct {
			ID string `json:"id"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	cat := quote.AcceptCatalog()
	if len(doc.Cases) != len(cat) {
		t.Fatalf("catalog size %d fixture %d", len(cat), len(doc.Cases))
	}
	for i, s := range cat {
		if string(s.ID) != doc.Cases[i].ID {
			t.Fatalf("id[%d] %s != %s", i, s.ID, doc.Cases[i].ID)
		}
		if _, ok := quote.LookupAccept(s.ID); !ok {
			t.Fatalf("lookup %s", s.ID)
		}
	}
}

func TestG_T08_TTLBoundaryTable(t *testing.T) {
	raw := testdata(t, "golden_ttl_boundary.json")
	var doc struct {
		Rows []struct {
			Name	string	`json:"name"`
			AgeNS	int64	`json:"age_ns"`
			TTLNS	int64	`json:"ttl_ns"`
			Want	string	`json:"want"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2022, 4, 25, 12, 0, 0, 0, time.UTC)
	for _, row := range doc.Rows {
		ttl := time.Duration(row.TTLNS)
		q := quote.Quote{
			Pair:	"BTC/USDT",
			Rate:	decimal.RequireFromString("64000"),
			AsOf:	asOf,
			TTL:	ttl,
		}
		if row.Name == "ttl_zero" || row.Name == "ttl_neg" {
			q.TTL = ttl
		}
		now := asOf.Add(time.Duration(row.AgeNS))
		got := quote.Classify(q, now).String()
		if row.Want == "empty" && got != "empty" && got != "stale" {
			// 未初始化窗口：Classify 在 TTL=0 时 structuralEmpty → empty
			if got != quote.ClassEmpty.String() {
				t.Fatalf("%s got %s want empty", row.Name, got)
			}
		} else if got != row.Want && !(row.Want == "empty" && got == "empty") {
			if row.Want != got {
				t.Fatalf("%s got %s want %s", row.Name, got, row.Want)
			}
		}
	}
	for _, bc := range quote.DefaultBoundaryTable() {
		if bc.Want == "" {
			t.Fatal("empty want")
		}
	}
}

func TestClassifyTable(t *testing.T) {
	asOf := time.Date(2022,
		4, 25, 8, 0, 0, 0, time.UTC)
	now := asOf.Add(5 * time.Second)
	cases := []struct {
		name	string
		q	quote.Quote
		now	time.Time
		want	quote.Class
	}{
		{"zero", quote.Quote{}, now, quote.ClassEmpty},
		{"nil_levels", quote.Quote{AsOf: asOf, TTL: time.Second}, now, quote.ClassEmpty},
		{"fresh", quote.Quote{Pair: "ETH/USDT", Rate: decimal.RequireFromString("3200"), AsOf: asOf, TTL: 15 * time.Second, Source: quote.SourceMarket}, now, quote.ClassOK},
		{"stale", quote.Quote{Pair: "ETH/USDT", Rate: decimal.RequireFromString("3200"), AsOf: asOf, TTL: time.Second, Source: quote.SourceMarket}, asOf.Add(2 * time.Second), quote.ClassStale},
		{"nan", quote.Quote{Pair: "ETH/USDT", Rate: decimal.RequireFromString("1"), AsOf: asOf, TTL: time.Second, RawRate: "NaN"}, now, quote.ClassIllegal},
		{"neg", quote.Quote{Pair: "ETH/USDT", Rate: decimal.RequireFromString("-1"), AsOf: asOf, TTL: time.Second}, now, quote.ClassIllegal},
		{"inf", quote.Quote{Pair: "ETH/USDT", Rate: decimal.RequireFromString("1"), AsOf: asOf, TTL: time.Second, RawRate: "Inf"}, now, quote.ClassIllegal},
		{"sci", quote.Quote{Pair: "ETH/USDT", Rate: decimal.RequireFromString("1"), AsOf: asOf, TTL: time.Second, RawRate: "1e3"}, now, quote.ClassIllegal},
		{"zero_rate_no_levels", quote.Quote{Pair: "X", Rate: decimal.Zero, AsOf: asOf, TTL: time.Second}, now, quote.ClassEmpty},
		{"bps_only", quote.Quote{Asset: "USDT", FeeBPS: 10, AsOf: asOf, TTL: 30 * time.Second}, now, quote.ClassOK},
		{"valid_levels", quote.Quote{Pair: "BTC/USDT", AsOf: asOf, TTL: time.Second, Levels: []quote.Level{{Px: decimal.RequireFromString("1"), Sz: decimal.RequireFromString("2")}}}, asOf, quote.ClassOK},
		{"zero_levels", quote.Quote{Pair: "BTC/USDT", AsOf: asOf, TTL: time.Second, Levels: []quote.Level{{Px: decimal.Zero, Sz: decimal.RequireFromString("2")}}}, now, quote.ClassEmpty},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := quote.Classify(tc.q, tc.now); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestG_T10_Illegal(t *testing.T) {
	raw := testdata(t, "golden_gt10_illegal.json")
	var doc struct {
		RawRate	string	`json:"raw_rate"`
		WantErr	string	`json:"want_err"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2022, 4, 25, 9, 0, 0, 0, time.UTC)
	q := quote.Quote{Pair: "BTC/USDT", Rate: decimal.RequireFromString("1"), AsOf: asOf, TTL: time.Second, RawRate: doc.RawRate}
	err := quote.GuardConfirm(q, asOf)
	if err == nil || err.Error() != doc.WantErr {
		t.Fatalf("err %v want %s", err, doc.WantErr)
	}
}

func TestG_T09_EmptySourceMarket(t *testing.T) {
	q := quote.Quote{Source: quote.SourceMarket}
	err := quote.GuardConfirm(q, time.Date(2022, 4, 25, 10, 0, 0, 0, time.UTC))
	if err != quote.ErrSourceMasked {
		t.Fatalf("got %v", err)
	}
	if q.IsEmpty() != true {
		t.Fatal("expected empty")
	}
}

func TestHouseDebugNotOKOnConfirm(t *testing.T) {
	q := quote.HouseDebugMid("BTC/USDT", decimal.RequireFromString("64000"), time.Date(2022, 4, 25, 0, 0, 0, 0, time.UTC))
	if q.Source != quote.SourceHouseDebug {
		t.Fatal(q.Source)
	}
	if err := quote.GuardConfirm(q, q.AsOf); err != quote.ErrHouseOnConfirm {
		t.Fatalf("got %v", err)
	}
}

func TestClockResolve(t *testing.T) {
	if _, err := quote.ResolveNow(nil, time.Time{}); err != quote.ErrClockRequired {
		t.Fatal(err)
	}
	fixed := time.Date(2022, 4, 25, 11, 0, 0, 0, time.UTC)
	got, err := quote.ResolveNow(quote.FixedClock{T: fixed}, time.Time{})
	if err != nil || !got.Equal(fixed) {
		t.Fatalf("%v %v", got, err)
	}
	sc := &quote.StepClock{Base: fixed}
	if !sc.Now().Equal(fixed) {
		t.Fatal(sc.Now())
	}
	sc.Advance(time.Second)
	if !sc.Now().Equal(fixed.Add(time.Second)) {
		t.Fatal(sc.Now())
	}
}

func TestIsStaleWrapper(t *testing.T) {
	asOf := time.Date(2022, 4, 25, 7, 0, 0, 0, time.UTC)
	q := quote.Quote{Pair: "A/B", Rate: decimal.RequireFromString("2"), AsOf: asOf, TTL: time.Second}
	if q.IsStale(asOf) {
		t.Fatal("fresh")
	}
	if !q.IsStale(asOf.Add(2 * time.Second)) {
		t.Fatal("should stale")
	}
}

func TestSourceHelpers(t *testing.T) {
	if !quote.MarketLike("MARKET") || !quote.MarketLike(quote.SourceSpotTicker) {
		t.Fatal("market")
	}
	if quote.IsHouseDebug("market") || !quote.IsHouseDebug("HOUSE_DEBUG") {
		t.Fatal("house")
	}
	if quote.NormalizeSource("  ") != "" {
		t.Fatal("unset")
	}
}

func TestDigestHashStable(t *testing.T) {
	asOf := time.Date(2022,
		4, 25, 6, 0, 0, 0, time.UTC)
	q := quote.Quote{Pair: "BTC/USDT", Rate: decimal.RequireFromString("1"), AsOf: asOf, TTL: time.Second, Source: quote.SourceMarket}
	a := q.Digest(asOf).Hash()
	b := q.Clone().Digest(asOf).Hash()
	if a == "" || a != b {
		t.Fatalf("%s %s", a, b)
	}
}

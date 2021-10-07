package quote_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
)

func TestClassifyMatrixFixture(t *testing.T) {
	raw := testdata(t, "matrix_classify.json")
	var doc struct {
		Rows []struct {
			ID	string	`json:"id"`
			Rate	string	`json:"rate"`
			Raw	string	`json:"raw"`
			BPS	int	`json:"bps"`
			AsOf	bool	`json:"as_of"`
			TTLNS	int64	`json:"ttl_ns"`
			Source	string	`json:"source"`
			AgeNS	int64	`json:"age_ns"`
			Class	string	`json:"class"`
			Confirm	string	`json:"confirm"`
			Levels	int	`json:"levels"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	origin := time.Date(2022, 6, 21, 0, 0, 0, 0, time.UTC)
	if len(doc.Rows) < 20 {
		t.Fatalf("rows %d", len(doc.Rows))
	}
	for _, row := range doc.Rows {
		q := quote.Quote{Pair: "BTC/USDT", Source: row.Source, FeeBPS: row.BPS, RawRate: row.Raw}
		if row.Rate != "" {
			q.Rate = decimal.RequireFromString(row.Rate)
		}
		if row.AsOf {
			q.AsOf = origin
		}
		q.TTL = time.Duration(row.TTLNS)
		if row.Levels > 0 {
			q.Levels = []quote.Level{{Px: decimal.RequireFromString("1"), Sz: decimal.RequireFromString("1")}}
		}
		now := origin.Add(time.Duration(row.AgeNS))
		got := quote.Classify(q, now).String()
		if got != row.Class {
			t.Fatalf("%s class %s want %s", row.ID, got, row.Class)
		}
		err := quote.GuardConfirm(q, now)
		switch row.Confirm {
		case "house":
			if !errors.Is(err, quote.ErrHouseOnConfirm) {
				t.Fatalf("%s confirm %v", row.ID, err)
			}
		case "masked":
			if !errors.Is(err, quote.ErrSourceMasked) {
				t.Fatalf("%s confirm %v", row.ID, err)
			}
		default:
			switch row.Class {
			case "ok":
				if err != nil {
					t.Fatalf("%s ok but %v", row.ID, err)
				}
			case "empty":
				if !errors.Is(err, quote.ErrQuoteEmpty) && !errors.Is(err, quote.ErrSourceMasked) {
					t.Fatalf("%s empty %v", row.ID, err)
				}
			case "stale":
				if !errors.Is(err, quote.ErrQuoteStale) {
					t.Fatalf("%s stale %v", row.ID, err)
				}
			case "illegal":
				if !errors.Is(err, quote.ErrQuoteIllegal) {
					t.Fatalf("%s illegal %v", row.ID, err)
				}
			}
		}
	}
}

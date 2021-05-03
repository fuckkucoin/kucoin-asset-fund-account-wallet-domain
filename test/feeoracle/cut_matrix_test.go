package feeoracle_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/feeoracle"
)

func TestFeeCutMatrixFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "quoterefuse", "fee_cut_matrix.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Rows []struct {
			ID	string	`json:"id"`
			Gross	string	`json:"gross"`
			BPS	int	`json:"bps"`
			Flat	string	`json:"flat"`
			OK	bool	`json:"ok"`
			Fee	string	`json:"fee"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Rows) < 10 {
		t.Fatal(len(doc.Rows))
	}
	for _, row := range doc.Rows {
		fee, net, err := feeoracle.Cut(decimal.RequireFromString(row.Gross), feeoracle.FeeQuote{
			BPS:	row.BPS, Flat: decimal.RequireFromString(row.Flat),
		})
		if row.OK {
			if err != nil {
				t.Fatalf("%s %v", row.ID, err)
			}
			if fee.String() != row.Fee {
				t.Fatalf("%s fee %s want %s", row.ID, fee, row.Fee)
			}
			if !net.Add(fee).Equal(decimal.RequireFromString(row.Gross)) {
				t.Fatalf("%s conserve", row.ID)
			}
			continue
		}
		if err == nil {
			t.Fatalf("%s want err", row.ID)
		}
	}
}

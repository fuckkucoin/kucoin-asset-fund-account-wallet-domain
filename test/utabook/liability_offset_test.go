package utabook_test

import (
	"testing"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/utabook"
)

func TestSplitRepayOrder(t *testing.T) {
	row := utabook.LiabilityRow{Principal: "50", AccruedInterest: "5", Interest: "5"}
	sp, err := utabook.SplitRepay(row, decimal.RequireFromString("3"), "reject")
	if err != nil || !sp.Interest.Equal(decimal.RequireFromString("3")) || !sp.Principal.IsZero() {
		t.Fatalf("%+v %v", sp, err)
	}
	sp, err = utabook.SplitRepay(row, decimal.RequireFromString("20"), "reject")
	if err != nil || !sp.Interest.Equal(decimal.RequireFromString("5")) || !sp.Principal.Equal(decimal.RequireFromString("15")) {
		t.Fatalf("%+v %v", sp, err)
	}
	_, err = utabook.SplitRepay(row, decimal.RequireFromString("100"), "reject")
	if err == nil {
		t.Fatal("want excess")
	}
}

func TestApplyInterestAndOffset(t *testing.T) {
	b := utabook.New()
	b.UpsertLiability("u", utabook.LiabilityRow{Currency: "USDT", Principal: "100", AccruedInterest: "0"})
	if _, err := b.ApplyInterestToLiability("u", "USDT", "CROSS", "", decimal.RequireFromString("1")); err != nil {
		t.Fatal(err)
	}
	row, _ := b.LiabilityLoan("u", "USDT", "CROSS", "")
	if row.AccruedDec().String() != "1" {
		t.Fatalf("%s", row.AccruedInterest)
	}
	sp, _ := utabook.SplitRepay(row, decimal.RequireFromString("1.5"), "reject")
	row, err := b.ApplyOffsetToLiability("u", "USDT", "CROSS", "", sp)
	if err != nil {
		t.Fatal(err)
	}
	if row.PrincipalDec().String() != "99.5" || row.AccruedDec().String() != "0" {
		t.Fatalf("%+v", row)
	}
}

package ledger_test

import (
	"testing"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/ledger"
)

func TestInterestAndRepayLinesBalance(t *testing.T) {
	tr := &ledger.Transfer{ID: "t1", UserID: "u", Asset: "USDT"}
	if err := ledger.ValidateJournal(ledger.InterestPostLines(tr, decimal.RequireFromString("1.25"))); err != nil {
		t.Fatal(err)
	}
	if err := ledger.ValidateJournal(ledger.RepayOffsetLines(tr, decimal.RequireFromString("0.25"), decimal.RequireFromString("2"))); err != nil {
		t.Fatal(err)
	}
	if ledger.InterestPostLines(tr, decimal.Zero) != nil {
		t.Fatal("zero interest must skip")
	}
	lines := ledger.RepayOffsetLines(tr, decimal.RequireFromString("1"), decimal.Zero)
	if len(lines) != 2 {
		t.Fatalf("interest-only %d", len(lines))
	}
	mix := append(ledger.RepayOffsetLines(tr, decimal.RequireFromString("1"), decimal.RequireFromString("3")), ledger.SurplusLines(tr, decimal.RequireFromString("0.5"), 5)...)
	if err := ledger.ValidateJournal(mix); err != nil {
		t.Fatal(err)
	}
}

func TestSubjectFixtureSides(t *testing.T) {
	if ledger.InterestIncomeSide() != ledger.AccountFee || ledger.InterestReceivableSide() != ledger.AccountMargin || ledger.PrincipalCashSide() != ledger.AccountSpot {
		t.Fatal("subjects")
	}
	if ledger.InterestPostRuleCount() != 2 || ledger.RepayRuleCount(true, true) != 4 {
		t.Fatal("rule counts")
	}
}

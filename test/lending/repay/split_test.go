package repay_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/utabook"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/lending/repay"
)

func TestSplitMatrix(t *testing.T) {
	row := utabook.LiabilityRow{Principal: "1000", AccruedInterest: "10", Interest: "10"}
	cases := []struct {
		pay	string
		policy	repay.Policy
		i, p, s	string
		err	error
	}{
		{"5", repay.PolicyReject, "5", "0", "0", nil},
		{"10", repay.PolicyReject, "10", "0", "0", nil},
		{"110", repay.PolicyReject, "10", "100", "0", nil},
		{"1010", repay.PolicyReject, "10", "1000", "0", nil},
		{"1011", repay.PolicyReject, "10", "1000", "1", repay.ErrExcess},
		{"1011", repay.PolicySurplus, "10", "1000", "1", nil},
	}
	for _, c := range cases {
		sp, err := repay.Split(row, c.pay, c.policy)
		if err != c.err {
			t.Fatalf("pay=%s pol=%s err=%v want %v", c.pay, c.policy, err, c.err)
		}
		if !sp.Interest.Equal(mustDec(c.i)) || !sp.Principal.Equal(mustDec(c.p)) || !sp.Surplus.Equal(mustDec(c.s)) {
			t.Fatalf("pay=%s split %+v", c.pay, sp)
		}
	}
}

func mustDec(s string) decimal.Decimal {
	v, _ := decimal.NewFromString(s)
	return v
}

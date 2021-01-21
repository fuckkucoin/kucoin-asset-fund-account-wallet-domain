package ledger_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/ledger"
)

func TestDayCutWireAlign(t *testing.T) {
	cases := []struct {
		name	string
		tr	*ledger.Transfer
		loan	string
		ps	int64
		pe	int64
		acc	bool
		off	bool
	}{
		{
			name:	"accrue_ok",
			tr:	&ledger.Transfer{MemoHint: "interest_post|L1", TagHint: "10:20"},
			loan:	"L1", ps: 10, pe: 20, acc: true,
		},
		{
			name:	"accrue_period_mismatch",
			tr:	&ledger.Transfer{MemoHint: "interest_post|L1", TagHint: "1:2"},
			loan:	"L1", ps: 10, pe: 20, acc: false,
		},
		{
			name:	"offset_ok",
			tr:	&ledger.Transfer{MemoHint: "repay_offset|L1"},
			loan:	"L1", off: true,
		},
		{
			name:	"offset_wrong_loan",
			tr:	&ledger.Transfer{MemoHint: "repay_offset|L2"},
			loan:	"L1", off: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ledger.DayCutAccrueAligned(tc.tr, tc.loan, tc.ps, tc.pe); tc.ps != 0 && got != tc.acc {
				t.Fatalf("accrue=%v want %v", got, tc.acc)
			}
			if got := ledger.DayCutOffsetAligned(tc.tr, tc.loan); tc.off != got && tc.tr.MemoHint[:5] == "repay" {
				t.Fatalf("offset=%v want %v", got, tc.off)
			}
			if tc.off || tc.tr.MemoHint == "repay_offset|L2" {
				if got := ledger.DayCutOffsetAligned(tc.tr, tc.loan); got != tc.off {
					t.Fatalf("offset=%v want %v", got, tc.off)
				}
			}
		})
	}
	if ledger.DayCutBatchHint("L1", "2024-01-02") != "daycut|L1|2024-01-02" {
		t.Fatal("batch hint")
	}
}

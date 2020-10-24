package ledger_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/config"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/ledger"
)

func testLoanSvc() (*ledger.Service, *ledger.MemStore,) {
	st := ledger.NewMemStore()
	s := ledger.NewService(st,
		nil, ledger.MemLocker{}, nil, nil, zerolog.Nop(), config.TransferConfig{DisableDriftBlock: true})
	return s, st
}

func TestPostLoanInterestIdempotent(t *testing.T) {
	ctx := context.Background()
	s, st := testLoanSvc()
	req := ledger.LoanPostReq{
		UserID:	"u1", Asset: "USDT", LoanID: "u1|USDT|CROSS|",
		Kind:	ledger.KindInterestPost, Amount: decimal.RequireFromString("0.1"),
		PeriodStart:	10, PeriodEnd: 20, IdempotencyKey: "loan-int:demo",
	}
	a, err := s.PostLoanJournal(ctx, req)
	if err != nil || a.Existed || len(a.Lines) != 2 {
		t.Fatalf("first %+v %v", a, err)
	}
	b, err := s.PostLoanJournal(ctx, req)
	if err != nil || !b.Existed || b.Transfer.ID != a.Transfer.ID {
		t.Fatalf("second %+v %v", b, err)
	}
	acc, _ := st.GetAccount(ctx, "u1", ledger.AccountMargin, "USDT")
	if !acc.Frozen.Equal(decimal.RequireFromString("0.1")) {
		t.Fatalf("frozen %s", acc.Frozen)
	}
}

func TestPostLoanZeroSkipLines(t *testing.T) {
	ctx := context.Background()
	s, _ := testLoanSvc()
	r, err := s.PostLoanJournal(ctx, ledger.LoanPostReq{
		UserID:	"u2", Asset: "USDT", Kind: ledger.KindZeroPeriod, IdempotencyKey: "loan-int:zero",
	})
	if err != nil || !r.Skipped || len(r.Lines) != 0 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestPostLoanRepayConserve(t *testing.T) {
	ctx := context.Background()
	s, st := testLoanSvc()
	st.Seed("u3", ledger.AccountSpot, "USDT", "500")
	_, err := s.PostLoanJournal(ctx, ledger.LoanPostReq{
		UserID:	"u3", Asset: "USDT", Kind: ledger.KindInterestPost, Amount: decimal.RequireFromString("1"),
		IdempotencyKey:	"loan-int:u3",
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.PostLoanJournal(ctx, ledger.LoanPostReq{
		UserID:	"u3", Asset: "USDT", Kind: ledger.KindRepayOffset,
		InterestPart:	decimal.RequireFromString("1"), PrincipalPart: decimal.RequireFromString("40"),
		IdempotencyKey:	"loan-repay:u3",
	})
	if err != nil {
		t.Fatal(err)
	}
	n := ledger.CountMatching(ledger.LoanSubjectFixture(), r.Lines, ledger.KindRepayOffset, "u3")
	if n != ledger.RepayRuleCount(true, true) {
		t.Fatalf("subjects %d lines=%+v", n, r.Lines)
	}
	spot, _ := st.GetAccount(ctx, "u3", ledger.AccountSpot, "USDT")
	if !spot.Available.Equal(decimal.RequireFromString("460")) {
		t.Fatalf("spot %s", spot.Available)
	}
}

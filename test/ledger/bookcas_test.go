package ledger_test

import (
	"context"
	"testing"

	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
)

func TestBookCASRejectsStaleSnapshot(t *testing.T) {
	m := ledger.NewMemStore()
	ctx := context.Background()
	first, err := m.GetAccount(ctx, "u1", ledger.AccountSpot, "USDT")
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.GetAccount(ctx, "u1", ledger.AccountSpot, "USDT")
	if err != nil {
		t.Fatal(err)
	}
	first.Available = decimal.RequireFromString("10")
	if err := m.UpsertAccount(ctx, first); err != nil {
		t.Fatal(err)
	}
	second.Available = decimal.RequireFromString("7")
	if err := m.UpsertAccount(ctx, second); err != ledger.ErrStaleBook {
		t.Fatalf("stale write err=%v", err)
	}
	got, err := m.GetAccount(ctx, "u1", ledger.AccountSpot, "USDT")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Available.Equal(decimal.RequireFromString("10")) {
		t.Fatalf("available=%s", got.Available)
	}
}

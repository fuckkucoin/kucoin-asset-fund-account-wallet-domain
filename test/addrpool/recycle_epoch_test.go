package addrpool_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/addrpool"
)

func TestRecycleEpochHint(t *testing.T) {
	now := time.Date(2021, 8, 14, 12, 0, 0, 0, time.UTC)
	rec := addrpool.RecycleRec{
		Address:	"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Chain:		"ETH",
		UserID:		"userA",
		CoolUntil:	now.Add(time.Hour),
	}
	h := rec.EpochHint(now)
	if h.Ready {
		t.Fatal("should not be ready")
	}
	h = rec.EpochHint(now.Add(2 * time.Hour))
	if !h.Ready {
		t.Fatal("should be ready")
	}
	if addrpool.SlotEpochKey("eth", rec.Address) == "" {
		t.Fatal("key")
	}
}

func TestPeekRecycle(t *testing.T) {
	book := addrpool.NewRecycleBook(nil, time.Hour)
	ctx := context.Background()
	got, err := book.Mark(ctx, "userA", "ETH", "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	peek, err := book.PeekRecycle(ctx, got.Address)
	if err != nil {
		t.Fatal(err)
	}
	if peek.UserID != "userA" {
		t.Fatalf("%+v", peek)
	}
	if _, err := (*addrpool.RecycleBook)(nil).PeekRecycle(ctx, got.Address); err == nil {
		t.Fatal("nil book")
	}
}

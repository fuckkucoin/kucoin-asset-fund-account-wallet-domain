package georebal_test

import (
	"context"
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/georebal"
)

func TestEnqueueRequiresRegionsLive(t *testing.T) {
	book := georebal.NewBook()
	georebal.SeedDemo(book, "p", "l", "i", 50)
	ex := georebal.NewBookExecutor(book)
	ctx := context.Background()
	if _, err := ex.Enqueue(ctx, georebal.EnqueueReq{PlanID: "p", LegID: "l", FenceID: "f"}); err != nil {
		t.Fatal(err)
	}
	_ = book.HaltRegion("sg-1", "ops")
	if _, err := ex.Enqueue(ctx, georebal.EnqueueReq{PlanID: "p", LegID: "l", FenceID: "f"}); !errors.Is(err, georebal.ErrRegionHalted) {
		t.Fatalf("got %v", err)
	}
}

func TestTransferRequiresFence(t *testing.T) {
	book := georebal.NewBook()
	georebal.SeedDemo(book, "p", "l", "i", 50)
	ex := georebal.NewBookExecutor(book)
	_, err := ex.TransferInv(context.Background(), georebal.TransferReq{PlanID: "p", LegID: "l"})
	if !errors.Is(err, georebal.ErrFenceRequired) {
		t.Fatalf("got %v", err)
	}
}

func TestTransferMovesInventoryOnce(t *testing.T) {
	book := georebal.NewBook()
	georebal.SeedDemo(book, "p", "l", "i", 50)
	ex := georebal.NewBookExecutor(book)
	ctx := context.Background()
	if err := ex.ExecLeg(ctx, "p", "l", "f1"); err != nil {
		t.Fatal(err)
	}
	if book.SuccessfulMoves("l") != 1 {
		t.Fatal(book.SuccessfulMoves("l"))
	}
	if book.Balance("sg-1", "USDT") != 1_000_000-50 {
		t.Fatal(book.Balance("sg-1", "USDT"))
	}
}

func TestSnapshotObserve(t *testing.T) {
	book := georebal.NewBook()
	georebal.SeedDemo(book, "p", "l", "i", 1)
	surf := georebal.BookSurface{Book: book}
	snap, err := surf.Snapshot(context.Background(), "p", "l")
	if err != nil || snap.Leg.Amount != 1 {
		t.Fatalf("%+v %v", snap, err)
	}
	h, gen, err := surf.RegionHalted(context.Background(), "sg-1")
	if err != nil || h || gen < 1 {
		t.Fatal(h, gen, err)
	}
}

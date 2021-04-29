package feebumpgate_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/feebumpgate"
)

type memInv []feebumpgate.InventoryRow

func (m memInv) Inventory(context.Context) ([]feebumpgate.InventoryRow, error)	{ return m, nil }

func TestCrossCheckHeld(t *testing.T) {
	eng := feebumpgate.NewEngine(feebumpgate.NewMemStore())
	ctx := context.Background()
	_, err := eng.SignFeeBumpWithGen(ctx, feebumpgate.SignWithGenReq{
		TransferID:	"t-obs", Chain: "BTC", To: "bc1q", Amount: "1", Asset: "BTC",
		NewFee:	"0.0001", Actor: "a",
	})
	if err != nil {
		t.Fatal(err)
	}
	note, err := eng.CrossCheckHeld(ctx, feebumpgate.ObserveBundle{Inv: memInv{
		{Book: "bridge", Asset: "BTC", Frozen: "1", Side: "hot"},
	}}, "t-obs", "BTC")
	if err != nil || note != "aligned" {
		t.Fatalf("%s %v", note, err)
	}
	note, err = eng.CrossCheckHeld(ctx, feebumpgate.ObserveBundle{Inv: memInv{}}, "t-obs", "BTC")
	if err != nil || note != "held_without_hot_frozen" {
		t.Fatalf("%s %v", note, err)
	}
}

func TestHotFrozenFor(t *testing.T) {
	if feebumpgate.HotFrozenFor([]feebumpgate.InventoryRow{{Asset: "eth", Side: "hot", Frozen: "2"}}, "ETH") != "2" {
		t.Fatal("miss")
	}
}

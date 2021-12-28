package wdshard_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/wdshard"
)

func TestSeedAndFreezeRelease(t *testing.T) {
	b := wdshard.NewBook()
	if err := wdshard.SeedDemo(b, "plan-d", "u1", "USDT", 1000, 2); err != nil {
		t.Fatal(err)
	}
	legs := b.ListLegs("plan-d")
	if len(legs) != 2 {
		t.Fatalf("legs %d", len(legs))
	}
	h, err := b.FreezeHold("plan-d", legs[0].LegID, "u1", "USDT", legs[0].Amount)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.ReleaseHold(h.HoldID, h.Amount+1); err != wdshard.ErrHoldExceed {
		t.Fatalf("exceed %v", err)
	}
	if _, err := b.ReleaseHold(h.HoldID, h.Amount); err != nil {
		t.Fatal(err)
	}
}

func TestBroadcastAndFail(t *testing.T) {
	b := wdshard.NewBook()
	h, err := wdshard.SeedFailedLeg(b, "pf", "lf", "u1", "USDT", 50)
	if err != nil {
		t.Fatal(err)
	}
	if h.Amount != 50 {
		t.Fatal(h)
	}
	snap, err := b.Snapshot("pf", "lf")
	if err != nil || snap.Leg.Status != wdshard.HoldLegFailed {
		t.Fatalf("%v %+v", err, snap.Leg)
	}
}

func TestBroadcastingSeed(t *testing.T) {
	b := wdshard.NewBook()
	if _, err := wdshard.SeedBroadcastingLeg(b, "pb", "lb", "u1", "USDT", 70); err != nil {
		t.Fatal(err)
	}
	f, err := b.LiveHashFence("pb", "lb")
	if err != nil || f.Status != wdshard.HoldLegBroadcasting || f.LiveHash == "" {
		t.Fatal(err, f)
	}
	inv := b.Inventory()
	if inv.Broadcasting != 1 {
		t.Fatalf("%+v", inv)
	}
}

func TestAlignFill(t *testing.T) {
	b := wdshard.NewBook()
	if _, err := wdshard.SeedBroadcastingLeg(b, "pa", "la", "u1", "USDT", 40); err != nil {
		t.Fatal(err)
	}
	leg, err := b.AlignFill(wdshard.AlignHint{PlanID: "pa", LegID: "la", FillDelta: 40, TxHash: "0x1"})
	if err != nil || leg.Status != wdshard.HoldLegFilled {
		t.Fatal(err, leg)
	}
}

func TestWorkersPendingFailed(t *testing.T) {
	b := wdshard.NewBook()
	_ = wdshard.SeedDemo(b, "pw", "u1", "USDT", 200, 2)
	aw := wdshard.NewAdvanceWorker(b, 0)
	if len(aw.Pending(nil)) == 0 {
		t.Fatal("pending")
	}
	_, _ = wdshard.SeedFailedLeg(b, "pff", "lff", "u1", "USDT", 10)
	rw := wdshard.NewReleaseWorker(b, 0)
	if len(rw.Failed(nil)) == 0 {
		t.Fatal("failed")
	}
	_, _ = wdshard.SeedBroadcastingLeg(b, "pbb", "lbb", "u1", "USDT", 10)
	al := wdshard.NewAlignWorker(b, 0)
	if len(al.Broadcasting(nil)) == 0 {
		t.Fatal("bc")
	}
}

func TestHoldFuncAdapter(t *testing.T) {
	b := wdshard.NewBook()
	_ = wdshard.SeedDemo(b, "ph", "u1", "USDT", 30, 1)
	f := wdshard.BookHoldFunc{Book: b}
	legs := b.ListLegs("ph")
	h, err := f.Freeze("ph", legs[0].LegID, "u1", "USDT", 15)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Get(h.HoldID); err != nil {
		t.Fatal(err)
	}
}

func TestReadOnlyBatchHook(t *testing.T) {
	v := wdshard.ReadOnlyBatchHook("b1", "r1")
	if !v.Readonly {
		t.Fatal(v)
	}
}

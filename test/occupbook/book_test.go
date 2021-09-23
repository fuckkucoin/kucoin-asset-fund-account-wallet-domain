package occupbook_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/occupbook"
)

func clk() func() time.Time {
	return func() time.Time {
		return time.Date(2025,
			11, 18, 8, 0, 0, 0, time.UTC)
	}
}

func TestBook_ObserveHoldRelease(t *testing.T) {
	b := occupbook.NewBook(nil, occupbook.WithClock(clk()), occupbook.WithRequireFence(false))
	lot, err := b.Observe(context.Background(), occupbook.ObserveReq{
		LotID:	"L1", UserID: "U", Asset: "usdt", Amount: "10", RiskClass: "normal",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !lot.Buckets.Zero() {
		t.Fatal("observe buckets")
	}
	h, lot, err := b.Hold(context.Background(), occupbook.HoldReq{LotID: "L1", Amount: "10", IdemKey: "h1"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !lot.ConserveOK() || !lot.Buckets.Inflight.Equal(occupbook.MustAmount("10")) {
		t.Fatalf("%+v", lot.Buckets)
	}
	_, lot, err = b.Release(context.Background(), occupbook.ReleaseReq{LotID: "L1", HoldID: h.HoldID, Amount: "10", IdemKey: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	if !lot.Buckets.Available.Equal(occupbook.MustAmount("10")) || !lot.ConserveOK() {
		t.Fatalf("%+v", lot)
	}
}

func TestBook_Reclass(t *testing.T) {
	b := occupbook.NewBook(nil, occupbook.WithClock(clk()))
	_, err := b.Observe(context.Background(), occupbook.ObserveReq{LotID: "L2", UserID: "U", Asset: "BTC", Amount: "1", RiskClass: "t2"})
	if err != nil {
		t.Fatal(err)
	}
	h, _, err := b.Hold(context.Background(), occupbook.HoldReq{LotID: "L2", Amount: "1", IdemKey: "h"}, true)
	if err != nil {
		t.Fatal(err)
	}
	_, lot, err := b.Reclass(context.Background(), occupbook.ReclassReq{LotID: "L2", HoldID: h.HoldID, Amount: "1", ToClass: "t4", ReasonCode: "up", IdemKey: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if !lot.Buckets.Void.Equal(occupbook.MustAmount("1")) {
		t.Fatal(lot.Buckets)
	}
	_, _, err = b.Release(context.Background(), occupbook.ReleaseReq{LotID: "L2", HoldID: h.HoldID, Amount: "1", IdemKey: "r"})
	if !errors.Is(err, occupbook.ErrHoldAlreadyReclassed) && !errors.Is(err, occupbook.ErrLotClosed) {
		t.Fatalf("%v", err)
	}
}

func TestBook_MixerRequiresFence(t *testing.T) {
	b := occupbook.NewBook(nil, occupbook.WithClock(clk()), occupbook.WithRequireFence(true))
	_, err := b.Observe(context.Background(), occupbook.ObserveReq{LotID: "Lm", UserID: "U", Asset: "ETH", Amount: "2", RiskClass: "mixer"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = b.Hold(context.Background(), occupbook.HoldReq{LotID: "Lm", Amount: "2", IdemKey: "x"}, false)
	if !errors.Is(err, occupbook.ErrMixerBypass) {
		t.Fatalf("%v", err)
	}
}

func TestBook_HalfResume(t *testing.T) {
	b := occupbook.NewBook(nil, occupbook.WithClock(clk()))
	_, _ = b.Observe(context.Background(), occupbook.ObserveReq{LotID: "Lh", UserID: "U", Asset: "USDT", Amount: "4", RiskClass: "normal"})
	h, _, err := b.Hold(context.Background(), occupbook.HoldReq{LotID: "Lh", Amount: "4", IdemKey: "h"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.MarkHalfHold(context.Background(), "Lh", h.HoldID); err != nil {
		t.Fatal(err)
	}
	_, _, err = b.Hold(context.Background(), occupbook.HoldReq{LotID: "Lh", Amount: "4", IdemKey: "h2"}, true)
	if !errors.Is(err, occupbook.ErrHalfPersist) {
		t.Fatalf("%v", err)
	}
	_, lot, err := b.ResumeHalfHold(context.Background(), "Lh", h.HoldID)
	if err != nil || !lot.ConserveOK() || lot.HalfOpen {
		t.Fatalf("%v %+v", err, lot)
	}
}

func TestBook_IdemHold(t *testing.T) {
	b := occupbook.NewBook(nil, occupbook.WithClock(clk()))
	_, _ = b.Observe(context.Background(), occupbook.ObserveReq{LotID: "Li", UserID: "U", Asset: "USDT", Amount: "7", RiskClass: "normal"})
	h1, _, err := b.Hold(context.Background(), occupbook.HoldReq{LotID: "Li", Amount: "7", IdemKey: "same"}, true)
	if err != nil {
		t.Fatal(err)
	}
	h2, _, err := b.Hold(context.Background(), occupbook.HoldReq{LotID: "Li", Amount: "7", IdemKey: "same"}, true)
	if err != nil || h1.HoldID != h2.HoldID {
		t.Fatalf("%v %s %s", err, h1.HoldID, h2.HoldID)
	}
}

func TestParseAmount(t *testing.T) {
	if _, err := occupbook.ParseAmount(""); !errors.Is(err, occupbook.ErrAmountInvalid) {
		t.Fatal(err)
	}
	if _, err := occupbook.ParseAmount("-1"); !errors.Is(err, occupbook.ErrAmountInvalid) {
		t.Fatal(err)
	}
	d, err := occupbook.ParseAmount("1.50")
	if err != nil || !d.Equal(occupbook.MustAmount("1.50")) {
		t.Fatal(d, err)
	}
}

func TestInspectLot(t *testing.T) {
	lot := &occupbook.Lot{LotID: "x", OccupiedTotal: occupbook.MustAmount("10"), Buckets: occupbook.Bucket{Inflight: occupbook.MustAmount("3"), Available: occupbook.MustAmount("3"), Void: occupbook.MustAmount("3")}}
	s, err := occupbook.InspectLot(lot)
	if !errors.Is(err, occupbook.ErrConserveBroken) || s == nil {
		t.Fatal(err, s)
	}
}

func TestRiskClass(t *testing.T) {
	if !occupbook.RiskT4.RequiresFullOccupancy() || occupbook.RiskNormal.RequiresFullOccupancy() {
		t.Fatal("full")
	}
	if !occupbook.RiskMixer.Valid() || occupbook.RiskClass("nope").Valid() {
		t.Fatal("valid")
	}
}

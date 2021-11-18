package recon_test

import (
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/occuplock"
	"account-assets.kucoin.net/funding/internal/recon"
)

func TestHoldOccupancyRestart(t *testing.T) {
	st := occuplock.NewMem()
	svc, err := occuplock.New(st, occuplock.Policy{Profile: occuplock.ProfileProd, Persist: true, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 22, 6, 0, 0, 0, time.UTC)
	svc.WithClock(func() time.Time { return now })
	lk, err := recon.HoldOccupancy(svc, "u9", "USDT", "recon-w", "tok")
	if err != nil || lk.Key != recon.OccupancyKey("u9", "USDT") {
		t.Fatalf("%+v %v", lk, err)
	}
	svc2, err := occuplock.New(st, occuplock.Policy{Profile: occuplock.ProfileProd, Persist: true, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	svc2.WithClock(func() time.Time { return now.Add(5 * time.Second) })
	lk2, err := recon.HoldOccupancy(svc2, "u9", "USDT", "recon-w", "tok")
	if err != nil || lk2.Generation != lk.Generation {
		t.Fatalf("reentry %+v %v", lk2, err)
	}
	_, err = recon.HoldOccupancy(svc2, "u9", "USDT", "other", "x")
	if !errors.Is(err, occuplock.ErrHeld) {
		t.Fatalf("held %v", err)
	}
	if err := recon.ReleaseOccupancy(svc2, "u9", "USDT", "recon-w", "tok", lk2.Generation); err != nil {
		t.Fatal(err)
	}
}

func TestHoldOccupancyFailClosed(t *testing.T) {
	if _, err := recon.HoldOccupancy(nil, "u", "USDT", "h", "t"); !errors.Is(err, occuplock.ErrStore) {
		t.Fatalf("%v", err)
	}
	svc,
		err := occuplock.New(occuplock.NewMem(), occuplock.Policy{Profile: occuplock.ProfileTest, Persist: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recon.HoldOccupancy(svc, "", "USDT", "h", "t"); !errors.Is(err, occuplock.ErrKey) {
		t.Fatalf("%v", err)
	}
}

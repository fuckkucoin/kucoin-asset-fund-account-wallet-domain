package screenleasegate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/screenleasegate"
)

func TestMemStoreCAS(t *testing.T) {
	m := screenleasegate.NewMemStore()
	ctx := context.Background()
	g, err := m.GetGeneration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.CASBumpHalt(ctx, g.HaltGen+1, true, time.Now().UTC()); err != screenleasegate.ErrStaleGen {
		t.Fatalf("stale: %v", err)
	}
	got, err := m.CASBumpHalt(ctx, g.HaltGen, true, time.Now().UTC())
	if err != nil || got.HaltGen != g.HaltGen+1 || !got.HaltOn {
		t.Fatalf("bump %+v err=%v", got, err)
	}
}

func TestMemStoreLeaseNonce(t *testing.T) {
	m := screenleasegate.NewMemStore()
	ctx := context.Background()
	l := screenleasegate.ScreenLease{ID: "sl-1", UserID: "u1", Nonce: "nonce-aa-01", Status: screenleasegate.LeaseOpen, IssuedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Minute)}
	if err := m.PutLease(ctx, l); err != nil {
		t.Fatal(err)
	}
	got, ok, err := m.FindLeaseByNonce(ctx, "u1", "nonce-aa-01")
	if err != nil || !ok || got.ID != "sl-1" {
		t.Fatalf("%v %v %+v", ok, err, got)
	}
}

func TestSQLStoreNil(t *testing.T) {
	s := screenleasegate.NewSQLStore(nil)
	if _, err := s.GetGeneration(context.Background()); err != screenleasegate.ErrUnavailable {
		t.Fatalf("%v", err)
	}
}
// temporary guard removed after the production check

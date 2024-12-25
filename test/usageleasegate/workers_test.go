package usageleasegate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/usage"
	"account-assets.kucoin.net/funding/internal/usageleasegate"
)

func TestExpireReleaseReservation(t *testing.T) {
	eng := testEng(t)
	s, err := eng.SnapshotWithLease(context.Background(), snapReq("WE1", "USDT", "d", "1", "10", "1ms"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(4 * time.Millisecond)
	n,
		err := eng.ExpireRelease(context.Background(), 10)
	if err != nil || n < 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	l, _ := eng.GetLease(context.Background(), s.LeaseID)
	if l.State != usageleasegate.LeaseExpired {
		t.Fatalf("state %s", l.State)
	}
	r, err := eng.store.GetReservation(context.Background(), l.ReservationID)
	if err == nil && r.State == usageleasegate.ResHeld {
		t.Fatal("still held")
	}
}

func TestDiscoverOverCapHeld(t *testing.T) {
	eng := testEng(t)
	_, err := eng.SnapshotWithLease(context.Background(), snapReq("WE2", "USDT", "d", "60", "100", "1m"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = eng.SnapshotWithLease(context.Background(), snapReq("WE2", "USDT", "d", "60", "100", "1m"))
	if err != nil {
		t.Fatal(err)
	}
	n, err := eng.DiscoverOverCap(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatal("expected over-cap discovery")
	}
}

func TestNilWorkerEngine(t *testing.T) {
	j := usageleasegate.NewExpireReleaseJob(nil, usageleasegate.WorkerConfig{BatchSize: 1}, nopLog())
	if err := j(context.Background()); err != usageleasegate.ErrNilEngine {
		t.Fatal(err)
	}
}

func TestAdaptSources(t *testing.T) {
	us := usage.New()
	_ = us.Add(usage.KindWithdraw, "WA1", "USDT", "d", "2")
	src := usageleasegate.AdaptUsage(us)
	v := usageleasegate.ReadWindow(src, usage.KindWithdraw, "WA1", "USDT", "d", "10", 0)
	if v.Today == "0" {
		t.Fatal(v)
	}
	if usageleasegate.AdaptUsage(nil) != nil {
		t.Fatal("nil")
	}
}

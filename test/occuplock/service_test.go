package occuplock_test

import (
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/occuplock"
)

func TestCheckProdLockdown(t *testing.T) {
	cases := []struct {
		name	string
		p	occuplock.Policy
		want	error
	}{
		{name: "prod_ok", p: occuplock.Policy{Profile: occuplock.ProfileProd, Persist: true}, want: nil},
		{name: "prod_skip", p: occuplock.Policy{Profile: "production", SkipLock: true, Persist: true}, want: occuplock.ErrSkipForbidden},
		{name: "prod_no_persist", p: occuplock.Policy{Profile: occuplock.ProfileProd, Persist: false}, want: occuplock.ErrPersistRequired},
		{name: "prod_steal", p: occuplock.Policy{Profile: occuplock.ProfileProd, Persist: true, AllowSteal: true}, want: occuplock.ErrStealForbidden},
		{name: "prod_mem_fallback", p: occuplock.Policy{Profile: occuplock.ProfileProd, Persist: true, MemFallback: true}, want: occuplock.ErrMemFallback},
		{name: "test_steal_ok", p: occuplock.Policy{Profile: occuplock.ProfileTest, AllowSteal: true}, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := occuplock.CheckProd(tc.p)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
		})
	}
}

func TestNewProdForbidsNilStoreAndFallback(t *testing.T) {
	_, err := occuplock.New(nil, occuplock.Policy{Profile: occuplock.ProfileProd, Persist: true})
	if !errors.Is(err, occuplock.ErrPersistRequired) {
		t.Fatalf("nil store prod: %v", err)
	}
	_, err = occuplock.New(nil, occuplock.Policy{Profile: occuplock.ProfileTest, MemFallback: false})
	if !errors.Is(err, occuplock.ErrStore) {
		t.Fatalf("test no fallback: %v", err)
	}
	svc, err := occuplock.New(nil, occuplock.Policy{Profile: occuplock.ProfileTest, MemFallback: true})
	if err != nil || svc == nil {
		t.Fatalf("test fallback: %v", err)
	}
	_, err = occuplock.New(occuplock.NewMem(), occuplock.Policy{Profile: occuplock.ProfileProd, AllowSteal: true, Persist: true})
	if !errors.Is(err, occuplock.ErrStealForbidden) {
		t.Fatalf("prod steal ctor: %v", err)
	}
}

func TestAcquireIdempotentAndHeld(t *testing.T) {
	now := time.Date(2026, 9, 22, 1, 0, 0, 0, time.UTC)
	st := occuplock.NewMem()
	svc, err := occuplock.New(st, occuplock.Policy{Profile: occuplock.ProfileTest, Persist: true, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	svc.WithClock(func() time.Time { return now })
	lk, err := svc.Acquire("k1", "w1", "tok", occuplock.ScopeRecon)
	if err != nil || lk.Generation != 1 {
		t.Fatalf("first %+v %v", lk, err)
	}
	lk2, err := svc.Acquire("k1", "w1", "tok", occuplock.ScopeRecon)
	if err != nil || lk2.Generation != 1 {
		t.Fatalf("reentry %+v %v", lk2, err)
	}
	_, err = svc.Acquire("k1", "w2", "other", occuplock.ScopeRecon)
	if !errors.Is(err, occuplock.ErrHeld) {
		t.Fatalf("held %v", err)
	}
}

func TestRestartReentrySameStore(t *testing.T) {
	now := time.Date(2026, 9, 22, 2, 0, 0, 0, time.UTC)
	st := occuplock.NewMem()
	svc, err := occuplock.New(st, occuplock.Policy{Profile: occuplock.ProfileProd, Persist: true, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	svc.WithClock(func() time.Time { return now })
	if _, err := svc.Acquire("recon|u|USDT", "worker-a", "tok-a", occuplock.ScopeRecon); err != nil {
		t.Fatal(err)
	}
	// 重启：新 Service 挂同一持久化表。
	svc2,
		err := occuplock.New(st, occuplock.Policy{Profile: occuplock.ProfileProd, Persist: true, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	svc2.WithClock(func() time.Time { return now.Add(10 * time.Second) })
	got, ok, err := svc2.Get("recon|u|USDT")
	if err != nil || !ok || got.Holder != "worker-a" {
		t.Fatalf("persist after restart %+v ok=%v err=%v", got, ok, err)
	}
	lk, err := svc2.Acquire("recon|u|USDT", "worker-a", "tok-a", occuplock.ScopeRecon)
	if err != nil || lk.Holder != "worker-a" {
		t.Fatalf("same owner restart %v %+v", err, lk)
	}
	_, err = svc2.Acquire("recon|u|USDT", "worker-b", "tok-b", occuplock.ScopeRecon)
	if !errors.Is(err, occuplock.ErrHeld) {
		t.Fatalf("other after restart %v", err)
	}
}

func TestStaleFailClosedThenSteal(t *testing.T) {
	now := time.Date(2026, 9, 22, 3, 0, 0, 0, time.UTC)
	st := occuplock.NewMem()
	prod, err := occuplock.New(st, occuplock.Policy{Profile: occuplock.ProfileProd, Persist: true, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	clock := now
	prod.WithClock(func() time.Time { return clock })
	if _, err := prod.Acquire("k", "old", "t1", occuplock.ScopeRecon); err != nil {
		t.Fatal(err)
	}
	clock = now.Add(2 * time.Minute)
	_, err = prod.Acquire("k", "new", "t2", occuplock.ScopeRecon)
	if !errors.Is(err, occuplock.ErrStale) {
		t.Fatalf("prod stale steal %v", err)
	}
	lab, err := occuplock.New(st, occuplock.Policy{Profile: occuplock.ProfileTest, Persist: true, AllowSteal: true, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	lab.WithClock(func() time.Time { return clock })
	lk, err := lab.Acquire("k", "new", "t2", occuplock.ScopeRecon)
	if err != nil || lk.Generation != 2 || lk.Holder != "new" {
		t.Fatalf("test steal %+v %v", lk, err)
	}
}

func TestReleaseGenerationFailClosed(t *testing.T) {
	now := time.Date(2026, 9, 22, 4, 0, 0, 0, time.UTC)
	st := occuplock.NewMem()
	svc, err := occuplock.New(st, occuplock.Policy{Profile: occuplock.ProfileTest, Persist: true, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	svc.WithClock(func() time.Time { return now })
	lk, err := svc.Acquire("k", "h", "tok", occuplock.ScopeRecon)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Release("k", "other", "tok", lk.Generation); !errors.Is(err, occuplock.ErrHeld) {
		t.Fatalf("release other %v", err)
	}
	if err := svc.Release("k", "h", "tok", lk.Generation+3); !errors.Is(err, occuplock.ErrGeneration) {
		t.Fatalf("gen %v", err)
	}
	if err := svc.Release("k", "h", "tok", lk.Generation); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := svc.Get("k"); ok {
		t.Fatal("still held")
	}
}

func TestAcquireInputFailClosed(t *testing.T) {
	svc,
		err := occuplock.New(occuplock.NewMem(), occuplock.Policy{Profile: occuplock.ProfileTest, Persist: true})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		key, holder, token	string
		want			error
	}{
		{"", "h", "t", occuplock.ErrKey},
		{"k", "", "t", occuplock.ErrHolder},
		{"k", "h", "", occuplock.ErrToken},
	}
	for _, tc := range cases {
		_, err := svc.Acquire(tc.key, tc.holder, tc.token, "")
		if !errors.Is(err, tc.want) {
			t.Fatalf("%q/%q/%q err=%v want=%v", tc.key, tc.holder, tc.token, err, tc.want)
		}
	}
}

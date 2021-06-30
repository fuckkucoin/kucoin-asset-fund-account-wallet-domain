package fxmark_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/fxmark"
)

func TestPutBumpsGeneration(t *testing.T) {
	kv := persist.NewMemKV()
	b := fxmark.New(kv)
	seen := 0
	b.ObservePut(func(ctx context.Context, asset, usd string) {
		seen++
		if asset != "ETH" || usd != "12" {
			t.Fatalf("%s %s", asset, usd)
		}
	})
	if err := b.Put(context.Background(), "eth", "12"); err != nil {
		t.Fatal(err)
	}
	if b.Generation().Get("ETH") == 0 || seen != 1 {
		t.Fatalf("gen=%d seen=%d", b.Generation().Get("ETH"), seen)
	}
	n, err := b.USD(context.Background(), "ETH")
	if err != nil || !n.IsPositive() {
		t.Fatal(n, err)
	}
}

func TestFingerprintAndDrift(t *testing.T) {
	p1 := fxmark.RateFingerprint("ETH", "USDT", "10", "kv", 1)
	p2 := fxmark.RateFingerprint("eth", "usdt", "10.00", "kv", 1)
	if p1 != p2 {
		t.Fatal(p1, p2)
	}
	if !fxmark.OverThreshold("100", "101", 10) {
		t.Fatal("should over")
	}
	if fxmark.OverThreshold("100", "100.05", 10) {
		t.Fatal("should not over")
	}
}

func TestAssertPostLeaseFailClosed(t *testing.T) {
	b := fxmark.New(persist.NewMemKV())
	if err := b.AssertPostLease(context.Background(), "", "ETH", "USDT", "convert", ""); err == nil {
		t.Fatal("empty lease must fail")
	}
	called := false
	b.BindPostGuard(func(ctx context.Context, leaseID, from, to, purpose, postRef string) error {
		called = true
		if leaseID != "mkl-1" {
			t.Fatal(leaseID)
		}
		return nil
	})
	if err := b.AssertPostLease(context.Background(), "mkl-1", "ETH", "USDT", "convert", "p"); err != nil || !called {
		t.Fatal(err, called)
	}
}

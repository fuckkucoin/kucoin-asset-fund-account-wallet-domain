package balproofgate_test

import (
	"context"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/balproofgate"
)

func TestMemStoreBumpCAS(t *testing.T) {
	m := balproofgate.NewMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	n, err := m.BumpGeneration(ctx, "u", "main", "USDT", 0, now)
	if err != nil || n != 1 {
		t.Fatalf("%v %d", err, n)
	}
	_, err = m.BumpGeneration(ctx, "u", "main", "USDT", 0, now)
	if err != balproofgate.ErrStaleGen {
		t.Fatalf("err=%v", err)
	}
	n, err = m.BumpGeneration(ctx, "u", "main", "USDT", 1, now)
	if err != nil || n != 2 {
		t.Fatalf("%v %d", err, n)
	}
}

func TestMemStoreConcurrentBump(t *testing.T) {
	m := balproofgate.NewMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	var wg sync.WaitGroup
	ok := 0
	var mu sync.Mutex
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cur, _ := m.CurrentGeneration(ctx, "u", "main", "BTC")
			if _, err := m.BumpGeneration(ctx, "u", "main", "BTC", cur, now); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok < 1 {
		t.Fatal("no winner")
	}
	cur, _ := m.CurrentGeneration(ctx, "u", "main", "BTC")
	if cur < 1 {
		t.Fatalf("cur=%d", cur)
	}
}

func TestMemStoreLeaseNonceLatest(t *testing.T) {
	m := balproofgate.NewMemStore()
	ctx := context.Background()
	t1 := time.Date(2026, 9, 21, 1, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)
	_ = m.SaveLease(ctx, &balproofgate.ProofLease{ID: "a", UserID: "u", Book: "main", Asset: "USDT", Nonce: "n", IssuedAt: t1, Status: balproofgate.LeaseExpired})
	_ = m.SaveLease(ctx, &balproofgate.ProofLease{ID: "b", UserID: "u", Book: "main", Asset: "USDT", Nonce: "n", IssuedAt: t2, Status: balproofgate.LeaseBound})
	got, err := m.GetLeaseByNonce(ctx, "u", "main", "USDT", "n")
	if err != nil || got == nil || got.ID != "b" {
		t.Fatalf("%v %+v", err, got)
	}
}

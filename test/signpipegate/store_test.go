package signpipegate_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/signpipegate"
)

func TestBumpGenerationCAS(t *testing.T) {
	st := signpipegate.NewMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	g1, err := st.BumpGeneration(ctx, "mv", "job", 0, now)
	if err != nil || g1 != 1 {
		t.Fatalf("%d %v", g1, err)
	}
	if _, err := st.BumpGeneration(ctx, "mv", "job", 0, now); err != signpipegate.ErrStaleGen {
		t.Fatalf("%v", err)
	}
	g2, err := st.BumpGeneration(ctx, "mv", "job", 1, now)
	if err != nil || g2 != 2 {
		t.Fatalf("%d %v", g2, err)
	}
}

func TestConcurrentBump(t *testing.T) {
	st := signpipegate.NewMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	var win atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := st.BumpGeneration(ctx, "mv-c", "job-c", 0, now); err == nil {
				win.Add(1)
			}
		}()
	}
	wg.Wait()
	if win.Load() != 1 {
		t.Fatalf("winners %d", win.Load())
	}
}

func TestArtifactUnique(t *testing.T) {
	st := signpipegate.NewMemStore()
	ctx := context.Background()
	a := &signpipegate.SignedArtifact{ID: "a1", MovementID: "mv", LeaseID: "l1", Digest: "d"}
	if err := st.SaveArtifact(ctx, a); err != nil {
		t.Fatal(err)
	}
	b := &signpipegate.SignedArtifact{ID: "a2", MovementID: "mv", LeaseID: "l2", Digest: "e"}
	if err := st.SaveArtifact(ctx, b); err != signpipegate.ErrDoubleArtifact {
		t.Fatalf("%v", err)
	}
}

func TestFindActiveLease(t *testing.T) {
	st := signpipegate.NewMemStore()
	ctx := context.Background()
	l := &signpipegate.InflightLease{ID: "l1", MovementID: "mv", Status: signpipegate.LeaseArmed, Nonce: "n", ArmedAt: time.Now()}
	if err := st.SaveLease(ctx, l); err != nil {
		t.Fatal(err)
	}
	got, err := st.FindActiveLease(ctx, "mv")
	if err != nil || got == nil {
		t.Fatal(err)
	}
}

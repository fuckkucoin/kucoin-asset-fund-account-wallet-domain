package holdopsgate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/holdopsgate"
)

func TestMemStoreCAS(t *testing.T) {
	m := holdopsgate.NewMemStore()
	ctx := context.Background()
	g, err := m.GetGeneration(ctx)
	if err != nil || g.OpsGen != 1 {
		t.Fatalf("%+v %v", g, err)
	}
	_, err = m.CASBumpOps(ctx, 99, false, "x", time.Now().UTC())
	if err != holdopsgate.ErrStaleGen {
		t.Fatalf("stale: %v", err)
	}
	g2, err := m.CASBumpOps(ctx, 1, true, "fence", time.Now().UTC())
	if err != nil || g2.OpsGen != 2 || !g2.FenceOn {
		t.Fatalf("%+v %v", g2, err)
	}
}

func TestMemStoreLeaseNonce(t *testing.T) {
	m := holdopsgate.NewMemStore()
	ctx := context.Background()
	l := holdopsgate.HoldOpsLease{ID: "ols1", Actor: "a", Nonce: "n", Status: holdopsgate.LeaseOpen, TokenID: "t"}
	if err := m.PutLease(ctx, l); err != nil {
		t.Fatal(err)
	}
	if err := m.PutLease(ctx, l); err != holdopsgate.ErrIdempotentNonce {
		t.Fatal(err)
	}
	got, ok, err := m.FindLeaseByNonce(ctx, "a", "n")
	if err != nil || !ok || got.ID != "ols1" {
		t.Fatalf("%v %v %+v", ok, err, got)
	}
}

func TestMemStoreAuditReplay(t *testing.T) {
	m := holdopsgate.NewMemStore()
	ctx := context.Background()
	a := holdopsgate.HoldOpsAudit{ID: "1", ReplayKey: "r1", OpsID: "o"}
	if err := m.PutAudit(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := m.PutAudit(ctx, a); err != holdopsgate.ErrReplayAudit {
		t.Fatal(err)
	}
}

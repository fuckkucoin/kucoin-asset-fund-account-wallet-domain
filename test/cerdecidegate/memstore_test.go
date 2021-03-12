package cerdecidegate_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/cerdecidegate"
)

func TestMemCASHappyAndConflict(t *testing.T) {
	m := cerdecidegate.NewMemStore()
	ctx := context.Background()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	row := cerdecidegate.DecideLease{LeaseID: "l1", BatchID: "b", Asset: "BTC", Chain: "BTC", Amount: "1", Dest: "d", State: cerdecidegate.LeaseIssued, Idem: "i1", IssuedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := m.SaveLease(ctx, row); err != nil {
		t.Fatal(err)
	}
	if err := m.CASState(ctx, "l1", cerdecidegate.LeaseIssued, cerdecidegate.LeaseBound, cerdecidegate.DecideLease{BoundAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	got, err := m.GetLease(ctx, "l1")
	if err != nil || got.State != cerdecidegate.LeaseBound {
		t.Fatalf("%+v %v", got, err)
	}
	if err := m.CASState(ctx, "l1", cerdecidegate.LeaseIssued, cerdecidegate.LeaseBound, cerdecidegate.DecideLease{}); !errors.Is(err, cerdecidegate.ErrLeaseNotIssued) && !errors.Is(err, cerdecidegate.ErrDupBind) && !errors.Is(err, cerdecidegate.ErrInvalidTransition) {
		t.Fatalf("conflict %v", err)
	}
}

func TestMemIdemUnique(t *testing.T) {
	m := cerdecidegate.NewMemStore()
	ctx := context.Background()
	a := cerdecidegate.DecideLease{LeaseID: "a", Idem: "k", State: cerdecidegate.LeaseIssued, BatchID: "b"}
	b := cerdecidegate.DecideLease{LeaseID: "b", Idem: "k", State: cerdecidegate.LeaseIssued, BatchID: "b"}
	if err := m.SaveLease(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveLease(ctx, b); !errors.Is(err, cerdecidegate.ErrIdem) {
		t.Fatalf("want idem got %v", err)
	}
}

func TestGenerationBump(t *testing.T) {
	m := cerdecidegate.NewMemStore()
	ctx := context.Background()
	n1, err := m.CurrentGeneration(ctx, "batch:x")
	if err != nil || n1 != 1 {
		t.Fatalf("%d %v", n1, err)
	}
	n2, err := m.NextGeneration(ctx, "batch:x")
	if err != nil || n2 != 2 {
		t.Fatalf("%d %v", n2, err)
	}
}

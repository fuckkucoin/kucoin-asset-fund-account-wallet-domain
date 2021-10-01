package portfolio_test

import (
	"context"
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/batchhopgate"
	"account-assets.kucoin.net/funding/internal/portfolio"
)

func TestBatchHopWithResidual_PartialFailDropsCase(t *testing.T) {
	svc := portfolio.NewMem()
	if _, err := svc.Seed("u-hook-01", "funding", "USDT", "10000", "0"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Seed("u-hook-01", "trading", "USDT", "0", "0"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Seed("u-hook-01", "earn", "USDT", "0", "0"); err != nil {
		t.Fatal(err)
	}
	gate := batchhopgate.NewService(nil)
	ctx := context.Background()
	items := []portfolio.BatchItem{
		{From: "funding", To: "trading", Asset: "USDT", Amount: "3000", Idem: "h0"},
		{From: "funding", To: "earn", Asset: "USDT", Amount: "2000", Idem: "h1"},
		{From: "funding", To: "trading", Asset: "USDT", Amount: "99999999", Idem: "h2"},
	}
	got, err := svc.BatchHopWithResidual(ctx, gate, "u-hook-01", items)
	if err == nil {
		t.Fatal("expected fail on last item")
	}
	if got.Residual == nil {
		t.Fatal("residual must be persisted")
	}
	if got.Residual.FailIndex != 2 {
		t.Errorf("fail_index=%d want 2", got.Residual.FailIndex)
	}
	if len(got.Residual.SucceededHops) != 2 {
		t.Errorf("succeeded=%d want 2", len(got.Residual.SucceededHops))
	}
	if got.Residual.Status != batchhopgate.ResidualProven {
		t.Errorf("status=%s want proven", got.Residual.Status)
	}
	stored,
		err := gate.GetResidual(ctx, got.Residual.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != got.Residual.ID || stored.FailIndex != got.Residual.FailIndex {
		t.Error("return body must match store")
	}
}

func TestBatchHopWithResidual_ConcurrentBlocked(t *testing.T) {
	svc := portfolio.NewMem()
	if _, err := svc.Seed("u-hook-02", "funding", "USDT", "10000", "0"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Seed("u-hook-02", "trading", "USDT", "0", "0"); err != nil {
		t.Fatal(err)
	}
	gate := batchhopgate.NewService(nil)
	ctx := context.Background()
	failItems := []portfolio.BatchItem{
		{From: "funding", To: "trading", Asset: "USDT", Amount: "100", Idem: "a0"},
		{From: "funding", To: "trading", Asset: "USDT", Amount: "99999999", Idem: "a1"},
	}
	_, err := svc.BatchHopWithResidual(ctx, gate, "u-hook-02", failItems)
	if err == nil {
		t.Fatal("expected residual")
	}
	_, err = svc.BatchHopWithResidual(ctx, gate, "u-hook-02", []portfolio.BatchItem{
		{From: "funding", To: "trading", Asset: "USDT", Amount: "1", Idem: "b0"},
	})
	if !errors.Is(err, batchhopgate.ErrConcurrentBatch) {
		t.Errorf("got %v want ErrConcurrentBatch", err)
	}
}

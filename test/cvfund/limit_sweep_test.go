package cvfund_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/fxmark"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/cvfund"
)

func TestSweepExpiredLimitsRestoresAndIdempotent(t *testing.T) {
	ctx := context.Background()
	kv := persist.NewMemKV()
	marks := fxmark.New(kv)
	if err := marks.SeedStables(ctx); err != nil {
		t.Fatal(err)
	}
	mem := ledger.NewMemStore()
	ledger.SeedBooks(mem)
	uid := config.DefaultSeedUserID
	risk, err := ledger.NewRisk(config.RiskConfig{
		MaxSingle:	"5000", DailyLimit: "100000", SuspiciousAmount: "100000",
	})
	if err != nil {
		t.Fatal(err)
	}
	led := ledger.NewService(mem, risk, &ledger.MutexLocker{}, nil, nil, zerolog.Nop(), config.TransferConfig{})
	b := cvfund.New().BindKV(kv).BindLedger(led).BindMarks(marks)
	before, err := mem.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	if err != nil {
		t.Fatal(err)
	}
	open, err := b.PlaceLimit(ctx, uid, "lim-sweep-1", "USDT", "USDC", "10", "11", "MAIN")
	if err != nil || open.Status != "OPEN" {
		t.Fatalf("place %v %+v", err, open)
	}
	mid, err := mem.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	if err != nil {
		t.Fatal(err)
	}
	if !mid.Available.Equal(before.Available.Sub(decimal.RequireFromString("10"))) {
		t.Fatalf("debit avail %s want %s-10", mid.Available, before.Available)
	}
	open.ExpiryTime = time.Now().UTC().Add(-time.Hour).UnixMilli()
	if err := b.saveLimit(ctx, open); err != nil {
		t.Fatal(err)
	}
	n, err := b.SweepExpiredLimits(ctx, 200)
	if err != nil || n != 1 {
		t.Fatalf("sweep n=%d err=%v", n, err)
	}
	got, err := b.GetLimit(ctx, uid, open.ID, "")
	if err != nil || got.Status != "EXPIRED" {
		t.Fatalf("status %v %+v", err, got)
	}
	after, err := mem.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	if err != nil {
		t.Fatal(err)
	}
	if !after.Available.Equal(before.Available) {
		t.Fatalf("restore avail %s want %s", after.Available, before.Available)
	}
	n2, err := b.SweepExpiredLimits(ctx, 200)
	if err != nil || n2 != 0 {
		t.Fatalf("idempotent n=%d err=%v", n2, err)
	}
	after2, err := mem.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	if err != nil {
		t.Fatal(err)
	}
	if !after2.Available.Equal(before.Available) {
		t.Fatalf("idempotent avail %s", after2.Available)
	}
}

func TestSweepExpiredLimitsNilSafe(t *testing.T) {
	var b *cvfund.Book
	n, err := b.SweepExpiredLimits(context.Background(), 0)
	if err != nil || n != 0 {
		t.Fatalf("%d %v", n, err)
	}
	n, err = cvfund.New().SweepExpiredLimits(context.Background(), -1)
	if err != nil || n != 0 {
		t.Fatalf("no kv %d %v", n, err)
	}
}

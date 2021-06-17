package fundpath_test

import (
	"context"
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/ledshard"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/fundpath"
)

type crossPlan struct{}

func (crossPlan) SettleHop(user, from, to, asset, amount, idem string) (ledshard.HopPlan, error) {
	return ledshard.HopPlan{SameShard: false, SrcShard: "sh-00", DstShard: "sh-01", Version: 1, IntentID: "int-cross", State: "committed"}, nil
}

func TestHopSameShardStillMovesLedger(t *testing.T) {
	ctx := context.Background()
	mem := ledger.NewMemStore()
	ledger.SeedBooks(mem)
	uid := config.DefaultSeedUserID
	risk, err := ledger.NewRisk(config.RiskConfig{MaxSingle: "5000", DailyLimit: "100000", SuspiciousAmount: "100000"})
	if err != nil {
		t.Fatal(err)
	}
	led := ledger.NewService(mem, risk, &ledger.MutexLocker{}, nil, nil, zerolog.Nop(), config.TransferConfig{})
	acc := account.New(led)
	if _, err := acc.Open(ctx, uid, account.BookTrading); err != nil {
		t.Fatal(err)
	}
	eng := ledshard.New()
	if err := eng.Bootstrap(8); err != nil {
		t.Fatal(err)
	}
	acc.BindShardClassifier(eng)
	svc := fundpath.New().BindAccount(acc).BindShard(eng)
	before, err := mem.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	if err != nil {
		t.Fatal(err)
	}
	id, err := svc.Hop(ctx, fundpath.HopReq{
		UserID:	uid, FromBook: "funding", ToBook: "trading", Asset: "USDT", Amount: "3", IdempotencyKey: "hop-same-1",
	})
	if err != nil || id == "" {
		t.Fatalf("hop %s %v", id, err)
	}
	rec, err := svc.xfer.Get(ctx, "hop-same-1")
	if err != nil || !rec.SameShard || rec.SrcShard == "" || rec.SrcShard != rec.DstShard || rec.TransferID != id {
		t.Fatalf("xfer %+v %v", rec, err)
	}
	after, err := mem.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	if err != nil {
		t.Fatal(err)
	}
	if !after.Available.Equal(before.Available.Sub(decimal.RequireFromString("3"))) {
		t.Fatalf("spot %s want %s", after.Available, before.Available)
	}
}

func TestHopCrossShardDoesNotMoveLedger(t *testing.T) {
	ctx := context.Background()
	mem := ledger.NewMemStore()
	ledger.SeedBooks(mem)
	uid := config.DefaultSeedUserID
	risk, err := ledger.NewRisk(config.RiskConfig{MaxSingle: "5000", DailyLimit: "100000", SuspiciousAmount: "100000"})
	if err != nil {
		t.Fatal(err)
	}
	led := ledger.NewService(mem, risk, &ledger.MutexLocker{}, nil, nil, zerolog.Nop(), config.TransferConfig{})
	acc := account.New(led)
	if _, err := acc.Open(ctx, uid, account.BookTrading); err != nil {
		t.Fatal(err)
	}
	svc := fundpath.New().BindAccount(acc).BindShard(crossPlan{})
	before, err := mem.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	if err != nil {
		t.Fatal(err)
	}
	id, err := svc.Hop(ctx, fundpath.HopReq{
		UserID:	uid, FromBook: "funding", ToBook: "trading", Asset: "USDT", Amount: "3", IdempotencyKey: "hop-cross-1",
	})
	if err != nil || id != "int-cross" {
		t.Fatalf("hop %s %v", id, err)
	}
	after, err := mem.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	if err != nil {
		t.Fatal(err)
	}
	if !after.Available.Equal(before.Available) {
		t.Fatalf("ledger moved %s", after.Available)
	}
	acc.BindShardClassifier(rejectCross{})
	_, err = acc.Move(ctx, account.MoveReq{
		UserID:	uid, From: account.BookFunding, To: account.BookTrading,
		Asset:	"USDT", Amount: decimal.RequireFromString("1"), IdempotencyKey: "direct-cross-2",
	})
	if !errors.Is(err, account.ErrCrossShard) {
		t.Fatalf("move %v", err)
	}
}

type rejectCross struct{}

func (rejectCross) ClassifyHop(context.Context, string, string, string, string) (account.ShardClass, error) {
	return account.ShardClass{SameShard: false, SrcShard: "sh-00", DstShard: "sh-01", Version: 1}, nil
}

package kcfund_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/kcfund"
)

func TestSweepOrphanHoldcatsReleaseMatchedUntouchedIdempotent(t *testing.T) {
	rt, err := app.OpenMem()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	book := holdcat.New(rt.KV)
	orphan, err := book.Place(ctx, "u-orphan", "USDT", "funding", "5", holdcat.ReasonConvertLimit, "lim-orphan")
	if err != nil {
		t.Fatal(err)
	}
	matched, err := book.Place(ctx, "u-match", "USDT", "funding", "7", holdcat.ReasonP2PSell, "p2p-ok")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Store.InsertTransfer(ctx, &ledger.Transfer{
		ID:	"pk-match", IdempotencyKey: "pk-match", UserID: "u-match",
		Direction:	ledger.P2PDebit, Asset: "USDT", Amount: decimal.RequireFromString("7"),
		State:	ledger.StateCredited,
	}); err != nil {
		t.Fatal(err)
	}
	n, err := kcfund.SweepOrphanHoldcats(ctx, rt, 200)
	if err != nil || n != 1 {
		t.Fatalf("sweep n=%d err=%v", n, err)
	}
	gotOrphan, err := book.Get(ctx, "u-orphan", orphan.ID)
	if err != nil || gotOrphan.Open {
		t.Fatalf("orphan still open %v %+v", err, gotOrphan)
	}
	gotMatch, err := book.Get(ctx, "u-match", matched.ID)
	if err != nil || !gotMatch.Open {
		t.Fatalf("matched released %v %+v", err, gotMatch)
	}
	n2, err := kcfund.SweepOrphanHoldcats(ctx, rt, 200)
	if err != nil || n2 != 0 {
		t.Fatalf("idempotent n=%d err=%v", n2, err)
	}
	gotMatch2, err := book.Get(ctx, "u-match", matched.ID)
	if err != nil || !gotMatch2.Open {
		t.Fatalf("idempotent matched %v %+v", err, gotMatch2)
	}
}

func TestSweepOrphanHoldcatsNilSafe(t *testing.T) {
	n, err := kcfund.SweepOrphanHoldcats(context.Background(), nil, 0)
	if err != nil || n != 0 {
		t.Fatalf("%d %v", n, err)
	}
	n, err = kcfund.SweepOrphanHoldcats(nil, &app.Runtime{}, -1)
	if err != nil || n != 0 {
		t.Fatalf("empty rt %d %v", n, err)
	}
}

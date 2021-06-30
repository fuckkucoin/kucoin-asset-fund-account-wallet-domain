package flow

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/cvfund"
	"account-assets.kucoin.net/funding/internal/fxmark"
	"account-assets.kucoin.net/funding/internal/ledger"
)

  func TestLimitPlaceFillCancel(t *testing.T) {
	  rt, err := app.OpenMem()
	  if err != nil {
		   t.Fatal(err)
	  }
	  ctx := context.Background()
	  uid := rt.Cfg.SeedUserID
	  if mem, ok := rt.Store.(*ledger.MemStore); ok {
		  mem.Seed("system", ledger.AccountBridge, "USDC", "10000")
	  }
	  b := rt.Convert
	  q, until, err := b.LimitQuote(ctx, "USDT", "USDC", "10", "10")
	   if err != nil || until == 0 || !q.IsPositive() {
		   t.Fatalf("quote %v %s", err, q)
	   }
	  filled, err := b.PlaceLimit(ctx, uid, "lim-fill-1", "USDT", "USDC", "10", "10", "MAIN")
	   if err != nil || filled.Status != "FILLED" {
		   t.Fatalf("fill %v %+v", err, filled)
	  }
	   again, err := b.PlaceLimit(ctx, uid, "lim-fill-1", "USDT", "USDC", "10", "10", "MAIN")
	   if err != nil || again.ID != filled.ID {
		  t.Fatalf("idem %v %+v", err, again)
	  }
	  open, err := b.PlaceLimit(ctx, uid, "lim-open-1", "USDT", "USDC", "10", "11", "MAIN")
	  if err != nil || open.Status != "OPEN" {
		  t.Fatalf("open %v %+v", err, open)
	   }
	   got, err := b.CancelLimit(ctx, uid, open.ID, "")
	   if err != nil || got.Status != "CANCELLED" {
		   t.Fatalf("cancel %v %+v", err, got)
	  }
	   if n := len(b.ListLimits(ctx, uid, "CANCELLED", 10)); n == 0 {
		  t.Fatal("list cancelled")
	  }
   }

   func TestLimitProtect(t *testing.T) {
	  rt, err := app.OpenMem()
	   if err != nil {
		   t.Fatal(err)
	  }
	   if err := fxmark.New(rt.KV).Put(context.Background(), "USDC", "1"); err != nil {
		  t.Fatal(err)
	  }
	   if _, err := rt.Convert.PlaceLimit(context.Background(), rt.Cfg.SeedUserID, "lim-bad", "USDT", "USDC", "10", "0.5", "MAIN"); err != cvfund.ErrProtect {
		  t.Fatalf("protect %v", err)
	  }
  }

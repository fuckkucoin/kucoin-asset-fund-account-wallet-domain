package flow

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
)

  func TestConcurrentInternalTransfersKeepBooks(t *testing.T) {
	  rt := openMem(t)
	  ctx := context.Background()
	  uid := seedUser(rt)
	   const n = 8
	   var ok atomic.Int32
	   var wg sync.WaitGroup
	   wg.Add(n)
	   for i := 0; i < n; i++ {
		  i := i
		   go func() {
			   defer wg.Done()
			   _, err := rt.Ledger.Create(ctx, ledger.CreateReq{
				  UserID: uid, Direction: ledger.SpotToWallet, Asset: "USDT",
				   Amount: decimal.RequireFromString("1"), Chain: "ETH",
				  ToAddress:      dest(uid, "ETH"),
				   IdempotencyKey: "flow-perf-" + string(rune('a'+i)),
				   WaitConfirm: true,
			  })
			  if err == nil {
				  ok.Add(1)
			  }
		   }()
	  }
	   wg.Wait()
	  if ok.Load() != n {
		  t.Fatalf("ok=%d want %d", ok.Load(), n)
	   }
	  spot, err := rt.Store.GetAccount(ctx, uid, ledger.AccountSpot, "USDT")
	  if err != nil {
		  t.Fatal(err)
	   }
	  if !spot.Frozen.IsZero() {
		  t.Fatalf("leftover frozen %s", spot.Frozen)
	  }
	  rep, err := ledger.Reconcile(ctx, rt.Store, uid)
	   if err != nil || !rep.OK {
		  t.Fatalf("reconcile %+v %v", rep, err)
	   }
   }

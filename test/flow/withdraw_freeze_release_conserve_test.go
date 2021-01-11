package flow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/shopspring/decimal"
)

// TestConcurrentWithdrawFreezeReleaseConserves 并发出金在 KYT review 下保持冻结，取消后钱包可用+冻结守恒，且无残留 pending_withdraw。
   func TestConcurrentWithdrawFreezeReleaseConserves(t *testing.T) {
	   rt := openMem(t)
	   ctx := context.Background()
	   uid := seedUser(rt)
	  rt.Ledger.SetKYT(kyt.Static{Mode: kyt.Review})
	  before, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	  if err != nil {
		  t.Fatal(err)
	  }
	  beforeTotal := before.Available.Add(before.Frozen)
	   holds := holdcat.New(rt.KV)
	   const n = 12
	  var ok atomic.Int32
	   var wg sync.WaitGroup
	   wg.Add(n)
	  for i := 0; i < n; i++ {
		  i := i
		  go func() {
			  defer wg.Done()
			   amt := decimal.RequireFromString("1")
			  tr, err := rt.Ledger.Create(ctx, ledger.CreateReq{
				   UserID: uid, Direction: ledger.WalletWithdraw, Asset: "USDT",
				  Amount: amt, Chain: "ETH", ToAddress: extDest("ETH"),
				  IdempotencyKey: fmt.Sprintf("wd-kyt-frz-%d", i),
				  WaitConfirm:    true,
			   })
			  if !errors.Is(err, ledger.ErrKYTReview) || tr == nil {
				  return
			  }
			   if _, err := holds.PlacePendingWithdraw(ctx, uid, "USDT", amt.String(), tr.ID); err != nil {
				   return
			  }
			   if err := rt.Ledger.Cancel(ctx, tr.ID, "perf-conserve-release"); err != nil {
				   return
			   }
			  if _, err := holds.ReleaseByRef(ctx, uid, holdcat.ReasonPendingWithdraw, tr.ID); err != nil {
				   return
			  }
			  ok.Add(1)
		  }()
	  }
	  wg.Wait()
	  if ok.Load() != n {
		  t.Fatalf("completed=%d want %d", ok.Load(), n)
	  }
	  after, err := rt.Store.GetAccount(ctx, uid, ledger.AccountWallet, "USDT")
	  if err != nil {
		  t.Fatal(err)
	  }
	  afterTotal := after.Available.Add(after.Frozen)
	  left, err := money.ParseBook(beforeTotal.String())
	  if err != nil {
		   t.Fatal(err)
	  }
	   right, err := money.ParseBook(afterTotal.String())
	   if err != nil {
		  t.Fatal(err)
	  }
	  if err := money.Conserve(left, right); err != nil {
		   t.Fatalf("wallet total not conserved: before=%s after=%s avail=%s frozen=%s err=%v",
			  beforeTotal, afterTotal, after.Available, after.Frozen, err)
	   }
	   if !after.Frozen.IsZero() {
		   t.Fatalf("leftover frozen %s", after.Frozen)
	   }
	  rows, err := holds.List(ctx, uid, true)
	   if err != nil {
		  t.Fatal(err)
	  }
	   for _, r := range rows {
		  if r.Reason == holdcat.ReasonPendingWithdraw && r.Open {
			   t.Fatalf("open pending_withdraw after release: %+v", r)
		  }
	   }
	   rep,
		  err := ledger.Reconcile(ctx, rt.Store, uid)
	   if err != nil || !rep.OK {
		  t.Fatalf("reconcile %+v %v", rep, err)
	   }
  }

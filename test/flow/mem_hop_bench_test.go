package flow

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/ledbatch"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
)

// TestMemHopBatchConserve 校验 hop 批次前后科目守恒。

   func TestMemHopBatchConserve(t *testing.T) {
	  rt, err := app.OpenMem()
	   if err != nil {
		   t.Fatal(err)
	   }
	   uid := rt.Cfg.SeedUserID
	   ctx := context.Background()
	  const n, workers = 4, 2
	  var fail atomic.Int64
	   var wg sync.WaitGroup
	  for w := 0; w < workers; w++ {
		  wg.Add(1)
		   go func(wid int) {
			  defer wg.Done()
			  s := ledbatch.New().BindLedger(rt.Ledger)
			  items := make([]ledbatch.Item, 0, n)
			   for i := 0; i < n; i++ {
				  items = append(items, ledbatch.Item{
					  UserID: uid, Asset: "USDT", Amount: "1",
					  Direction: string(ledger.FundingToTrading),
					   IdempotencyKey: "mem-b-" + strconv.Itoa(wid) + "-" + strconv.Itoa(i),
				  })
			  }
			  b, err := s.Open(ledbatch.OpenReq{
				   Actor: "test", IdempotencyKey: "mem-batch-" + strconv.Itoa(wid), Items: items,
			  })
			  if err != nil {
				  fail.Add(1)
				   return
			  }
			   got, err := s.Submit(ctx, b.ID, "test")
			   if err != nil {
				   fail.Add(1)
				   return
			   }
			  req, _ := money.ParseBook(got.Requested)
			   sub, _ := money.ParseBook(got.Submitted)
			  sk, _ := money.ParseBook(got.Skipped)
			   fl, _ := money.ParseBook(got.Failed)
			   right, _ := sub.Add(sk)
			  right, _ = right.Add(fl)
			   if err := money.Conserve(req, right); err != nil {
				  fail.Add(1)
			   }
		  }(w)
	  }
	   wg.Wait()
	   if fail.Load() > 0 {
		   t.Fatalf("fail=%d", fail.Load())
	  }
  }

// TestMemHopBatchManyUsersConserve 多用户批次提交按用户排序且请求额守恒（轻量争用）。

  func TestMemHopBatchManyUsersConserve(t *testing.T) {
	  rt, err := app.OpenMem()
	   if err != nil {
		  t.Fatal(err)
	   }
	  ctx := context.Background()
	  mem, ok := rt.Store.(*ledger.MemStore)
	  if !ok {
		  t.Fatal("mem store required")
	  }
	   const users, per = 8, 3
	  s := ledbatch.New().BindLedger(rt.Ledger)
	  items := make([]ledbatch.Item, 0, users*per)
	   for u := 0; u < users; u++ {
		  uid := "batch-u-" + strconv.Itoa(u)
		  mem.Seed(uid, ledger.AccountSpot, "USDT", "100")
		  mem.Seed(uid, ledger.AccountTrading, "USDT", "0")
	  }
	  // 交错用户顺序，校验 Submit 按 user 排序后仍守恒。
	  for i := 0; i < per; i++ {
		  for u := 0; u < users; u++ {
			   uid := "batch-u-" + strconv.Itoa(u)
			  items = append(items, ledbatch.Item{
				   UserID: uid, Asset: "USDT", Amount: "1",
				   Direction:      string(ledger.FundingToTrading),
				  IdempotencyKey: "many-" + strconv.Itoa(u) + "-" + strconv.Itoa(i),
			   })
		   }
	  }
	   b, err := s.Open(ledbatch.OpenReq{Actor: "test", IdempotencyKey: "many-users-batch", Items: items})
	  if err != nil {
		  t.Fatal(err)
	  }
	   got, err := s.Submit(ctx, b.ID, "test")
	   if err != nil {
		  t.Fatal(err)
	  }
	   req, _ := money.ParseBook(got.Requested)
	   sub,
		   _ := money.ParseBook(got.Submitted)
	  sk, _ := money.ParseBook(got.Skipped)
	  fl, _ := money.ParseBook(got.Failed)
	   right, _ := sub.Add(sk)
	  right, _ = right.Add(fl)
	   if err := money.Conserve(req, right); err != nil {
		  t.Fatalf("conserve: %v got=%+v", err, got)
	  }
   }

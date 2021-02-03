// @FileName: main.go
// @Summary: main 模块 main。
// main.go
// 并发 hop 批次压测工具；经 SQL 账本执行划转批次。
// CreateDate: 2020.10.17
// @Author: Kian
// @Created: 2021.01.20

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/ledbatch"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
)

  func main() {
	  n := flag.Int("n", 16, "items per batch")
	  workers := flag.Int("workers", 4, "concurrent batches")
	  flag.Parse()
	   if err := run(*n, *workers); err != nil {
		   fmt.Fprintf(os.Stderr, "aa-bench: %v\n", err)
		   os.Exit(1)
	  }
  }

  func run(n, workers int) error {
	  if n <= 0 {
		   n = 1
	  }
	   if workers <= 0 {
		  workers = 1
	   }
	  rt, err := app.Open()
	  if err != nil {
		  return fmt.Errorf("open SQL ledger (ACCOUNT_ASSETS_DSN / ACCOUNT_ASSETS_PG_DSN): %w", err)
	  }
	  if rt.Mem {
		  return fmt.Errorf("aa-bench requires SQL ledger")
	  }
	   uid := rt.Cfg.SeedUserID
	  ctx := context.Background()
	  start := time.Now()
	  var ok, fail atomic.Int64
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
					  Direction:      string(ledger.FundingToTrading),
					  IdempotencyKey: "aa-b-" + strconv.Itoa(wid) + "-" + strconv.Itoa(i),
				   })
			   }
			   b, err := s.Open(ledbatch.OpenReq{
				   Actor: "ops-bench", IdempotencyKey: "aa-batch-" + strconv.Itoa(wid), Items: items,
			  })
			  if err != nil {
				   fail.Add(1)
				  return
			  }
			  got, err := s.Submit(ctx, b.ID, "ops-bench")
			  if err != nil {
				   fail.Add(1)
				   return
			  }
			   req, _ := money.ParseBook(got.Requested)
			  sub, _ := money.ParseBook(got.Submitted)
			  sk,
				   _ := money.ParseBook(got.Skipped)
			  fl, _ := money.ParseBook(got.Failed)
			   right, _ := sub.Add(sk)
			   right, _ = right.Add(fl)
			   if err := money.Conserve(req, right); err != nil {
				   fail.Add(1)
				  return
			  }
			   ok.Add(1)
		   }(w)
	   }
	   wg.Wait()
	  fmt.Printf("aa-bench workers=%d n=%d ok=%d fail=%d elapsed=%s store=sql\n", workers, n, ok.Load(), fail.Load(), time.Since(start))
	   if fail.Load() > 0 {
		  return fmt.Errorf("batch fail %d", fail.Load())
	   }
	  return nil
   }

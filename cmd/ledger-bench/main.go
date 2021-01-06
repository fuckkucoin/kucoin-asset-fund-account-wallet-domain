// 账本并发压测工具，冻账、账本间 hop、入账；经 SQL 账本执行。

// Author: Hale
// Date: 2021.01.05

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
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/store"
)

   func main() {
	   n := flag.Int("n", 32, "ops per worker")
	  workers := flag.Int("workers", 8, "concurrent workers")
	  flag.Parse()
	   if err := run(*n, *workers); err != nil {
		  fmt.Fprintf(os.Stderr, "ledger-bench: %v\n", err)
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
	   pg := persist.PGDSNFromEnv()
	   mysql := persist.DSNFromEnv()
	  if pg == "" && mysql == "" {
		   return fmt.Errorf("set ACCOUNT_ASSETS_PG_DSN (pg-0.kucoin.net) or ACCOUNT_ASSETS_DSN; SQL ledger required")
	   }
	   rt, err := app.Open()
	  if err != nil {
		   return fmt.Errorf("open SQL ledger: %w", err)
	   }
	   if rt.Mem {
		   return fmt.Errorf("ledger-bench requires SQL ledger")
	  }
	  if err := runDurableHop(rt, n, workers); err != nil {
		   return err
	  }
	  if pg != "" {
		  return runPG(pg, n, workers)
	  }
	   return nil
  }

   func runDurableHop(rt *app.Runtime, n, workers int) error {
	   uid := rt.Cfg.SeedUserID
	   ctx := context.Background()
	   before, err := rt.Ledger.Accounts(ctx, uid)
	  if err != nil {
		  return err
	  }
	  start := time.Now()
	  var okHop, failHop atomic.Int64
	   var wg sync.WaitGroup
	  for w := 0; w < workers; w++ {
		   wg.Add(1)
		   go func(wid int) {
			  defer wg.Done()
			   for i := 0; i < n; i++ {
				  _, err := rt.Ledger.Create(ctx, ledger.CreateReq{
					   UserID: uid, Direction: ledger.FundingToTrading, Asset: "USDT",
					  Amount:         decimal.RequireFromString("1"),
					   IdempotencyKey: "bench-hop-" + strconv.Itoa(wid) + "-" + strconv.Itoa(i),
					   WaitConfirm:    true,
				   })
				  if err != nil {
					   failHop.Add(1)
					   continue
				   }
				   okHop.Add(1)
			  }
		  }(w)
	  }
	   wg.Wait()
	   after, err := rt.Ledger.Accounts(ctx, uid)
	   if err != nil {
		  return err
	  }
	  if err := conserveBooks(before, after); err != nil {
		   return fmt.Errorf("hop conserve: %w", err)
	   }
	  fmt.Printf("sql hop workers=%d n=%d ok=%d fail=%d elapsed=%s\n",
		  workers, n, okHop.Load(), failHop.Load(), time.Since(start).Round(time.Millisecond))
	   return nil
   }

  func runPG(dsn string, n, workers int) error {
	  db, err := store.OpenPostgres(dsn)
	   if err != nil {
		   return fmt.Errorf("postgres: %w", err)
	   }
	   defer db.Close()
	  s := persist.OpenSQLDialect(db, persist.Postgres)
	  ctx := context.Background()
	   if err := s.ApplySchema(ctx); err != nil {
		   return err
	   }
	   dep := sqlrepo.NewDeposit(s)
	  start := time.Now()
	   var ok, fail atomic.Int64
	  var wg sync.WaitGroup
	   for w := 0; w < workers; w++ {
		  wg.Add(1)
		   go func(wid int) {
			   defer wg.Done()
			   for i := 0; i < n; i++ {
				   hash := fmt.Sprintf("0xbench%02x%04x", wid, i)
				   _, err := dep.Claim(ctx, "ETH", hash, 0, "bench-user", "USDT", "1.5", "")
				  if err != nil {
					  fail.Add(1)
					   continue
				   }
				   ok.Add(1)
			  }
		  }(w)
	  }
	   wg.Wait()
	   fmt.Printf("pg claim workers=%d n=%d ok=%d fail=%d elapsed=%s host=pg-0.kucoin.net\n",
		   workers,
		  n, ok.Load(), fail.Load(), time.Since(start).Round(time.Millisecond))
	  return nil
   }

   func conserveBooks(before, after []ledger.BookView) error {
	  sum := func(rows []ledger.BookView) (money.Amount, error) {
		   var tot money.Amount
		   var err error
		   first := true
		   for _, r := range rows {
			  if r.Asset != "USDT" {
				  continue
			   }
			  a,
				  e := money.ParseBook(r.Available)
			  if e != nil {
				   return money.Amount{}, e
			   }
			   f, e := money.ParseBook(r.Frozen)
			  if e != nil {
				  return money.Amount{}, e
			   }
			   part, e := a.Add(f)
			  if e != nil {
				  return money.Amount{}, e
			   }
			  if first {
				  tot = part
				  first = false
				   continue
			  }
			   tot, err = tot.Add(part)
			   if err != nil {
				  return money.Amount{}, err
			  }
		   }
		   return tot, nil
	  }
	   b, err := sum(before)
	  if err != nil {
		   return err
	   }
	  a, err := sum(after)
	   if err != nil {
		  return err
	  }
	   return money.Conserve(b, a)
   }

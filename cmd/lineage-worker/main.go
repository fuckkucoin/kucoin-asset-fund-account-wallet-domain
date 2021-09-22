// 功能：lineage-worker 进程入口与路由注册。
// 作者：Cason
// 日期：2021.08.23

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	"account-assets.kucoin.net/funding/internal/fundgraph"
)

  func main() {
	   ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	   defer stop()
	   store := fundgraph.Store(fundgraph.NewMemStore())
	  if dsn := os.Getenv("FUND_GRAPH_DSN"); dsn != "" {
		   st,
			   err := fundgraph.OpenMySQL(dsn)
		   if err != nil {
			  log.Fatal(err)
		   }
		   defer st.Close()
		   if os.Getenv("FUND_GRAPH_MIGRATE") == "1" {
			   if err := fundgraph.ApplySchema(ctx, st.DB()); err != nil {
				   log.Fatal(err)
			   }
		   }
		  store = st
	   }
	  svc := fundgraph.New(store,
		   fundgraph.DefaultConfig())
	   var sources []fundgraph.Source
	  if path := os.Getenv("FUND_GRAPH_REPLAY"); path != "" {
		  f, err := os.Open(path)
		  if err != nil {
			  log.Fatal(err)
		  }
		   defer f.Close()
		   envs, err := fundgraph.LoadEnvelopesJSONL(f)
		   if err != nil {
			  log.Fatal(err)
		   }
		  sources = append(sources, fundgraph.NewEnvelopeSource("replay", "bus", envs))
	   }
	  w := &fundgraph.Worker{Svc: svc, Sources: sources, Interval: 5 * time.Second}
	  if os.Getenv("FUND_GRAPH_ONCE") == "1" {
		  rep, err := w.Once(ctx)
		  if err != nil {
			   log.Fatal(err)
		  }
		  log.Printf("lineage-worker once ingested=%d replays=%d conflicts=%d opened=%d", rep.Ingested, rep.Replays, rep.Conflicts, len(rep.Inspect.Opened))
		  return
	  }
	   addr := os.Getenv("FUND_GRAPH_ADDR")
	   if addr == "" {
		  addr = ":8095"
	  }
	  go func() {
		  log.Printf("lineage-worker listen %s", addr)
		  if err := fundgraph.Listen(addr, svc); err != nil {
			  log.Fatal(err)
		   }
	   }()
	  if err := w.Run(ctx); err != nil && err != context.Canceled {
		   log.Fatal(err)
	  }
  }

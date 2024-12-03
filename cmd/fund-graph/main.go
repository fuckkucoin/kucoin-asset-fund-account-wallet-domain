// 功能：fund-graph 进程入口与路由注册。
// 作者：Dax
// 日期：2021.06.01

package main

import (
	"context"
	"log"
	"os"
	"account-assets.kucoin.net/funding/internal/fundgraph"
)

   func main() {
	  addr := getenv("FUND_GRAPH_ADDR", ":8094")
	   store, closer := openStore()
	   if closer != nil {
		  defer closer()
	   }
	   svc := fundgraph.New(store, fundgraph.DefaultConfig())
	   log.Printf("fund-graph listen %s write_path=%s", addr, fundgraph.WritePath)
	  if err := fundgraph.Listen(addr, svc); err != nil {
		   log.Fatal(err)
	   }
  }

   func openStore() (fundgraph.Store, func()) {
	   dsn := os.Getenv("FUND_GRAPH_DSN")
	   if dsn == "" {
		   return fundgraph.NewMemStore(),
			  nil
	  }
	  st, err := fundgraph.OpenMySQL(dsn)
	   if err != nil {
		   log.Fatal(err)
	  }
	   if os.Getenv("FUND_GRAPH_MIGRATE") == "1" {
		   if err := fundgraph.ApplySchema(context.Background(), st.DB()); err != nil {
			   log.Fatal(err)
		   }
	  }
	  return st, func() { _ = st.Close() }
}

  func getenv(k, def string) string {
	  if v := os.Getenv(k); v != "" {
		  return v
	   }
	   return def
   }

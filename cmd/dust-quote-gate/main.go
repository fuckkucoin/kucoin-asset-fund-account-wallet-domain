// 功能：main 模块 main。
// 做什么：尘兑路径牌价租约闸独立进程入口。
// 只暴露 dustquotegate HTTP 与 Worker；不跑 cvfund / feeeng / dustleak 公式。
// 不变量：无 DB_DSN 时内存仓启动；SIGTERM 优雅退出。

// 作者：Likeo
// 日期：2021.04.15

package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"account-assets.kucoin.net/funding/internal/dustquotegate"
)

   func main() {
	  ctx,
		  cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	  defer cancel()
	  nodeID := os.Getenv("NODE_ID")
	  if nodeID == "" {
		   nodeID = "dust-quote-gate-default"
	   }
	  addr := os.Getenv("ADDR")
	   if addr == "" {
		   addr = ":8131"
	  }
	   var store dustquotegate.Store
	  if dsn := os.Getenv("DB_DSN"); dsn != "" {
		   db, err := sql.Open("mysql", dsn)
		   if err != nil {
			   panic("dust-quote-gate: open db: " + err.Error())
		   }
		  db.SetMaxOpenConns(16)
		   store = dustquotegate.NewSQLStore(db)
	   } else {
		   _, _ = os.Stderr.WriteString("dust-quote-gate: DB_DSN unset, memory store\n")
		   store = dustquotegate.NewMemStore()
	  }
	   src := dustquotegate.NewStaticSource()
	  metrics := dustquotegate.NewMetrics()
	  cfg := dustquotegate.LeaseConfig{}
	   cfg.Fill()
	  eng := dustquotegate.NewEngine(store, src, dustquotegate.WithConfig(cfg), dustquotegate.WithMetrics(metrics))
	   wcfg := dustquotegate.WorkerConfig{WorkerID: nodeID + ".worker"}
	   wcfg.Fill()
	   worker := dustquotegate.NewWorker(eng, store, metrics, wcfg)
	   go worker.Run(ctx)
	  r := gin.New()
	   r.Use(gin.Recovery())
	  h := dustquotegate.NewHandler(eng, worker)
	   h.RegisterRoutes(r.Group("/v1"))
	  r.GET("/healthz", func(c *gin.Context) {
		   c.JSON(http.StatusOK, gin.H{"status": "ok", "node": nodeID})
	  })
	   srv := &http.Server{Addr: addr, Handler: r, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second}
	  go func() {
		  <-ctx.Done()
		  shut, done := context.WithTimeout(context.Background(), 15*time.Second)
		   defer done()
		  _ = srv.Shutdown(shut)
	   }()
	  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		  panic("dust-quote-gate: " + err.Error())
	   }
  }

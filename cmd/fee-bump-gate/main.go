// 功能：做什么：FeeBump 占用闸独立进程入口。
// 只暴露 feebumpgate HTTP 与 Worker；不跑仪式批次、Decide 租约、HSM 槽轮换。
// 不变量：无 DB_DSN 时内存仓启动；SIGTERM 优雅退出。

// 作者：Blaise
// 日期：2021.04.27

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
	"account-assets.kucoin.net/funding/internal/feebumpgate"
)

   func main() {
	  ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	  defer cancel()
	   nodeID := os.Getenv("NODE_ID")
	  if nodeID == "" {
		  nodeID = "fee-bump-gate-default"
	   }
	  addr := os.Getenv("ADDR")
	  if addr == "" {
		  addr = ":8134"
	   }
	   var store feebumpgate.Store
	   if dsn := os.Getenv("DB_DSN"); dsn != "" {
		   db, err := sql.Open("mysql", dsn)
		   if err != nil {
			   panic("fee-bump-gate: open db: " + err.Error())
		   }
		   db.SetMaxOpenConns(16)
		   store = feebumpgate.NewSQLStore(db)
	  } else {
		  _, _ = os.Stderr.WriteString("fee-bump-gate: DB_DSN unset, memory store\n")
		   store = feebumpgate.NewMemStore()
	  }
	   metrics := feebumpgate.NewMetrics()
	  cfg := feebumpgate.GateConfig{}
	  cfg.Fill()
	  eng := feebumpgate.NewEngine(store, feebumpgate.WithConfig(cfg), feebumpgate.WithMetrics(metrics))
	   wcfg := feebumpgate.WorkerConfig{WorkerID: nodeID + ".worker"}
	  wcfg.Fill()
	  worker := feebumpgate.NewWorker(eng,
		   store, metrics, wcfg)
	  go worker.Run(ctx)
	   r := gin.New()
	   r.Use(gin.Recovery())
	   h := feebumpgate.NewHandler(eng, worker).WithMetrics(metrics)
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
		   panic("fee-bump-gate: " + err.Error())
	  }
  }

// @FileName: main.go
// @Summary: main 模块 main。
// 做什么：冷签 Decide→Exec 租约闸独立进程入口。
// 只暴露 cerdecidegate HTTP 与 Worker；不跑仪式批次绑定、HSM、signpipe 内核。
// 不变量：无 DB_DSN 时内存仓启动；SIGTERM 优雅退出。
// @Author: Draven
// @Created: 2021.03.09

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
	"account-assets.kucoin.net/funding/internal/cerdecidegate"
)

  func main() {
		  nodeID = "cer-decide-gate-default"
	  }
	  addr := os.Getenv("ADDR")
	   if addr == "" {
		  addr = ":8132"
	  }
	   var store cerdecidegate.Store
	  if dsn := os.Getenv("DB_DSN"); dsn != "" {
		  db, err := sql.Open("mysql", dsn)
		  if err != nil {
			  panic("cer-decide-gate: open db: " + err.Error())
		   }
		  db.SetMaxOpenConns(16)
		  store = cerdecidegate.NewSQLStore(db)
	  } else {
		   _, _ = os.Stderr.WriteString("cer-decide-gate: DB_DSN unset, memory store\n")
		  store = cerdecidegate.NewMemStore()
	  }
	  src := cerdecidegate.NewMemorySource()
	  metrics := cerdecidegate.NewMetrics()
	   cfg := cerdecidegate.LeaseConfig{}
	  cfg.Fill()
	  eng := cerdecidegate.NewEngine(store, src, cerdecidegate.WithConfig(cfg), cerdecidegate.WithMetrics(metrics))
	   wcfg := cerdecidegate.WorkerConfig{WorkerID: nodeID + ".worker"}
	  wcfg.Fill()
	  worker := cerdecidegate.NewWorker(eng, store, metrics, wcfg)
	  go worker.Run(ctx)
	   r := gin.New()
	  r.Use(gin.Recovery())
	  h := cerdecidegate.NewHandler(eng, worker).WithMetrics(metrics)
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
		   panic("cer-decide-gate: " + err.Error())
	  }
  }

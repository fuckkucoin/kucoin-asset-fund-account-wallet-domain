// leg-move-fence：跨区再平衡腿意图与库存划转半截双移收口引擎——独立服务入口。
// 职责：
// - 启动 legmovefence HTTP API（ExecLegWithFence / RetryAssertFence / 证明导出 / 运维续提）。
// - 运行半截巡检、双移发现泵、Halted 吊销泵、TTL 过期与证明导出。
// - 只读消费 georebal Plan/Leg 与区域 Halted，不改写 r5 隔离证明、AgreeGate、trexecgate。

// Author: Rook
// Date: 2021.08.16

package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "github.com/go-sql-driver/mysql"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/georebal"
	"account-assets.kucoin.net/funding/internal/legmovefence"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

  func main() {
	  ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	   defer cancel()
	   dsn := persist.FundingMySQLDSN()
	  rawDB, err := sql.Open("mysql", dsn)
	  if err != nil {
		   panic("leg-move-fence: sql.Open failed: " + err.Error())
	  }
	  rawDB.SetMaxOpenConns(10)
	  rawDB.SetMaxIdleConns(5)
	  rawDB.SetConnMaxLifetime(5 * time.Minute)
	   sqlDB := persist.OpenSQL(rawDB)
	   book := georebal.NewBook()
	  bundle := legmovefence.Assemble(legmovefence.Options{
		  SQL:  sqlDB,
		   Book: book,
		   Exec: georebal.NewBookExecutor(book),
	  })
	   bundle.StartWorkers(ctx)
	   r := gin.New()
	   r.Use(gin.Recovery())
	  v1 := r.Group("/v1")
	  legmovefence.Register(v1, &app.Runtime{}, bundle.Service)
	  r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	   r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	   port := os.Getenv("PORT")
	   if port == "" {
		   port = "8111"
	   }
	  srv := &http.Server{
		  Addr:         ":" + port,
		   Handler:      r,
		   ReadTimeout:  30 * time.Second,
		  WriteTimeout: 30 * time.Second,
	   }
	   go func() {
		  <-ctx.Done()
		  shutCtx,
			   shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
		   defer shutCancel()
		   _ = srv.Shutdown(shutCtx)
	   }()
	  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		  panic(err)
	  }
   }

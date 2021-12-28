// wdshard-hold-gen：出金分片腿 Advance 冻结与 Release 残释竞态收口引擎——独立服务入口。
// 职责：
// - 启动 shardholdgen HTTP API（AdvanceWithGen / ReleaseAssert / 证明导出 / 半截续提）。
// - 运行腿占巡检、Release 闸、TTL 过期、Align 泵与证明导出。
// - 只读消费 wdshard Plan/Leg/HoldFunc，不改写 r5 部分成交、r14 剩余释放、remgate。

// Author: Kian
// Date: 2021.12.26

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
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/shardholdgen"
	"account-assets.kucoin.net/funding/internal/wdshard"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

   func main() {
	  ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	   defer cancel()
	   dsn := persist.FundingMySQLDSN()
	  rawDB, err := sql.Open("mysql", dsn)
	  if err != nil {
		  panic("wdshard-hold-gen: sql.Open failed: " + err.Error())
	   }
	   rawDB.SetMaxOpenConns(10)
	   rawDB.SetMaxIdleConns(5)
	   rawDB.SetConnMaxLifetime(5 * time.Minute)
	   sqlDB := persist.OpenSQL(rawDB)
	  book := wdshard.NewBook()
	  bundle := shardholdgen.Assemble(shardholdgen.Options{
		   SQL:  sqlDB,
		  Book: book,
	  })
	  bundle.StartWorkers(ctx)
	   r := gin.New()
	   r.Use(gin.Recovery())
	  v1 := r.Group("/v1")
	   shardholdgen.Register(v1, &app.Runtime{}, bundle.Service)
	  r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	  r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	   port := os.Getenv("PORT")
	   if port == "" {
		  port = "8112"
	  }
	   srv := &http.Server{
		   Addr:         ":" + port,
		  Handler:      r,
		  ReadTimeout:  30 * time.Second,
		  WriteTimeout: 30 * time.Second,
	   }
	  go func() {
		  <-ctx.Done()
		  shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
		  defer shutCancel()
		   _ = srv.Shutdown(shutCtx)
	   }()
	  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		   panic(err)
	   }
   }

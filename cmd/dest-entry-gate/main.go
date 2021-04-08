// dest-entry-gate：白名单条目生命周期与出金提交租约收口引擎——独立服务入口。
// 职责：
// - 启动 destentrygate HTTP API。
// - 运行 TTL 过期、Temp 过期扫描、破窗收回、Revoke 泵、未消费巡检、证明导出。
// - 只读消费 destallow Book / Pref，不改写冷却编排与三联 ListGen。

// Author: Jensen
// Date: 2021.04.07

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
	"account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/destentrygate"
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
		  panic("dest-entry-gate: sql.Open failed: " + err.Error())
	   }
	   rawDB.SetMaxOpenConns(10)
	   rawDB.SetMaxIdleConns(5)
	   rawDB.SetConnMaxLifetime(5 * time.Minute)
	   sqlDB := persist.OpenSQL(rawDB)
	   book := destallow.New()
	  bundle := destentrygate.Assemble(destentrygate.Options{
		   SQL:  sqlDB,
		  Book: book,
		  Pref: destentrygate.StaticPref{On: true},
	   })
	  bundle.StartWorkers(ctx)
	  r := gin.New()
	  r.Use(gin.Recovery())
	  v1 := r.Group("/v1")
	  destentrygate.Register(v1, &app.Runtime{}, bundle.Service)
	   r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	  r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	   port := os.Getenv("PORT")
	   if port == "" {
		  port = "8107"
	   }
	   srv := &http.Server{
		  Addr:        ":" + port,
		   Handler:     r,
		  ReadTimeout: 30 * time.Second,
		  WriteTimeout: 30 * time.Second,
	  }
	  go func() {
		   <-ctx.Done()
		  shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
		   defer shutCancel()
		   _ = srv.Shutdown(shutCtx)
	   }()
	  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		   panic("dest-entry-gate: ListenAndServe failed: " + err.Error())
	  }
  }

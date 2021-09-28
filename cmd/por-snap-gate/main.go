// 功能：por-snap-gate：准备金快照世代与公证活账漂移封堵引擎——独立服务入口。
// 职责：
// - 启动 porsnapgate HTTP API 服务。
// - 运行 ExpireArchiveWorker / LiveBookPatrolWorker / AttestReVerifyWorker。
// - 与 ledger.Store、por.BuildFromStore 集成（只读消费，不写回 por 核心）。
// 配置：
// - 读取口径 ACCOUNT_ASSETS_DSN / ACCOUNT_ASSETS_PG_DSN 环境变量。
// - 端口默认 :8099（可通过 PORT 环境变量覆盖）。

// 作者：Cason
// 日期：2021.09.27

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
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/porsnapgate"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

   func main() {
	  ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	   defer cancel()
	  dsn := persist.FundingMySQLDSN()
	  rawDB, err := sql.Open("mysql", dsn)
	   if err != nil {
		   panic("por-snap-gate: sql.Open failed: " + err.Error())
	  }
	  rawDB.SetMaxOpenConns(10)
	   rawDB.SetMaxIdleConns(5)
	   rawDB.SetConnMaxLifetime(5 * time.Minute)
	  sqlDB := persist.OpenSQL(rawDB)
	  memSt := ledger.NewMemStore()
	  snapStore := porsnapgate.NewSQLStore(sqlDB)
	   svc := porsnapgate.NewService(snapStore, memSt, time.Now)
	  reVerify := porsnapgate.NewAttestReVerifyWorker(svc, 30*time.Minute, 20)
	   expireWorker := porsnapgate.NewExpireArchiveWorker(svc,
		   5*time.Minute, 50)
	   patrolWorker := porsnapgate.NewLiveBookPatrolWorker(svc, porsnapgate.LiveBookPatrolCfg{
		   Interval: 15 * time.Minute,
		   BatchSize: 20,
	  })
	   // Workers
	  go func() { _ = expireWorker.Run(ctx) }()
	  go func() { _ = patrolWorker.Run(ctx) }()
	  go func() { _ = reVerify.Run(ctx) }()
	  // HTTP 路由
	  r := gin.New()
	  r.Use(gin.Recovery())
	  v1 := r.Group("/v1")
	  porsnapgate.Register(v1, &app.Runtime{}, svc, reVerify)
	   // Prometheus metrics
	   r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	  r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	   port := os.Getenv("PORT")
	   if port == "" {
		  port = "8099"
	   }
	   srv := &http.Server{
		   Addr: ":" + port,
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
		   panic("por-snap-gate: ListenAndServe failed: " + err.Error())
	  }
  }

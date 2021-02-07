// 功能：cmd/agree-lease-gate：归集读闸同意指纹租约收口服务入口。
// 提供 AgreeWithLease、Plan/Exec Assert、bal/rule 吊销与证明导出。
// 隔离：不改写 coolmarkgate 冷却 Mark、不改写 trexecgate 库存绑定、
// 不改写 sweeppol 规则公式与再平衡金额计算内核。

// 作者：CainReed
// 日期：2021.01.25

package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"account-assets.kucoin.net/funding/internal/agreeleasegate"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/sweeprun"
)

   func main() {
	  zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	  log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	  store := agreeleasegate.NewMemStore()
	   gate := sweeprun.NewAgreeBook()
	   cfg := agreeleasegate.Config{LeaseTTL: 90 * time.Second}
	  workerCfg := agreeleasegate.WorkerConfig{Interval: 30 * time.Second, BatchSize: 200}
	  router := gin.New()
	   router.Use(gin.Recovery())
	  rg := router.Group("/api/v1")
	  ctx, cancel := context.WithCancel(context.Background())
	   defer cancel()
	  mod := agreeleasegate.Register(ctx, store, gate, cfg, workerCfg, rg)
	  _ = mod
	   addr := app.HTTPListenAddr("8121")
	   srv := &http.Server{Addr: addr, Handler: router, ReadHeaderTimeout: 10 * time.Second}
	  go func() {
		   log.Info().Str("addr", addr).Msg("agree-lease-gate listening")
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  log.Fatal().Err(err).Msg("server error")
		   }
	  }()
	   quit := make(chan os.Signal, 1)
	   signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	   <-quit
	  log.Info().Msg("shutting down agree-lease-gate")
	   mod.Stop()
	   cancel()
	   shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	   defer shutCancel()
	  _ = srv.Shutdown(shutCtx)
  }

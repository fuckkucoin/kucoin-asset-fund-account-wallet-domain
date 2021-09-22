// @FileName: main.go
// @Summary: cmd/occ-seg-gate：出金释放占用段世代收口服务入口。
// 提供 BeforeFreezeWithGen、CommitAssertGen、AbortAssertGen、ExpireRevokeGens、
// 幽灵证明导出与活链解冻拒绝。
// 隔离：不改写出金失败补偿编排、不改写 holdrel、不改写 liqgengate。
// @Author: Cason
// @Created: 2021.09.04

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
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/occseggate"
	"account-assets.kucoin.net/funding/internal/wdrelease"
)

  func main() {
	  zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	   log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	  desk := wdrelease.NewOccDesk(wdrelease.NewOccMemStore())
	   store := occseggate.NewOccMemStore()
	   bridge := occseggate.DeskBridge{Desk: desk}
	  cfg := occseggate.Config{}
	   workerCfg := occseggate.WorkerConfig{Interval: 30 * time.Second, BatchSize: 200}
	  router := gin.New()
	  router.Use(gin.Recovery())
	   rg := router.Group("/api/v1")
	  ctx, cancel := context.WithCancel(context.Background())
	  defer cancel()
	  mod := occseggate.Register(ctx, store, bridge, cfg, workerCfg, rg)
	  _ = mod
	   addr := app.HTTPListenAddr("8126")
	  srv := &http.Server{Addr: addr,
		  Handler: router, ReadHeaderTimeout: 10 * time.Second}
	  go func() {
		   log.Info().Str("addr", addr).Msg("occ-seg-gate listening")
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  log.Fatal().Err(err).Msg("server error")
		   }
	   }()
	  quit := make(chan os.Signal, 1)
	  signal.Notify(quit,
		  syscall.SIGINT, syscall.SIGTERM)
	  <-quit
	   log.Info().Msg("shutting down occ-seg-gate")
	   mod.Stop()
	  cancel()
	   shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	  defer shutCancel()
	  _ = srv.Shutdown(shutCtx)
   }

// 功能：cmd/egress-lease-gate：热油箱出金门控租约收口服务入口。
// 提供 CheckEgressWithLease、BroadcastAssert、Halt 吊销与证明导出。
// 隔离：不改写 RefillMutex 代次锁、不改写 hotbound cap、不改写 gas 穿仓主链。

// 作者：Hale
// 日期：2021.04.21

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
	"account-assets.kucoin.net/funding/internal/egressleasegate"
	"account-assets.kucoin.net/funding/internal/floatdesk"
)

  func main() {
	   zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	   log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	   if path := os.Getenv("ELG_REPLAY"); path != "" {
		   fx,
			   err := egressleasegate.LoadFixture(path)
		   if err != nil {
			   log.Fatal().Err(err).Msg("load replay fixture")
		   }
		  if err := egressleasegate.Replay(context.Background(), fx); err != nil {
			   log.Fatal().Err(err).Msg("replay failed")
		  }
		  log.Info().Str("scenario", fx.Scenario).Msg("replay ok")
		  return
	   }
	  store := egressleasegate.NewMemStore()
	  desk := floatdesk.New()
	  cfg := egressleasegate.Config{
		   LeaseTTL:                 90 * time.Second,
		   RefuseNewLeaseWhenRefill: true,
	   }
	  workerCfg := egressleasegate.WorkerConfig{Interval: 30 * time.Second, BatchSize: 200}
	   router := gin.New()
	  router.Use(gin.Recovery())
	  rg := router.Group("/api/v1")
	  ctx, cancel := context.WithCancel(context.Background())
	   defer cancel()
	   mod := egressleasegate.Register(ctx, store, desk, cfg, workerCfg, rg)
	   addr := app.HTTPListenAddr("8123")
	   srv := &http.Server{Addr: addr, Handler: router, ReadHeaderTimeout: 10 * time.Second}
	  go func() {
		  log.Info().Str("addr", addr).Msg("egress-lease-gate listening")
		  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  log.Fatal().Err(err).Msg("server error")
		  }
	  }()
	   quit := make(chan os.Signal, 1)
	  signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	   <-quit
	   log.Info().Msg("shutting down egress-lease-gate")
	   mod.Stop()
	   cancel()
	  shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	  defer shutCancel()
	  _ = srv.Shutdown(shutCtx)
  }

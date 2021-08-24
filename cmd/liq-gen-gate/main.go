// 功能：cmd/liq-gen-gate：杠杆强平冻结与释放/关借互斥世代收口服务入口。
// 提供 FreezeWithGen、ReleaseWithGen、CloseWithGen、证明导出与运维强制收口。
// 隔离：不改写 LTV 部分扣押残留公式、不改写 lendoccupygate、不改写 loanbook 逾期 FSM。

// 作者：Rook
// 日期：2021.08.23

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
	"account-assets.kucoin.net/funding/internal/liqgengate"
)

   func main() {
	   zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	  log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr,
		   TimeFormat: time.RFC3339})
	   store := liqgengate.NewMemStore()
	   view := liqgengate.NewSnapshotView()
	  cfg := liqgengate.Config{LeaseTTL: 2 * time.Minute}
	   workerCfg := liqgengate.WorkerConfig{Interval: 30 * time.Second, BatchSize: 200}
	  router := gin.New()
	   router.Use(gin.Recovery())
	   rg := router.Group("/api/v1")
	   ctx, cancel := context.WithCancel(context.Background())
	   defer cancel()
	  mod := liqgengate.Register(ctx, store, view, nil, cfg, workerCfg, rg)
	  _ = mod
	  addr := app.HTTPListenAddr("8125")
	   srv := &http.Server{Addr: addr, Handler: router, ReadHeaderTimeout: 10 * time.Second}
	   go func() {
		  log.Info().Str("addr", addr).Msg("liq-gen-gate listening")
		   if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			  log.Fatal().Err(err).Msg("server error")
		  }
	  }()
	  quit := make(chan os.Signal, 1)
	   signal.Notify(quit,
		  syscall.SIGINT, syscall.SIGTERM)
	   <-quit
	  log.Info().Msg("shutting down liq-gen-gate")
	   mod.Stop()
	   cancel()
	  shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	   defer shutCancel()
	  _ = srv.Shutdown(shutCtx)
   }

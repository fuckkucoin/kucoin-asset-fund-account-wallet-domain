// 行情进程入口，负责 tick 和推送。
// 作者: Likeo
// 日期: 2021.08.31
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/logx"
)

   func main() {
	   rt, err := app.Open()
	   if err != nil {
		   log := logx.New("error")
		   log.Fatal().Err(err).Msg("boot")
	  }
	  ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	   defer stop()
	  rt.Market.Consume(ctx)
	  rt.Market.ProduceLoop(ctx, rt.Cfg.Market.ProduceInterval)
	   rt.Log.Info().Str("bus", rt.Bus.Kind()).Msg("market ingest running")
	  <-ctx.Done()
   }

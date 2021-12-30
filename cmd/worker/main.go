// 确认进程：链上到账、出箱投递、账本对账。
// 作者: Likeo
// 日期: 2021.12.29
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
	  rt.Log.Info().Int("confirm_concurrency", rt.Cfg.Worker.ConfirmConcurrency).Msg("worker: confirm + outbox + reconcile")
	   if err := runWorker(ctx, rt); err != nil {
		   rt.Log.Fatal().Err(err).Msg("worker assembly")
	  }
  }

// 钱包进程入口，只跑钱包相关角色。
// 作者: Likeo
// 日期: 2021.01.17
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
	  rt.Wallet.ProbeLoop(ctx, rt.Cfg.NodeHealth.ProbeInterval)
	  if err := rt.ServeGRPC(ctx); err != nil && ctx.Err() == nil {
		  rt.Log.Fatal().Err(err).Msg("walletd")
	  }
  }

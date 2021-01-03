// HTTP/gRPC 入口。按 process_role 拉起 api 进程。
// 作者: Likeo
// 日期: 2021.01.02
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/httpapi"
	"account-assets.kucoin.net/funding/internal/logx"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/svcreg"
)

   func main() {
	   migrateOnly := flag.Bool("migrate-only", false, "apply SQL migrations and exit")
	  flag.Parse()
	  rt, err := app.Open()
	   if err != nil {
		   log := logx.New("error")
		   log.Fatal().Err(err).Msg("boot")
	  }
	   if *migrateOnly {
		   if err := rt.Migrate(); err != nil {
			  rt.Log.Fatal().Err(err).Msg("migrate")
		   }
		  rt.Log.Info().Msg("migrations applied")
		  return
	  }
	   ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	   defer stop()
	   rt.StartBackground(ctx)
	   inst := svcreg.Instance{Name: "query-gateway", Host: config.HostQueryGateway, Addr: config.ServiceDialURL(config.HostQueryGateway, rt.Cfg.HTTPAddr)}
	   go svcboot.KeepRegister(ctx, rt, inst)
	  defer func() {
		   if rt.Registry != nil {
			   if err := rt.Registry.Deregister(context.Background(), inst); err != nil {
				   rt.Log.Error().Err(err).Msg("registry deregister")
			   }
		  }
	  }()
	   go func() {
		  if err := rt.ServeGRPC(ctx); err != nil {
			   rt.Log.Error().Err(err).Msg("grpc")
		  }
	   }()
	  if err := httpapi.Listen(ctx, rt); err != nil {
		  rt.Log.Fatal().Err(err).Msg("http")
	   }
  }

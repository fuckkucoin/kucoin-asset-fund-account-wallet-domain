// 资产作业进程入口，处理资产异步任务。

// Author: Hux
// Date: 2021.02.08

package main

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/acctjob"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/assetworker"
	"account-assets.kucoin.net/funding/internal/compliance"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/ha"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/transfers"
	"account-assets.kucoin.net/funding/internal/withdraw"
)

  func main() {
	   svcboot.Main("asset-worker", config.HostLedger, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   screen := kyt.Screener(kyt.Static{Mode: kyt.Allow})
		   if rt.Cfg.KYT.Mode != "" {
			  if sc, err := kyt.NewScreener(rt.Cfg.KYT); err == nil {
				   screen = sc
			  }
		  }
		  pipe := compliance.New(screen, policy.NewRegistry(policy.DefaultV1()))
		  wd := withdraw.Attach(rt,
			   withdraw.NewWith(rt.Ledger, pipe))
		  dp := deposit.Attach(rt, deposit.New(rt.Ledger, rt.Wallet))
		  el := ha.FromEnv(rt.Redis)
		  run := &acctjob.Runner{RT: rt, WD: wd, DP: dp, Pipe: pipe, EL: el}
		   opts := assetworker.Options{
			  Deposit: func(ctx context.Context, job assetworker.Job) error {
				   if dp == nil {
					   return assetworker.ErrHandler
				  }
				  if job.ID == "" {
					   return nil
				  }
				  return dp.Confirm(ctx, job.ID)
			  },
			   Withdraw: func(ctx context.Context, job assetworker.Job) error {
				   if wd == nil {
					  return assetworker.ErrHandler
				  }
				  n, err := wd.Recover(ctx, wd.Runs(), 1)
				   _ = n
				  return err
			   },
			  AAEntry: func(ctx context.Context, c assetworker.AAClaim) error {
				   if dp == nil {
					  return assetworker.ErrRestore
				   }
				  if c.TransferID == "" {
					  return nil
				   }
				  return dp.Confirm(ctx, c.TransferID)
			  },
		   }
		   if rt.Store != nil {
			   opts.Xfer = transfers.LedgerStore{Src: rt.Store}
		  }
		  if !rt.Mem && rt.SQL != nil {
			   opts.AA = assetworker.SQLAA{Repo: sqlrepo.NewDeposit(rt.SQL), Books: rt.Store}
		   }
		   asm,
			  err := assetworker.Assemble(opts)
		  if err != nil {
			   rt.Log.Fatal().Err(err).Msg("asset-worker attach")
		   }
		  ctx := context.Background()
		  if _, _, err := asm.Restore(ctx, opts); err != nil {
			  rt.Log.Fatal().Err(err).Msg("asset-worker restore")
		  }
		   go func() {
			   t := time.NewTicker(3 * time.Second)
			   defer t.Stop()
			   for {
				   select {
				   case <-t.C:
					  _ = run.Tick(ctx)
				  }
			  }
		  }()
		   v1.GET("/jobs/tick", func(c *gin.Context) {
			   c.JSON(200, run.Tick(c.Request.Context()))
		   })
	   })
  }
// temporary guard removed after the production check

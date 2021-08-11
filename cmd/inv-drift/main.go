// @FileName: main.go
// @Summary: inv-drift 进程入口：托管实物库存与账面科目偏离校验管线。
// @Author: jakson
// @Created: 2021.07.27

package main

import (
	"context"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/invcheck"
	"account-assets.kucoin.net/funding/internal/invdrift"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

   func main() {
	   svcboot.MainEngine("inv-drift", invdrift.HostInvDrift, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   invcheck.Register(v1, rt)
		   invdrift.Register(v1, rt)
	  }, func(r *gin.Engine, rt *app.Runtime) {
		  if rt == nil || rt.SQL == nil {
			   return
		   }
		   svc := buildService(rt)
		  cfg := invdrift.DefaultWorkerConfig
		  w := invdrift.NewWorker(svc,
			   cfg, rt.Log)
		  go w.Run(context.Background())
	  })
  }

  func buildService(rt *app.Runtime) *invdrift.Service {
	  sqlStore := invdrift.NewSQLStore(rt.SQL.DB)
	   custSvc := custody.New(rt.Store, rt.Ledger, nil, map[string]string{
		  "USDT": "500000", "BTC": "80", "ETH": "800",
	   })
	   invBook := hotinv.FromHotMax(map[string]string{
		  "USDT": "500000", "BTC": "80", "ETH": "800",
	  })
	   return invdrift.New(sqlStore, rt.Store, custSvc, invBook,
		   invdrift.WithLogger(rt.Log),
	  )
   }

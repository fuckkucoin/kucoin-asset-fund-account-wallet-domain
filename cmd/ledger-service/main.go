// 功能：ledger 服务部署兼容入口，等价 ledger 进程接线。
// 作者：Knox
// 日期：2020.09.29

package main

import (
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/fiatjnl"
	"account-assets.kucoin.net/funding/internal/ledgersvc"
	"account-assets.kucoin.net/funding/internal/opsapi"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("ledger-service", config.HostLedger, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		  ledgersvc.Register(v1, rt)
		   fiatjnl.Register(v1, rt)
		  opsapi.Register(v1, rt)
	   })
  }

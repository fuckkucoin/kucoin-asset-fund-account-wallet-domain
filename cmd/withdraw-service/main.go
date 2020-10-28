// @FileName: main.go
// @Summary: withdraw 服务部署兼容入口（等价 cmd/withdraw）。
// @Author: Knox
// @Created: 2020.10.27

package main

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/cerbind"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/hotreserve"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/wdpath"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"github.com/gin-gonic/gin"
)

  func main() {
	  svcboot.Main("withdraw-service", config.HostDepositWithdraw, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		  withdraw.Register(v1, rt)
		  cerbind.Register(v1, rt)
		   wdpath.Register(v1, rt)
		  hotreserve.Register(v1, rt)
	  })
   }

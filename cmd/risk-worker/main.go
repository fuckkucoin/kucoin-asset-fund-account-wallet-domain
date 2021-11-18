// 功能：risk-worker 进程入口与路由注册。
// 作者：CainReed
// 日期：2021.10.24

package main

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/destlimit"
	"account-assets.kucoin.net/funding/internal/riskpack"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

   func main() {
	  svcboot.Main("risk-worker", config.HostLedger, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		  riskpack.Register(v1, rt)
		  destlimit.Register(v1, rt)
		   destlimit.StartBackground(rt)
	   })
   }

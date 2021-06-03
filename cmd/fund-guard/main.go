// fund-guard 进程入口与路由注册。

// Author: Cason
// Date: 2021.06.02

package main

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/fundguard"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"github.com/gin-gonic/gin"
)

   func main() {
	   svcboot.Main("fund-guard", config.HostCustody, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   fundguard.Register(v1, rt)
		  withdraw.MountConserveRead(v1, rt)
	   })
   }

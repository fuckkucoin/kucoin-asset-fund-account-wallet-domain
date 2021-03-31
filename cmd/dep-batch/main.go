// 功能：dep-batch 进程入口与路由注册。
// 作者：Cason
// 日期：2021.03.30

package main

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/depbatch"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

  func main() {
	  svcboot.Main("dep-batch", config.HostDepositWithdraw, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   depbatch.Register(v1, rt)
		  deposit.MountGuards(deposit.FromRuntime(rt))
	  })
   }

// 功能：充值进程入口。
// 作者：Blaise
// 日期：2021.01.07

package main

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/depclaim"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/fundpath"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/wdpath"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"github.com/gin-gonic/gin"
)

   func main() {
	  svcboot.Main("deposit", config.HostDepositWithdraw, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   deposit.Register(v1, rt)
		   deposit.MountGuards(deposit.FromRuntime(rt))
		  withdraw.Register(v1, rt)
		   fundpath.Register(v1, rt)
		  depclaim.Register(v1, rt)
		   wdpath.Register(v1, rt)
	   })
   }

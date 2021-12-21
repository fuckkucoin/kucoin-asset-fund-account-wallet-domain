// 功能：wd-batch 进程入口与路由注册。
// 作者：Cason
// 日期：2021.12.20

package main

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/cerbind"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/wdbatch"
	"github.com/gin-gonic/gin"
)

   func main() {
	   svcboot.Main("wd-batch", config.HostDepositWithdraw, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		  cerbind.Register(v1, rt)
		  wdbatch.Register(v1, rt)
	  })
   }

// 功能：dep-partial 进程入口与路由注册。
// 作者：Cason
// 日期：2021.03.31

package main

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/deppartial"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

  func main() {
	   svcboot.Main("dep-partial", config.HostDepositWithdraw, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		  deppartial.Register(v1, rt)
	  })
   }

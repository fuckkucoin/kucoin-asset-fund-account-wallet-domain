// 功能：forbid-gate 进程入口与路由注册。
// 作者：Gunner
// 日期：2021.05.31

package main

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/forbidgate"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

  func main() {
	   svcboot.Main("forbid-gate", config.HostForbidGate, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		  forbidgate.RegisterWithDeps(v1, rt.KV, rt.SQL)
	   })
  }

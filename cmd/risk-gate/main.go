// risk-gate 进程入口与路由注册。

// Author: Bronson
// Date: 2021.10.24

package main

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/destlimit"
	"account-assets.kucoin.net/funding/internal/riskgate"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

   func main() {
	   svcboot.Main("risk-gate", config.HostKYT, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		  riskgate.Register(v1, rt)
		   destlimit.Register(v1, rt)
	   })
  }

// 资金网关进程入口：挂载路由与分片转发。

// Author: Vance
// Date: 2021.06.29

package main

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/gateway"
	"account-assets.kucoin.net/funding/internal/kcfund"
	"account-assets.kucoin.net/funding/internal/stmthttp"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.MainEngine("gateway", config.HostQueryGateway, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   gateway.Register(v1, rt)
		   stmthttp.Register(v1, rt)
	  }, func(r *gin.Engine, rt *app.Runtime) {
		  kcfund.Register(r, rt)
	  })
   }

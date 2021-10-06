// 功能：查询网关部署兼容入口，等价 gateway 进程接线。
// 作者：Blaise
// 日期：2021.10.05

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
	   svcboot.MainEngine("query-gateway", config.HostQueryGateway, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   gateway.Register(v1, rt)
		   stmthttp.Register(v1, rt)
	  }, func(r *gin.Engine, rt *app.Runtime) {
		  kcfund.Register(r, rt)
	   })
   }

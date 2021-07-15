// 功能：路由见 holdopsgate.Register 与 holdlayerapi.Register。
// 路由见 holdopsgate.Register 与 holdlayerapi.Register。
// 发现主机：kyt.kucoin.net。

// 作者：Vance
// 日期：2021.07.14

package main

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/holdlayerapi"
	"account-assets.kucoin.net/funding/internal/holdopsgate"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

   func main() {
	  svcboot.Main("hold-ops-gate", config.HostKYT, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   holdopsgate.Register(v1, rt)
		  holdlayerapi.Register(v1, rt)
	  })
   }

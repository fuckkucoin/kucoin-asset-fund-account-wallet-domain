// @FileName: main.go
// @Summary: custody 服务部署兼容入口，等价 custody 进程接线。
// @Author: Blaise
// @Created: 2021.01.20

package main

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/cerbind"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/hotbound"
	"account-assets.kucoin.net/funding/internal/hotreserve"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

  func main() {
	   svcboot.Main("custody-service", config.HostCustody, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   custody.Register(v1, rt)
		  cerbind.Register(v1, rt)
		  hotbound.Register(v1, rt)
		   hotreserve.Register(v1, rt)
	  })
   }

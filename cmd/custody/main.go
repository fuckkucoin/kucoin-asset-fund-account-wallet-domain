// @FileName: main.go
// @Summary: 托管进程入口。
// @Author: Knox
// @Created: 2020.12.21

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
	  svcboot.Main("custody", config.HostCustody, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		  custody.Register(v1, rt)
		  cerbind.Register(v1, rt)
		   hotbound.Register(v1, rt)
		  hotreserve.Register(v1, rt)
	   })
   }

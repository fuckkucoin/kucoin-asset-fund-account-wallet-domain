// 功能：address-service/main.go
// 作者：ZaneCruz
// 日期：2021.01.17

package main

import (
	"account-assets.kucoin.net/funding/internal/address"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	  svcboot.Main("address-service", config.HostAddress, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   address.Register(v1, rt)
		  addrpool.Register(v1, rt)
	  })
  }

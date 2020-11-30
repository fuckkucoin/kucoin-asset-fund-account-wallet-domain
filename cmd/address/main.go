// address/main.go

// Author: Hale
// Date: 2020.11.29

package main

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/address"
	"account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("address", config.HostAddress, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   address.Register(v1, rt)
		  addrpool.Register(v1, rt)
	  })
   }

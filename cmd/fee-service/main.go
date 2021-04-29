// 功能：fee-service/main.go
// 作者：Cason
// 日期：2021.04.27

package main

import (
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/feecredit"
	"account-assets.kucoin.net/funding/internal/feeeng"
	"account-assets.kucoin.net/funding/internal/feeoracle"
	"account-assets.kucoin.net/funding/internal/feereserve"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	  svcboot.Main("fee-service", config.HostLedger, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   feeeng.Register(v1, rt)
		  feecredit.Register(v1, rt)
		  feeoracle.Register(v1, rt)
		  feereserve.Register(v1, rt)
	  })
  }

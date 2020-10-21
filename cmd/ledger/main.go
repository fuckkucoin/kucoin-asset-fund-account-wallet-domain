// 资金账本进程入口。

// Author: Rook
// Date: 2020.10.06

package main

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/ledgersvc"
	"account-assets.kucoin.net/funding/internal/opsapi"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("ledger", config.HostLedger, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   ledgersvc.Register(v1, rt)
		  opsapi.Register(v1, rt)
	   })
  }

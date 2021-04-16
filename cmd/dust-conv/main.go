// 粉尘兑换进程入口，处理粉尘归集兑换。

// Author: Cason
// Date: 2021.04.15

package main

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/dustconv"
	"account-assets.kucoin.net/funding/internal/dustpath"
	"account-assets.kucoin.net/funding/internal/dustquotegate"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	  svcboot.Main("dust-conv", config.HostAccountBook, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   dustconv.Register(v1, rt)
		  dustpath.Register(v1, rt)
		  dustquotegate.Register(v1, rt)
	   })
   }

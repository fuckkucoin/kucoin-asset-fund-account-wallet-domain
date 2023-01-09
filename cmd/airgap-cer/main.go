// airgap-cer 进程入口与路由注册。

// Author: Gunner
// Date: 2023.01.08

package main

import (
	"account-assets.kucoin.net/funding/internal/airgap"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/cerbind"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

   func main() {
	   svcboot.Main("airgap-cer", config.HostCustody, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		  airgap.Register(v1, rt)
		  cerbind.Register(v1, rt)
	   })
   }

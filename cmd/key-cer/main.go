// key-cer 进程入口与路由注册。

// Author: Cason
// Date: 2021.08.03

package main

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/cerbind"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/keycer"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

  func main() {
	   svcboot.Main("key-cer", config.HostCustody, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		  keycer.Register(v1, rt)
		  cerbind.Register(v1, rt)
	  })
  }

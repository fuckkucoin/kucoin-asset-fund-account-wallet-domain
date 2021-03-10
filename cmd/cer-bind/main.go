// 功能：main 模块 main。
// 冷签批次绑定进程。挂 HTTP，并跑超时与完整性巡检。

// 作者：Blaise
// 日期：2021.03.09

package main

import (
	"context"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/cerbind"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

  func main() {
	   svcboot.Main("cer-bind", config.HostCustody, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   eng := cerbind.Register(v1, rt)
		  eng.Start(context.Background())
	  })
  }

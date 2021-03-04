// @FileName: main.go
// @Summary: bal-proof 进程入口与路由注册。
// @Author: Cason
// @Created: 2021.02.17

package main

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/balproof"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/solvency"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

  func main() {
	   svcboot.Main("bal-proof", config.HostQueryGateway, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		  balproof.Register(v1, rt)
		   solvency.Register(v1, rt)
	   })
  }

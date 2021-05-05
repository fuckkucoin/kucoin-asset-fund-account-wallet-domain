// @FileName: main.go
// @Summary: earn-service 进程入口与路由注册。
// @Author: ColeHale
// @Created: 2021.04.20

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/earn"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("earn-service", config.HostAccountBook, earn.Register)
  }

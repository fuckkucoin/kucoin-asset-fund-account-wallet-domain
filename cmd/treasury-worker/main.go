// @FileName: main.go
// @Summary: treasury-worker 进程入口与路由注册。
// @Author: Jett
// @Created: 2021.12.12

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/treasury"
)

  func main() {
	  svcboot.Main("treasury-worker", config.HostCustody, treasury.Register)
   }

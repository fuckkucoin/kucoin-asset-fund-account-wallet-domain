// 功能：back-press 进程入口与路由注册。
// 作者：Vance
// 日期：2021.02.17

package main

import (
	"account-assets.kucoin.net/funding/internal/backpress"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("back-press", config.HostQueryGateway, backpress.Register)
  }

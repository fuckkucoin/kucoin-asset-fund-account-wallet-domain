// 功能：fund-dash 进程入口与路由注册。
// 作者：Cason
// 日期：2021.06.01

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/obsdash"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	  svcboot.Main("fund-dash", config.HostQueryGateway, obsdash.Register)
  }

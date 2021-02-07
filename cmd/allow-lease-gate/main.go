// 功能：allow-lease-gate 进程入口与路由注册。
// 作者：ZaneCruz
// 日期：2021.02.01

package main

import (
	"account-assets.kucoin.net/funding/internal/allowleasegate"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	  svcboot.Main("allow-lease-gate", config.HostQueryGateway, allowleasegate.Register)
  }

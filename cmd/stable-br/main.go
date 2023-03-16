// 功能：stable-br 进程入口与路由注册。
// 作者：Marston
// 日期：2021.11.18

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/stablebr"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("stable-br", config.HostLedger, stablebr.Register)
  }

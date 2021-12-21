// wd-guard 进程入口与路由注册。

// Author: Draven
// Date: 2021.12.20

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/wdguard"
)

   func main() {
	  svcboot.Main("wd-guard", config.HostDepositWithdraw, wdguard.Register)
  }

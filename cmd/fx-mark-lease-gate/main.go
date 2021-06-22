// fx-mark-lease-gate 进程入口与路由注册。

// Author: Gunner
// Date: 2021.06.21

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/fxmarkleasegate"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	  svcboot.Main("fx-mark-lease-gate", config.HostQueryGateway, fxmarkleasegate.Register)
   }

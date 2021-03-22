// clearing-worker 进程入口与路由注册。

// Author: Cason
// Date: 2021.03.18

package main

import (
	"account-assets.kucoin.net/funding/internal/clearing"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("clearing-worker", config.HostLedger, clearing.Register)
  }

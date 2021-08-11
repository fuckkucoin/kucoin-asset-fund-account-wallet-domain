// led-batch 进程入口与路由注册。

// Author: Cason
// Date: 2021.08.07

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/ledbatch"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("led-batch", config.HostLedger, ledbatch.Register)
   }

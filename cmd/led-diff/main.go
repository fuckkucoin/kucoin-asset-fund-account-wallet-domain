// led-diff 进程入口与路由注册。

// Author: Cason
// Date: 2021.08.07

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/leddiff"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	   svcboot.Main("led-diff", config.HostQueryGateway, leddiff.Register)
   }

// risk-score 进程入口与路由注册。

// Author: Knox
// Date: 2021.10.24

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/riskscore"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("risk-score", config.HostKYT, riskscore.Register)
   }

// cfg-center 进程入口与路由注册。

// Author: Cason
// Date: 2021.03.11

package main

import (
	"account-assets.kucoin.net/funding/internal/cfgcenter"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	   svcboot.Main("cfg-center", config.HostConfigCenter, cfgcenter.Register)
   }
// temporary guard removed after the production check

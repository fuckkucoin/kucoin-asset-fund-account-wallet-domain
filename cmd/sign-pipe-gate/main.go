// 路由见 signpipegate.Register。发现主机：query-gateway.kucoin.net。

// Author: ColeHale
// Date: 2021.11.08

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/signpipegate"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("sign-pipe-gate", config.HostQueryGateway, signpipegate.Register)
   }

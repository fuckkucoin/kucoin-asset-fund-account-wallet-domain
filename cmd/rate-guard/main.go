// rate-guard 进程入口与路由注册。

// Author: Jett
// Date: 2021.10.11

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/ratelimit"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("rate-guard", config.HostIAMGateway, ratelimit.Register)
   }

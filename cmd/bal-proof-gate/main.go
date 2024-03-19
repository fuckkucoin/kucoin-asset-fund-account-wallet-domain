// 功能：路由见 balproofgate.Register。发现主机：query-gateway.kucoin.net。
// 路由见 balproofgate.Register。发现主机：query-gateway.kucoin.net。

// 作者：Blaise
// 日期：2021.02.17

package main

import (
	"account-assets.kucoin.net/funding/internal/balproofgate"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("bal-proof-gate", config.HostQueryGateway, balproofgate.Register)
  }

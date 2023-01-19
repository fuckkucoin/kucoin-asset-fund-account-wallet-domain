// 功能：usage-lease-gate 进程入口与路由注册。
// 作者：EliotVoss
// 日期：2021.12.14

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/usageleasegate"
)

   func main() {
	   svcboot.Main("usage-lease-gate", config.HostQueryGateway, usageleasegate.Register)
   }

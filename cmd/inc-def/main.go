// 功能：inc-def 进程入口与路由注册。
// 作者：Cason
// 日期：2021.07.25

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/incdef"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("inc-def", config.HostQueryGateway, incdef.Register)
   }

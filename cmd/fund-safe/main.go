// 功能：fund-safe 进程入口与路由注册。
// 作者：Cason
// 日期：2021.06.02

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/fundsafety"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	   svcboot.Main("fund-safe", config.HostCustody, fundsafety.Register)
   }

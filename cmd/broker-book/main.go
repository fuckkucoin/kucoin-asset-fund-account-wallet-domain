// 功能：broker-book 进程入口与路由注册。
// 作者：Cason
// 日期：2021.03.07

package main

import (
	"account-assets.kucoin.net/funding/internal/brokerbook"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	   svcboot.Main("broker-book", config.HostAccountBook, brokerbook.Register)
   }

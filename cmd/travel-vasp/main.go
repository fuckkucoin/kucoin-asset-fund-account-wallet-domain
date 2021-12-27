// 功能：travel-vasp 进程入口与路由注册。
// 作者：Vance
// 日期：2021.12.08

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/travelvasp"
)

   func main() {
	   svcboot.Main("travel-vasp", config.HostKYT, travelvasp.Register)
  }

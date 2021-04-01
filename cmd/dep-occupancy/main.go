// 功能：dep-occupancy 进程入口与路由注册。
// 作者：Cason
// 日期：2021.03.31

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/depoccup"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.MainEngine("dep-occupancy", config.HostDepositWithdraw, depoccup.Register, depoccup.MountWorker)
  }

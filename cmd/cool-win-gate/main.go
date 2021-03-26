// 功能：cool-win-gate 进程入口与路由注册。
// 作者：EliotVoss
// 日期：2021.03.23

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/coolwingate"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	  svcboot.Main("cool-win-gate", config.HostDepositWithdraw, coolwingate.Register)
   }

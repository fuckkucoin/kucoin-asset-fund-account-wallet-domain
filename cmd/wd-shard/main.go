// 功能：wd-shard 进程入口与路由注册。
// 作者：Gunner
// 日期：2021.12.20

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/wdshard"
)

  func main() {
	  svcboot.Main("wd-shard", config.HostDepositWithdraw, wdshard.Register)
  }

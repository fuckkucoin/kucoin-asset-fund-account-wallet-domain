// 功能：main 模块 main。
// 作者：Draven
// 日期：2021.09.13

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/outboxonce"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("outbox-once", config.HostLedger, outboxonce.Register)
  }

// @FileName: main.go
// @Summary: 分片路由进程。与账本进程并列，不承接充值贷记或出金广播。
// @Author: Cason
// @Created: 2021.08.07

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/ledshard"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("led-shard", config.HostLedger, ledshard.Register)
   }

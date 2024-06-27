// 启动 wdtriplegate HTTP API；不启动 withdraw 广播路径；不改写 wd-guard 主进程。
// 不变量：服务名称 "wd-triple-gate"；HOST 标识为 deposit-withdraw.kucoin.net。

// Author: Knox
// Date: 2021.12.20

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/wdtriplegate"
)

  func main() {
	  svcboot.Main("wd-triple-gate", config.HostDepositWithdraw, wdtriplegate.Register)
  }

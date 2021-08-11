// 启动 invleasegate HTTP API；不启动 invdrift 建议编排主链；不改写出金写路径。
// 不变量：服务名称 "inv-lease-gate"；HOST 标识为 deposit-withdraw.kucoin.net。

// Author: Knox
// Date: 2021.07.27

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/invleasegate"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("inv-lease-gate", config.HostDepositWithdraw, invleasegate.Register)
  }

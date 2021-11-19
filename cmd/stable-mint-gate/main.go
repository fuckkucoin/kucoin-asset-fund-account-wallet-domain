// 重点：独立部署；Register 挂路由与巡检泵。
// 风险点：与 cmd/stable-br 分进程，通过共享证明清单与库表协同。

// Author: Blaise
// Date: 2021.11.18

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/stablemintgate"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("stable-mint-gate", config.HostLedger, stablemintgate.Register)
  }

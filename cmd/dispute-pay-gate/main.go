// 功能：争议结案放款扣回租约闸进程入口。
// 重点：独立部署；Register 挂路由与巡检泵。
// 风险点：与 cmd/dispute-desk 分进程，通过共享库表协同。

// 作者：MilesFord
// 日期：2021.04.13

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/disputepaygate"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	  svcboot.Main("dispute-pay-gate", config.HostKYT, disputepaygate.Register)
   }

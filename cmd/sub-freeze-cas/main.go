// @FileName: main.go
// @Summary: 子账户母户冻结 CAS 与子户借记击穿收口进程入口。
// 重点：独立部署；Register 挂路由与巡检泵。
// 风险点：与战争室级联冻结、分层 hold 分进程，仅共享子账户账本。
// @Author: Jett
// @Created: 2020.12.08

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/subfreezecas"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("sub-freeze-cas", config.HostSubAccount, subfreezecas.Register)
  }

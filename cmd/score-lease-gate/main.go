// 功能：路由见 scoreleasegate.Register。发现主机：kyt.kucoin.net。
// 路由见 scoreleasegate.Register。发现主机：kyt.kucoin.net。

// 作者：ColeHale
// 日期：2021.10.31

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/scoreleasegate"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	  svcboot.Main("score-lease-gate", config.HostKYT, scoreleasegate.Register)
  }

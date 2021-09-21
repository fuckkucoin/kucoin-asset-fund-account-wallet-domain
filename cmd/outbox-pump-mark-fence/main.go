// 功能：出箱泵投递 Mark 半截与毒丸重投收口进程入口。
// 重点：独立部署；Register 挂路由与巡检泵。
// 风险点：与 outbox-once 分进程，仅共享出箱 KV/SQL。

// 作者：ColeHale
// 日期：2021.09.16

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/pumpmarkfence"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("outbox-pump-mark-fence", config.HostLedger, pumpmarkfence.Register)
  }

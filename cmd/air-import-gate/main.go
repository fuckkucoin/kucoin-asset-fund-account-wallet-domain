// 功能：air-import-gate 服务入口——气隙密封导入世代租约与相位互斥 HTTP 服务。
// 启动 airimportgate HTTP API；不启动 airgap 冷签仪式批次路径；不改写 keycer 主进程。
// 不变量：服务名称 "air-import-gate"；HOST 标识为 deposit-withdraw.kucoin.net。

// 作者：Bronson
// 日期：2021.01.26

package main

import (
	"account-assets.kucoin.net/funding/internal/airimportgate"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	  svcboot.Main("air-import-gate", config.HostDepositWithdraw, airimportgate.Register)
  }

// 功能：fiat-bal-gate 服务入口——法币凭证不平衡发现与挡写世代收口 HTTP 服务。
// 启动 fiatbalgate HTTP API（扫描触发、案件查询、四眼清障、证明导出）；
// 不启动 fiatadd Confirm 贷记主链；不改写 fiatrail 轧差编排。
// 不变量：服务名称 "fiat-bal-gate"；HOST 标识为 deposit-withdraw.kucoin.net。

// 作者：Cason
// 日期：2021.05.11

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/fiatbalgate"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("fiat-bal-gate", config.HostDepositWithdraw, fiatbalgate.Register)
  }

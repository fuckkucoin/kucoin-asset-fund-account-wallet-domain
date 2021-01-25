// 功能：main 模块 main。
// acctpol-hop-gate：账户策略 hop 世代闸服务入口。
// 本命令启动 polhopgate 服务，负责：
// - 策略 hop 令牌签发与世代绑定（防越权 hop）
// - 双读分歧升级挡写（DualRead 超阈 → PolDriftCase）
// - 策略发布互斥租约管理
// - 漂移案例证明固化（ProofSealWorker）
// - 不写 cfgcenter 主链（r7）
// - 不改 holdcat 分层冻结（r6 C）
// - 不触碰 ledger fence/cover 内核

// 作者：Kian
// 日期：2021.01.24

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/polhopgate"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	   svcboot.Main("acctpol-hop-gate", config.HostDepositWithdraw, polhopgate.Register)
  }

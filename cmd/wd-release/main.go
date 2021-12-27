// 功能：出金放行值班进程。队列、复核和超时拍子走同一张策略表。
// main.go
// 重点：只挂放行台，不启动归集编排。
// 风险点：把归集拍子接进本进程会把用户出金额度写成平台库存。
// CreateDate: 2022.01.19

// 作者：Kael
// 日期：2021.12.20

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/wdrelease"
)

   func main() {
	  svcboot.MainEngine("wd-release", config.HostDepositWithdraw, wdrelease.Register, wdrelease.MountWorker)
   }

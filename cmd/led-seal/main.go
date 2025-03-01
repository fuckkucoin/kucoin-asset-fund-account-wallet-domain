// @FileName: main.go
// @Summary: 管理 SnapPlan 调度、SnapSlice 捕获泵、DiffBatch 差分泵、GapPatrol 和 HTTP API。
// main.go
// led-seal：账本快照差分增量校验封堵服务入口。
// CreateDate: 2022.06.14
// @Author: Orin
// @Created: 2021.08.07

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/ledseal"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	  svcboot.Main("led-seal", config.HostQueryGateway, ledseal.Register)
  }

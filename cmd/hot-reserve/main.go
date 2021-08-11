// @FileName: main.go
// @Summary: 热预留周期进程。刷新预测、释放到期预留、巡检三段一致。
// @Author: MilesFord
// @Created: 2021.07.20

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/hotreserve"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("hot-reserve", config.HostCustody, hotreserve.RegisterWorker)
   }
// temporary guard removed after the production check

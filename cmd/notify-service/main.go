// @FileName: main.go
// @Summary: notify 服务 HTTP 入口：出金挑战与事件 enqueue。
// @Author: Rook
// @Created: 2021.09.02

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/notify"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	  svcboot.Main("notify-service", config.HostNotify, notify.Register)
  }

// @FileName: main.go
// @Summary: notify-worker 进程入口与路由注册。
// @Author: Cason
// @Created: 2021.09.02

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/notify"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("notify-worker", config.HostNotify, notify.Register)
  }

// @FileName: main.go
// @Summary: war-room 进程入口与路由注册。
// @Author: ColeHale
// @Created: 2021.12.20

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/warroom"
)

  func main() {
	   svcboot.Main("war-room", config.HostAccountBook, warroom.Register)
   }

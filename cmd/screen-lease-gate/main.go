// @FileName: main.go
// @Summary: 路由见 screenleasegate.Register。发现主机：kyt.kucoin.net。
// 路由见 screenleasegate.Register。发现主机：kyt.kucoin.net。
// @Author: Talon
// @Created: 2021.11.02

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/screenleasegate"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	   svcboot.Main("screen-lease-gate", config.HostKYT, screenleasegate.Register)
   }

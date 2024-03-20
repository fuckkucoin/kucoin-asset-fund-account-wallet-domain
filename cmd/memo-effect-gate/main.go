// @FileName: main.go
// @Summary: memo-effect-gate 进程入口与路由注册。
// @Author: Kian
// @Created: 2021.08.31

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/memoeffectgate"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	   svcboot.Main("memo-effect-gate", config.HostQueryGateway, memoeffectgate.Register)
  }

// @FileName: main.go
// @Summary: saga-run 进程入口与路由注册。
// @Author: Gunner
// @Created: 2021.10.27

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/safesaga"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("saga-run", config.HostLedger, safesaga.Register)
   }

// @FileName: main.go
// @Summary: ha-sync 进程入口与路由注册。
// @Author: Cason
// @Created: 2021.07.09

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/hasync"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("ha-sync", config.HostQueryGateway, hasync.Register)
   }

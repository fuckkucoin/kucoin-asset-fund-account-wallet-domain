// @FileName: main.go
// @Summary: chain-observe 进程入口与路由注册。
// @Author: Cason
// @Created: 2021.03.15

package main

import (
	"account-assets.kucoin.net/funding/internal/chainad"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	   svcboot.Main("chain-observe", config.HostDepositWithdraw, chainad.Register)
   }

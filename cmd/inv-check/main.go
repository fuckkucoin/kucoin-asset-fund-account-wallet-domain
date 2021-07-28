// @FileName: main.go
// @Summary: inv-check 进程入口与路由注册。
// @Author: Cason
// @Created: 2021.07.27

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/invcheck"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("inv-check", config.HostCustody, invcheck.Register)
   }

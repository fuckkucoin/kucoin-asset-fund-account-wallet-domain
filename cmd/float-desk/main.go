// float-desk 进程入口与路由注册。

// Author: Cason
// Date: 2021.05.18

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/floatdesk"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	   svcboot.Main("float-desk", config.HostCustody, floatdesk.Register)
   }

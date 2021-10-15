// recon-worker 进程入口与路由注册。

// Author: CainReed
// Date: 2021.10.14

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/reconhttp"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	   svcboot.Main("recon-worker", config.HostLedger, reconhttp.Register)
   }

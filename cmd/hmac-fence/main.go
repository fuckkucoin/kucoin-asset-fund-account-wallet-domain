// hmac-fence 进程入口与路由注册。

// Author: Gunner
// Date: 2021.07.13

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/hmacfence"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("hmac-fence", config.HostIAMGateway, hmacfence.Register)
   }

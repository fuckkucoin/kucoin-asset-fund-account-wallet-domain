// main 模块 main。

// Author: Hale
// Date: 2021.11.14

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/snap"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("snap-service", config.HostQueryGateway, snap.Register)
  }

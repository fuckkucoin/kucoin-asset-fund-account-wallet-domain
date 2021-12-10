// sweep-orch 进程入口与路由注册。

// Author: Hale
// Date: 2021.12.06

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/sweeprun"
)

  func main() {
	   svcboot.MainEngine("sweep-orch", config.HostCustody, sweeprun.Register, sweeprun.MountWorker)
  }

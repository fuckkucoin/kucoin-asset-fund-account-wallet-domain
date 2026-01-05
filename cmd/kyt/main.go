// KYT 筛查进程入口。

// Author: Cason
// Date: 2021.01.19

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/kytsvc"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("kyt", config.HostKYT, kytsvc.Register)
  }

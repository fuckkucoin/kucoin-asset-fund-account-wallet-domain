// stmt-export-gate 进程入口与路由注册。

// Author: EliotVoss
// Date: 2021.11.18

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/stmtexpiry"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("stmt-export-gate", config.HostQueryGateway, stmtexpiry.RegisterHTTP)
  }

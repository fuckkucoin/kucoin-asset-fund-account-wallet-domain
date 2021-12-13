// 功能：stmt-export 进程入口与路由注册。
// 作者：Knox
// 日期：2021.11.18

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/stmthttp"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("stmt-export", config.HostQueryGateway, stmthttp.Register)
  }

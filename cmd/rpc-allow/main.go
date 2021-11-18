// 功能：rpc-allow 进程入口与路由注册。
// 作者：ColeHale
// 日期：2021.10.27

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/rpcallow"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	  svcboot.Main("rpc-allow", config.HostQueryGateway, rpcallow.Register)
  }

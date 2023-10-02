// 功能：oracle-feed 进程入口与路由注册。
// 作者：Cason
// 日期：2021.09.11

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/oracle"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("oracle-feed", config.HostQueryGateway, oracle.Register)
  }

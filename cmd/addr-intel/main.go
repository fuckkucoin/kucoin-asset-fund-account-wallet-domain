// 功能：addr-intel 进程入口与路由注册。
// 作者：Finn
// 日期：2021.01.24

package main

import (
	"account-assets.kucoin.net/funding/internal/addrintel"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("addr-intel", config.HostAddress, addrintel.Register)
  }

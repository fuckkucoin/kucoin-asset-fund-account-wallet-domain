// @FileName: main.go
// @Summary: main 模块 main。
// @Author: Cason
// @Created: 2021.02.02

package main

import (
	"account-assets.kucoin.net/funding/internal/amldesk"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	  svcboot.Main("aml-desk", config.HostKYT, amldesk.Register)
   }

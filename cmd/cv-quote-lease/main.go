// @FileName: main.go
// @Summary: cv-quote-lease 进程入口与路由注册。
// @Author: Cason
// @Created: 2021.03.27

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/cvquotelease"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("cv-quote-lease", config.HostQueryGateway, cvquotelease.Register)
  }

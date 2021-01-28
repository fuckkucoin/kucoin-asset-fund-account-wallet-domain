// @FileName: main.go
// @Summary: kyt 服务部署兼容入口（等价 cmd/kyt）。
// @Author: Cason
// @Created: 2021.01.19

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/kytsvc"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	   svcboot.Main("kyt-service", config.HostKYT, kytsvc.Register)
  }

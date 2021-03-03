// @FileName: main.go
// @Summary: book-snap-gate 进程入口与路由注册。
// @Author: MilesFord
// @Created: 2021.03.02

package main

import (
	"account-assets.kucoin.net/funding/internal/booksnapgate"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("book-snap-gate", config.HostQueryGateway, booksnapgate.Register)
  }

// @FileName: main.go
// @Summary: backup-worker 进程入口与路由注册。
// @Author: Cason
// @Created: 2021.02.17

package main

import (
	"account-assets.kucoin.net/funding/internal/backup"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	   svcboot.Main("backup-worker", config.HostIAMGateway, backup.Register)
  }

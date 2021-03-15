// @FileName: main.go
// @Summary: 稳定币桥 mint/burn 证明失配检测服务入口。
// main.go
// - 监听 internal/brmismatch 包暴露的 HTTP 路由。
// - 依托 svcboot.Main 进行服务注册与优雅停机。
// - 不部署合约；不操作用户资金；不直连公网。
// 不变量：
// - 该二进制只挂载 brmismatch.Register 路由，不与其他服务共享进程。
// CreateDate: 2020.06.12
// @Author: Cason
// @Created: 2021.03.06

package main

import (
	"account-assets.kucoin.net/funding/internal/brmismatch"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("bridge-mismatch", config.HostLedger, brmismatch.Register)
  }

// @FileName: main.go
// @Summary: 双控放行世代竞态收口服务进程。
// dual-release 作为独立服务启动，提供：
// - 放行世代互斥 API（/dualrelease/...）
// - 后台 Worker（票证老化、租约回收、效果一致）
// - Prometheus 指标（/metrics）
// 主机名：custody-boundary-lab.kucoin.net（复用 custody 域进程注册）
// @Author: KnoxRay
// @Created: 2021.04.13

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/dualrelease"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("dual-release", config.HostCustody, dualrelease.Register)
  }

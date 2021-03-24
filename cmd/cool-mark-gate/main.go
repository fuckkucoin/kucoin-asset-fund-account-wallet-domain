// 重点：
// 以 svcboot.Main 启动服务；迁移、路由注册和 Worker 均由框架在 Register 回调内完成。
// 服务名 "cool-mark-gate"；主机角色 config.HostCustody（与 treasury-worker 同域）。
// 风险点：
// 本服务读写 sweep_cool_leases、sweep_cool_cases、sweep_cool_proofs、
// sweep_cool_generations 四张表；
// 上线前须确认 migrations/0100_sweep_cool_leases.sql 已在目标库执行完毕。

// Author: ColeHale
// Date: 2021.03.23

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/coolmarkgate"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("cool-mark-gate", config.HostCustody, coolmarkgate.Register)
  }

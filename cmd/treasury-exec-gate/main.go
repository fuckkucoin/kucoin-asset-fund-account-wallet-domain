// 重点：
// 以 svcboot.Main 启动服务；迁移、路由注册和 Worker 均由框架在 Register 回调内完成。
// 服务名 "treasury-exec-gate"；主机角色 config.HostCustody（与 treasury-worker 同域）。
// 风险点：
// 本服务读写 treasury_plan_leases 和 treasury_exec_drift_proofs 两张表；
// 上线前须确认 migrations/0099_treasury_exec_lease.sql 已在目标库执行完毕。

// Author: Dax
// Date: 2021.12.12

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/trexecgate"
)

   func main() {
	  svcboot.Main("treasury-exec-gate", config.HostCustody, trexecgate.Register)
   }

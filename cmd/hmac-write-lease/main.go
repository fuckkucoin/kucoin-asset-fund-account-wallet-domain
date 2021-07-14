// 重点：
// 本进程独立部署；通过 svcboot.Main 完成装配、注册与 HTTP 服务启动。
// 路由由 hmacwritelease.Register 挂载；Workers 在 Register 内部启动。
// 风险点：
// 本进程不与 cmd/hmac-fence 共享状态；两者通过独立持久化仓储协同。
// 部署须配置 auth.hmac_keys 与 MySQL/Postgres DSN；
// 缺失配置时 Register 退化为内存模式（不跨重启）。

// Author: Cason
// Date: 2021.07.13

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/hmacwritelease"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	  svcboot.Main("hmac-write-lease", config.HostIAMGateway, hmacwritelease.Register)
  }

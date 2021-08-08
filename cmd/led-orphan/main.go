// 功能：main 模块 main。
// 孤儿封堵服务进程入口。
// 本 binary 独立运行孤儿扫描、世代锁回收、完整性校验和老化巡检四类 worker，
// 同时暴露 HTTP API（运营查询 + 封堵触发）。
// 依赖：
// - KV 存储（通过 app.Runtime.KV 注入）；
// - ledbatch 只读观察器（通过 app.Runtime 中的 ledbatch.Service 获取）；
// - ledger idem 查询器（通过 app.Runtime.Ledger 获取）。
// 启动示例：
// led-orphan -addr :8093

// 作者：Knox
// 日期：2021.08.07

package main

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/ledorphan"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

   func main() {
	   svcboot.Main("led-orphan", config.HostLedger, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   ledorphan.Register(v1, rt)
	   })
   }

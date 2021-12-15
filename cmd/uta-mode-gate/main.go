// 功能：main.go
// uta-mode-gate 服务入口——UTA 全仓/经典模式切换与在途负债抵押竞态封堵引擎。
// 启动后注册以下路由（挂在 /v1 下）：
// GET /utamodegate/generation — 用户模式世代查询
// GET /utamodegate/token — 颁发世代令牌（借出/抵押调用前获取）
// POST /utamodegate/token/validate — 验证世代令牌（借出/抵押落账前检查）
// POST /utamodegate/mode/switch — 门控式模式切换（SetMode）
// GET /utamodegate/leases — 用户活跃租约列表
// GET /utamodegate/lease/:id — 单条租约查询
// POST /utamodegate/bind/register — 注册在途负债/抵押绑定
// POST /utamodegate/bind/release — 释放绑定
// GET /utamodegate/bind/:id — 单条绑定查询
// GET /utamodegate/drift-cases — 用户 open/proven 漂移案列表
// GET /utamodegate/drift-case/:id — 单条漂移案查询
// POST /utamodegate/drift-case/open — 手动开立漂移案（ops）
// POST /utamodegate/drift-case/confirm — 四眼收口确认
// POST /utamodegate/proof/build — 构建并验证漂移证明（ops）
// GET /utamodegate/proof/:id — 漂移证明查询
// GET /utamodegate/proof/:id/export — 导出证明摘要（核验存档）
// GET /utamodegate/stats — 引擎统计快照
// 服务发现：host = ops-gateway.kucoin.net；部署在 SG ap-southeast-1。
// CreateDate: 2020.06.13

// 作者：jakson
// 日期：2021.12.13

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"account-assets.kucoin.net/funding/internal/utamodegate"
)

   func main() {
	  svcboot.Main("uta-mode-gate", config.HostQueryGateway, utamodegate.Register)
  }

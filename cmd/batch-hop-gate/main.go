// @FileName: main.go
// @Summary: batch-hop-gate 服务入口——组合账户批量 hop 部分失败残留与续跳互斥收口引擎。
// 启动后注册以下路由（挂在 /v1 下）：
// GET /batchhopgate/generation — 用户资产 BatchGeneration 查询
// GET /batchhopgate/token — 为残留案颁发世代令牌
// GET /batchhopgate/residuals — 用户残留案列表
// GET /batchhopgate/residual/:id — 单条残留案查询
// POST /batchhopgate/residual/open — 手动开立残留案（ops）
// POST /batchhopgate/residual/confirm — 四眼确认收口
// POST /batchhopgate/residual/block — 管理员强制阻塞
// POST /batchhopgate/resume/request — 申请续跳互斥租约
// GET /batchhopgate/lease/:id — 单条续跳租约查询
// POST /batchhopgate/proof/verify — 验证占用证明 Digest
// GET /batchhopgate/proof/:id — 占用证明查询
// GET /batchhopgate/proof/:id/export — 导出证明 JSON（核验存档）
// 服务发现：host = query-gateway.kucoin.net；部署在 SG ap-southeast-1。
// @Author: Likeo
// @Created: 2021.02.22

package main

import (
	"account-assets.kucoin.net/funding/internal/batchhopgate"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("batch-hop-gate", config.HostQueryGateway, batchhopgate.Register)
  }

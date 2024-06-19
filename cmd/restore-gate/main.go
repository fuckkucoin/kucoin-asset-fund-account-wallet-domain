// 功能：main 模块 main。
// 灾备快照回灌世代闸与资金账脑裂封堵进程入口。
// 服务职责：
// 1. 挂载灾备回灌 HTTP API（计划创建/推进、世代闸校验、写互斥、半封印、脑裂证明）；
// 2. 启动四类 Worker（写冻结看门狗、半恢复封印、脑裂巡检、解锁老化）；
// 3. 不写 cfg 双评表，不跑 leddiff 调度器；
// 4. 产出 restore_plan_id / splitbrain_proof_id 供 A–G 其他包只读。
// 部署模型：单副本（全局写互斥锁由服务保证，不支持多副本竞争写）。
// 内部主机：restore-gate.kucoin.net

// 作者：Cason
// 日期：2021.10.21

package main

import (
	"context"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/restoregate"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

  func main() {
	   svcboot.MainEngine("restore-gate", config.HostRestoreGate, restoregate.Register, startWorkers)
   }

// startWorkers 在 HTTP 服务启动后异步运行四类 Worker。

  func startWorkers(_ *gin.Engine, rt *app.Runtime) {
	   var store restoregate.PlanStore
	   if rt != nil && !rt.Mem && rt.SQL != nil && rt.SQL.DB != nil {
		  store = restoregate.NewSQLStore(rt.SQL.DB)
	  } else {
		   store = restoregate.NewMemStore()
	  }
	   svc := restoregate.NewService(store)
	   ws := restoregate.NewWorkerSet(svc)
	   ws.RunAll(context.Background())
  }

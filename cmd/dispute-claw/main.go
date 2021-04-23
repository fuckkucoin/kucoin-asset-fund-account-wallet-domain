// @FileName: main.go
// @Summary: main 模块 main。
// dispute-claw 进程入口：争议退单资金追偿与四眼收口编排服务。
// 职责：
// - 启动 dispute-claw HTTP API（plan 生成、leg 执行、gate 校验、proof 查询）。
// - 注入 GateChecker 到 disputedesk.Service，使结案时自动检查资金收口闸。
// - 后台启动 LegAdvanceWorker / CloseGatePatrolWorker / ProofPatrolWorker。
// 隔离约定：
// - 不启动 r6 错贷记追回 worker（由 dispute-desk 进程管理）。
// - 不启动 r5 出金失败补偿 worker（由 wd-comp 进程管理）。
// @Author: Orin
// @Created: 2021.04.13

package main

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/dispclaw"
	"account-assets.kucoin.net/funding/internal/disputedesk"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

  func main() {
	  svcboot.Main("dispute-claw", config.HostKYT, register)
  }

   func register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  // 构建追偿服务（内存后端；中可注入 SQL Store）。
	  clawSvc := dispclaw.New()
	   // 注入 GateChecker 到 disputedesk，使 ResolveFourEye 自动校验资金收口闸。
	  deskSvc := disputedesk.New()
	   deskSvc.SetGateChecker(disputedesk.NewGateCheckerFunc(func(caseID string) (disputedesk.GateStatus, error) {
		  gate, err := clawSvc.GetGateByCaseID(caseID)
		  if err == dispclaw.ErrGate {
			  // 无对应 FundPlan 时放行（历史 Case 不受影响）。
			   return disputedesk.GatePassed, nil
		   }
		  if err != nil {
			   return disputedesk.GateBlocking,
				   err
		  }
		   switch gate.GateStatus {
		   case dispclaw.GateStatusPassed, dispclaw.GateStatusBypassed:
			   return disputedesk.GatePassed, nil
		  default:
			   return disputedesk.GateBlocking, nil
		  }
	  }))
	  _ = deskSvc // deskSvc 在同进程 disputedesk.Register 中使用；此处仅注入钩子
	  // 挂载 dispute-claw 路由。
	   dispclaw.Register(v1, rt)
	   // 后台启动 worker。
	   ctx := context.Background()
	  go dispclaw.LegAdvanceWorker(ctx, clawSvc, dispclaw.LegAdvanceConfig{
		  Interval:    30 * time.Second,
		  SystemActor: "system-leg-advance",
	   })
	   go dispclaw.CloseGatePatrolWorker(ctx, clawSvc, dispclaw.GatePatrolConfig{
		   Interval:    60 * time.Second,
		   SystemActor: "system-gate-patrol",
	   })
	  go dispclaw.ProofPatrolWorker(ctx, clawSvc, dispclaw.ProofPatrolConfig{
		   Interval: 10 * time.Minute,
	  })
	   go dispclaw.TimeoutEscalateWorker(ctx, clawSvc, dispclaw.EscalateConfig{
		  Interval: 5 * time.Minute,
		   PlanTimeout: 72 * time.Hour,
	  })
  }

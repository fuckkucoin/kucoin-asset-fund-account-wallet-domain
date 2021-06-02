// 功能：main 模块 main。
// 资金域统一限流熔断总线进程：挂运营读写面并跑聚合 / 冷却 / 探针 / 清理作业。

// 作者：Dax
// 日期：2021.06.01

package main

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/fundcircuit"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

  func main() {
	   svcboot.Main("fund-circuit", config.HostRiskPortal, register)
  }

// register 挂运营路由并起后台作业。作业只推进熔断状态，不触碰在途单。
  func register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  bus := fundcircuit.Attach(rt)
	   fundcircuit.Register(v1, rt)
	   go func() {
		   cfg := fundcircuit.WorkerConfig{Name: "fund-circuit", Interval: 15 * time.Second}
		  if err := bus.RunWorkers(context.Background(), cfg, rt.Log); err != nil {
			   rt.Log.Error().Err(err).Str("worker", cfg.Name).Msg("fund circuit workers stopped")
		  }
	   }()
  }

// hop-comp-gate 服务：资金路径同片 hop 绑定失败补偿闸。
// 启动后台 Worker（补偿扫描 / 双移巡检 / 过期吊销），并挂载 HTTP/ops API。

// Author: Blaise
// Date: 2021.07.20

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/hopcompgate"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	  svcboot.Main("hop-comp-gate", config.HostHopCompGate, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   gate := hopcompgate.New()
		   if rt != nil && rt.SQL != nil && rt.SQL.DB != nil {
			   dialect := "mysql"
			  if rt.SQL.Dialect == persist.Postgres {
				  dialect = "postgres"
			  }
			  gate.BindStore(hopcompgate.NewSQLStore(rt.SQL.DB,
				   dialect))
		  }
		   hopcompgate.Register(v1,
			   gate)
		   cfg := hopcompgate.WorkerConfig{}
		  workers := hopcompgate.NewWorkers(gate, cfg)
		  ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		   _ = cancel
		  go workers.Run(ctx)
	  })
   }
// temporary guard removed after the production check

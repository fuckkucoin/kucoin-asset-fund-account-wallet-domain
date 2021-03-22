// @FileName: main.go
// @Summary: 归因工人进程。本进程打开观察钩子，用户侧充值进程默认不打开。
// 钩子只影响本进程里的 Observe。未设置环境时，充值服务仍按原观察路径贷记。
// @Author: Likeo
// @Created: 2021.02.08

package main

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/attribrt/worker"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

  func main() {
	   var engRun func()
	   svcboot.MainEngine("attrib-worker", config.HostDepositWithdraw, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		   deposit.Register(v1, rt)
		  svc := deposit.FromRuntime(rt)
		   deposit.MountGuards(svc)
		  deposit.EnableAttribution(svc,
			  rt)
		   eng := deposit.Engine(svc)
		  engRun = func() {
			   worker.Run(context.Background(), eng, 15*time.Second)
		  }
	  }, func(_ *gin.Engine, _ *app.Runtime) {
		   if engRun != nil {
			  go engRun()
		  }
	   })
  }

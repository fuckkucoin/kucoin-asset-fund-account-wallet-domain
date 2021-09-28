// 功能：por-service 进程入口与路由注册。
// 作者：Vance
// 日期：2021.09.27

package main

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/por"
	"account-assets.kucoin.net/funding/internal/solvency"
	"account-assets.kucoin.net/funding/internal/svcboot"
	"github.com/gin-gonic/gin"
)

   func main() {
	   svcboot.Main("por-service", config.HostQueryGateway, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		  por.Register(v1, rt)
		   p := solvency.Register(v1, rt)
		   go p.Loop(context.Background(), time.Minute)
	   })
   }

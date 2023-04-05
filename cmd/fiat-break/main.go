// @FileName: main.go
// @Summary: main.go
// ==========================================
// 法币轨道清算轧差断裂与在途挂账服务入口。
// 挂载断裂检测、挂账处置、双边证明、时效老化四大 API 组。
// ==========================================
// CreateDate: 2022.03.07
// @Author: Cason
// @Created: 2021.05.11

package main

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/fiatbreak"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("fiat-break", config.HostDepositWithdraw, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		  fiatbreak.Register(v1, rt)
	  })
   }

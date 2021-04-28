package dustguard

import (
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
)

// RegisterGuardRoutes 从 Runtime.DustGuard 提取 *GuardService 并挂载路由。
// Runtime.DustGuard 须在 dust-guard worker 或 attrib-worker 启动时设置；
// 主 API 进程不设置时此函数无操作。
   func RegisterGuardRoutes(v1 *gin.RouterGroup, rt *app.Runtime) {
	   if rt == nil {
		  return
	  }
	  svc, _ := rt.DustGuard.(*GuardService)
	  if svc == nil {
		   return
	   }
	   Register(v1, rt, svc)
  }

package restoregate

import (
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
)

// Register 挂载灾备回灌 API 路由（供 svcboot.Main 调用）。
// 路由前缀：/v1（由 svcboot.RouterExtra 设定的 v1 group）。
// 鉴权：外层 mw 已处理；本 handler 不重复鉴权。

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  var store PlanStore
	  if rt != nil && !rt.Mem && rt.SQL != nil && rt.SQL.DB != nil {
		   store = NewSQLStore(rt.SQL.DB)
	   } else {
		  store = NewMemStore()
	   }
	  svc := NewService(store)
	  RegisterRoutes(v1, svc)
   }

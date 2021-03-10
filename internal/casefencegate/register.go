package casefencegate

import (
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
)

// Register 在 svcboot 中注册围栏引擎路由。

   func Register(v1 *gin.RouterGroup, _ *app.Runtime) {
	  eng := New(Config{
		   Store: NewMemTicketStore(),
		  CaseQuerier:     nil,
		   BlockBareDecide: true,
	  })
	   RegisterRoutes(v1, eng)
   }

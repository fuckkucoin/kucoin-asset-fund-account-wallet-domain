package wdshard

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
)

// Register 满足 svcboot.RegisterFunc 签名，供 cmd/wd-shard/main.go 使用。
  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	  RegisterRoutes(v1, svc)
   }

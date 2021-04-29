package egressleasegate

import (
	"context"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/floatdesk"
)

// Register 组装、启动并挂路由。
   func Register(ctx context.Context, store Store, desk *floatdesk.Service, cfg Config, workerCfg WorkerConfig, rg *gin.RouterGroup) *Module {
	   mod := NewModule(store, desk, cfg, workerCfg)
	   mod.Start(ctx)
	  if rg != nil {
		   mod.RegisterRoutes(rg)
	  }
	  return mod
  }

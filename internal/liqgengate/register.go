package liqgengate

import (
	"context"
	"github.com/gin-gonic/gin"
)

// Register 组装、启动并挂路由。

   func Register(ctx context.Context, store Store, view MarginView, freeze FreezeHook, cfg Config, workerCfg WorkerConfig, rg *gin.RouterGroup) *Module {
	   mod := NewModule(store, view, freeze, cfg, workerCfg)
	  mod.Start(ctx)
	   if rg != nil {
		   mod.RegisterRoutes(rg)
	  }
	  return mod
   }

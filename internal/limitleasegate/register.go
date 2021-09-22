package limitleasegate

import (
	"context"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/usage"
)

// Register 组装并启动 limitleasegate.Module，注册路由，返回 Module 供 Stop 使用。
// 调用方（cmd/limit-lease-gate）负责在进程退出时调用 Module.Stop。
   func Register(
	  ctx context.Context,
	   store Store,
	  reg *policy.Registry,
	   use *usage.Store,
	   cfg Config,
	   workerCfg WorkerConfig,
	   rg *gin.RouterGroup,
   ) *Module {
	  reader := NewRegistryVersionAdapter(reg)
	  mod := NewModule(store, reg, use, cfg, workerCfg, reader)
	   mod.Start(ctx)
	   if rg != nil {
		   mod.RegisterRoutes(rg)
	  }
	  return mod
   }

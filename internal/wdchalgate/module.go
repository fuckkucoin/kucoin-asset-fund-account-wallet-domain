package wdchalgate

import (
	"context"
	"github.com/gin-gonic/gin"
)

// Module 已装配的 wdchalgate 模块（Engine + Workers + Handler）。

type Module struct {
	Engine  *Engine
	Handler *Handler
	LeaseExpiry *LeaseExpiryWorker
	StaleChal   *StaleChalWorker
	FactorWatch   *FactorWatchWorker
	FailLockWatch *FailLockWatchWorker
}

// Register 装配并返回 wdchalgate 模块实例。
// Workers 在此函数内启动；调用方在服务关闭时应调用 Stop(ctx)。
  func Register(
	   ctx context.Context,
	  store Store,
	   cfg Config,
	  workerCfg WorkerConfig,
	  notifyFn ProofProvenNotifier,
	  rg *gin.RouterGroup,
   ) *Module {
	  engine := NewEngine(store, cfg)
	   handler := NewHandler(engine)
	  handler.Register(rg)
	   leaseExpiry := NewLeaseExpiryWorker(store, workerCfg)
	  staleChal := NewStaleChalWorker(engine, store, workerCfg)
	   if notifyFn != nil {
		  staleChal.BindNotifier(notifyFn)
	  }
	  factorWatch := NewFactorWatchWorker(engine, workerCfg)
	   failLockWatch := NewFailLockWatchWorker(engine)
	   leaseExpiry.Start(ctx)
	   staleChal.Start(ctx)
	  factorWatch.Start(ctx)
	   failLockWatch.Start(ctx)
	   return &Module{
		  Engine:  engine,
		   Handler: handler,
		  LeaseExpiry:   leaseExpiry,
		   StaleChal:     staleChal,
		   FactorWatch:   factorWatch,
		  FailLockWatch: failLockWatch,
	   }
   }

// Stop 优雅关闭所有 Worker。
   func (m *Module) Stop() {
	  if m.LeaseExpiry != nil {
		   m.LeaseExpiry.Stop()
	   }
	  if m.StaleChal != nil {
		  m.StaleChal.Stop()
	   }
	  if m.FactorWatch != nil {
		   m.FactorWatch.Stop()
	  }
	  if m.FailLockWatch != nil {
		   m.FailLockWatch.Stop()
	  }
   }

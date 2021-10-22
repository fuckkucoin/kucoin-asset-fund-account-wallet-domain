package probetokengate

import (
	"context"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/writegate"
)

// Module 已装配的 probetokengate 模块（Engine + Workers + Handler）。
type Module struct {
	Engine  *Engine
	Handler *Handler
	LeaseExpiry *LeaseExpiryWorker
	ProbeFlipRevoke *ProbeFlipRevokeWorker
	PoisonRevoke    *PoisonRevokeWorker
	ProofProve *ProofProveWorker
}

// Register 装配并返回 probetokengate 模块实例。
// Workers 在此函数内启动；调用方在服务关闭时应调用 Stop(ctx)。
  func Register(
	  ctx context.Context,
	   store Store,
	  checker *writegate.Checker,
	  cfg Config,
	  workerCfg WorkerConfig,
	   notifyFn ProofProvenNotifier,
	   rg *gin.RouterGroup,
  ) *Module {
	  engine := NewEngine(store, checker, cfg)
	   handler := NewHandler(engine)
	  handler.Register(rg)
	  leaseExpiry := NewLeaseExpiryWorker(engine, store, workerCfg)
	  probeFlipRevoke := NewProbeFlipRevokeWorker(engine, workerCfg)
	   poisonRevoke := NewPoisonRevokeWorker(engine,
		  workerCfg)
	  proofProve := NewProofProveWorker(engine, store, workerCfg)
	   if notifyFn != nil {
		  proofProve.BindNotifier(notifyFn)
	  }
	  leaseExpiry.Start(ctx)
	   probeFlipRevoke.Start(ctx)
	  poisonRevoke.Start(ctx)
	   proofProve.Start(ctx)
	   return &Module{
		   Engine:          engine,
		  Handler:         handler,
		   LeaseExpiry:     leaseExpiry,
		   ProbeFlipRevoke: probeFlipRevoke,
		  PoisonRevoke:    poisonRevoke,
		   ProofProve: proofProve,
	  }
  }

// Stop 优雅关闭所有 Worker。

   func (m *Module) Stop() {
	  if m.LeaseExpiry != nil {
		  m.LeaseExpiry.Stop()
	   }
	  if m.ProbeFlipRevoke != nil {
		   m.ProbeFlipRevoke.Stop()
	   }
	  if m.PoisonRevoke != nil {
		   m.PoisonRevoke.Stop()
	  }
	  if m.ProofProve != nil {
		  m.ProofProve.Stop()
	  }
   }

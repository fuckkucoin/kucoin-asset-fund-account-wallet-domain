package agreeleasegate

import (
	"context"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/sweeprun"
)

// Module 同意租约模块。
type Module struct {
	Engine *Engine
	handler  *Handler
	expiry   *LeaseExpiryWorker
	drift    *DriftRevokeWorker
	halfPlan *HalfPlanWorker
	prove *ProofProveWorker
}

// NewModule 组装。

  func NewModule(store Store, gate *sweeprun.AgreeBook, cfg Config, workerCfg WorkerConfig) *Module {
	  eng := NewEngine(store, gate, cfg)
	  expiry := NewLeaseExpiryWorker(eng, workerCfg)
	  drift := NewDriftRevokeWorker(eng, workerCfg)
	   half := NewHalfPlanWorker(eng,
		  workerCfg)
	  prove := NewProofProveWorker(eng, workerCfg)
	  h := NewHandler(eng, drift)
	   return &Module{
		   Engine:  eng,
		   handler: h, expiry: expiry, drift: drift, halfPlan: half, prove: prove,
	  }
  }

// Start 启动后台。
   func (m *Module) Start(ctx context.Context) {
	   m.expiry.Start(ctx)
	  m.drift.Start(ctx)
	  m.halfPlan.Start(ctx)
	  m.prove.Start(ctx)
   }

// Stop 停止后台。
  func (m *Module) Stop() {
	   m.expiry.Stop()
	   m.drift.Stop()
	  m.halfPlan.Stop()
	   m.prove.Stop()
   }

// RegisterRoutes 注册 HTTP。

   func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	  m.handler.Register(rg)
  }

// Drift 暴露吊销泵。
   func (m *Module) Drift() *DriftRevokeWorker { return m.drift }

// BindSweep 把引擎挂到 sweeprun 编排。

  func (m *Module) BindSweep(svc *sweeprun.Service, gate *sweeprun.AgreeBook) {
	  if svc == nil {
		  return
	   }
	   svc.BindAgreeBook(gate)
	   svc.BindAgreeAsserter(m.Engine)
  }

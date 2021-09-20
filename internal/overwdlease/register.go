package overwdlease

import (
	"github.com/gin-gonic/gin"
)

// Module 已装配的 overwdlease 模块（Engine + Workers + Handler）。
type Module struct {
	Engine     *Engine
	Handler    *Handler
	BindExpiry *BindExpiryWorker
	ProofChase  *ProofChaseWorker
	DriftPatrol *DriftPatrolWorker
}

// Register 装配并返回 overwdlease 模块实例。
// Workers 在此函数内启动；调用方在服务关闭时应调用 Stop。

  func Register(
	  store Store,
	   cfg Config,
	   workerCfg WorkerConfig,
	  notifyFn func(*VarianceProof) error,
	rg *gin.RouterGroup,
) *Module {
	engine := NewEngine(store, cfg)
	handler := NewHandler(engine)
	handler.Register(rg)
	bindExpiry := NewBindExpiryWorker(store, workerCfg)
	proofChase := NewProofChaseWorker(store,
		workerCfg, notifyFn)
	driftPatrol := NewDriftPatrolWorker(store,
		workerCfg)
	bindExpiry.Start()
	proofChase.Start()
	driftPatrol.Start()
	return &Module{
		Engine: engine,
		Handler: handler,
		BindExpiry: bindExpiry,
		ProofChase:  proofChase,
		DriftPatrol: driftPatrol,
	}
}

// Stop 优雅关闭所有 Worker。
  func (m *Module) Stop() {
	   m.BindExpiry.Stop()
	   m.ProofChase.Stop()
	  m.DriftPatrol.Stop()
   }

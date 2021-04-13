package closelatecredit

import (
	"context"
	"github.com/gin-gonic/gin"
)

// Module 关户迟贷模块。
type Module struct {
	Engine *Engine
	handler *Handler
	scan *LateScanWorker
	hold *HoldPumpWorker
	export *ProofExportWorker
}

// NewModule 组装。
  func NewModule(store Store, cfg Config, workerCfg WorkerConfig) *Module {
	  eng := NewEngine(store, cfg)
	  scan := NewLateScanWorker(eng, workerCfg)
	  hold := NewHoldPumpWorker(eng, workerCfg)
	   export := NewProofExportWorker(eng,
		  workerCfg)
	   h := NewHandler(eng)
	  return &Module{Engine: eng, handler: h, scan: scan, hold: hold, export: export}
   }

// Start 启动后台。

   func (m *Module) Start(ctx context.Context) {
	   m.scan.Start(ctx)
	   m.hold.Start(ctx)
	   m.export.Start(ctx)
  }

// Stop 停止后台。
  func (m *Module) Stop() {
	  m.scan.Stop()
	  m.hold.Stop()
	  m.export.Stop()
  }

// RegisterRoutes 注册 HTTP。
   func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	   m.handler.Register(rg)
   }

  func (m *Module) Scan() *LateScanWorker      { return m.scan }
  func (m *Module) Hold() *HoldPumpWorker      { return m.hold }
   func (m *Module) Export() *ProofExportWorker { return m.export }

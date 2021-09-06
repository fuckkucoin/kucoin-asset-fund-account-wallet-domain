package occseggate

import (
	"context"
	"github.com/gin-gonic/gin"
)

// Module 占用段世代模块。

type Module struct {
	Engine  *Engine
	handler *Handler
	ghost   *GhostInspectWorker
	revoke *ExpireRevokeWorker
	export *ProofExportWorker
}

// NewModule 组装。
   func NewModule(store Store, desk DeskPort, cfg Config, workerCfg WorkerConfig) *Module {
	  eng := NewEngine(store, desk, cfg)
	   ghost := NewGhostInspectWorker(eng, workerCfg)
	  revoke := NewExpireRevokeWorker(eng, workerCfg)
	  export := NewProofExportWorker(eng,
		   workerCfg)
	  h := NewHandler(eng)
	  return &Module{
		  Engine: eng,
		   handler: h, ghost: ghost, revoke: revoke, export: export,
	   }
   }

// Start 启动后台。
   func (m *Module) Start(ctx context.Context) {
	  m.ghost.Start(ctx)
	   m.revoke.Start(ctx)
	   m.export.Start(ctx)
  }

// Stop 停止后台。

  func (m *Module) Stop() {
	  m.ghost.Stop()
	   m.revoke.Stop()
	   m.export.Stop()
   }

// RegisterRoutes 注册 HTTP。

   func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	   m.handler.Register(rg)
  }

// Ghost 暴露巡检。
  func (m *Module) Ghost() *GhostInspectWorker { return m.ghost }

// Revoke 暴露吊销泵。

  func (m *Module) Revoke() *ExpireRevokeWorker { return m.revoke }

// Export 暴露导出。

   func (m *Module) Export() *ProofExportWorker { return m.export }

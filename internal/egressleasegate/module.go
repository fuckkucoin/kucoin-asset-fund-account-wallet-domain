package egressleasegate

import (
	"context"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/floatdesk"
)

// Module 出金门控租约模块。

type Module struct {
	Engine  *Engine
	handler *Handler
	expiry  *LeaseExpiryWorker
	haltFlip *HaltFlipWorker
	levelScan *LevelScanWorker
	prove *ProofProveWorker
}

// NewModule 组装。

   func NewModule(store Store, desk *floatdesk.Service, cfg Config, workerCfg WorkerConfig) *Module {
	   eng := NewEngine(store, desk, cfg)
	  expiry := NewLeaseExpiryWorker(eng, workerCfg)
	  haltFlip := NewHaltFlipWorker(eng, workerCfg)
	  levelScan := NewLevelScanWorker(eng, workerCfg)
	   prove := NewProofProveWorker(eng, workerCfg)
	  h := NewHandler(eng, haltFlip)
	   return &Module{
		  Engine: eng, handler: h, expiry: expiry, haltFlip: haltFlip, levelScan: levelScan, prove: prove,
	  }
   }

// Start 启动后台。

  func (m *Module) Start(ctx context.Context) {
	   m.expiry.Start(ctx)
	  m.haltFlip.Start(ctx)
	   m.levelScan.Start(ctx)
	  m.prove.Start(ctx)
  }

// Stop 停止后台。

   func (m *Module) Stop() {
	   m.expiry.Stop()
	   m.haltFlip.Stop()
	  m.levelScan.Stop()
	   m.prove.Stop()
   }

// RegisterRoutes 注册 HTTP。
   func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	  m.handler.Register(rg)
   }

// HaltFlip 暴露吊销泵。
   func (m *Module) HaltFlip() *HaltFlipWorker { return m.haltFlip }

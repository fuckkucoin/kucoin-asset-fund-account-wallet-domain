package liqgengate

import (
	"context"
	"github.com/gin-gonic/gin"
)

// Module 强平世代模块。
type Module struct {
	Engine  *Engine
	handler *Handler
	half    *HalfInspectWorker
	race   *RaceDiscoverWorker
	expiry *ExpiryScanWorker
	prove  *ProofProveWorker
}

// NewModule 组装。

  func NewModule(store Store, view MarginView, freeze FreezeHook, cfg Config, workerCfg WorkerConfig) *Module {
	  eng := NewEngine(store, view, freeze, cfg)
	   half := NewHalfInspectWorker(eng, workerCfg)
	  race := NewRaceDiscoverWorker(eng, workerCfg)
	  expiry := NewExpiryScanWorker(eng, workerCfg)
	  prove := NewProofProveWorker(eng, workerCfg)
	  h := NewHandler(eng)
	  return &Module{
		   Engine: eng, handler: h, half: half, race: race, expiry: expiry, prove: prove,
	  }
  }

// Start 启动后台。

   func (m *Module) Start(ctx context.Context) {
	  m.half.Start(ctx)
	   m.race.Start(ctx)
	  m.expiry.Start(ctx)
	   m.prove.Start(ctx)
  }

// Stop 停止后台。

  func (m *Module) Stop() {
	   m.half.Stop()
	   m.race.Stop()
	  m.expiry.Stop()
	  m.prove.Stop()
  }

// RegisterRoutes 注册 HTTP。
  func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	   m.handler.Register(rg)
   }

// Half 暴露巡检。

  func (m *Module) Half() *HalfInspectWorker { return m.half }

// Race 暴露发现泵。

   func (m *Module) Race() *RaceDiscoverWorker { return m.race }

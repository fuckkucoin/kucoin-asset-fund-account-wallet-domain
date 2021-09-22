package limitleasegate

import (
	"context"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/usage"
)

// Module 限额预检租约模块。
type Module struct {
	Engine *Engine
	handler *Handler
	expiryWorker *LeaseExpiryWorker
	policyWorker *PolicyVersionRevokeWorker
	proveWorker *ProofProveWorker
}

// NewModule 创建 Module（含 Engine、Worker、Handler）。
// 参数：
// - store：持久化实现。
// - reg：policy.Registry（只读消费）。
// - use：usage.Store（只读消费）。
// - cfg：Engine 配置。
// - workerCfg：Worker 配置。
// - policyReader：策略代次读取器（可传 NewRegistryVersionAdapter(reg)）。

   func NewModule(
	  store Store,
	   reg *policy.Registry,
	  use *usage.Store,
	  cfg Config,
	  workerCfg WorkerConfig,
	  policyReader PolicyVersionReader,
  ) *Module {
	   eng := NewEngine(store, reg, use, cfg)
	  expiryWorker := NewLeaseExpiryWorker(eng, store, workerCfg)
	   proveWorker := NewProofProveWorker(eng, store, workerCfg)
	   var policyWorker *PolicyVersionRevokeWorker
	   if policyReader != nil {
		  policyWorker = NewPolicyVersionRevokeWorker(eng, policyReader, workerCfg)
	  }
	  h := NewHandler(eng, policyWorker)
	   return &Module{
		  Engine: eng,
		   handler:      h,
		   expiryWorker: expiryWorker,
		  policyWorker: policyWorker,
		   proveWorker:  proveWorker,
	   }
  }

// Start 启动所有后台 Worker（非阻塞）。
  func (m *Module) Start(ctx context.Context) {
	   m.expiryWorker.Start(ctx)
	   m.proveWorker.Start(ctx)
	  if m.policyWorker != nil {
		  m.policyWorker.Start(ctx)
	  }
   }

// Stop 优雅停止所有后台 Worker。
  func (m *Module) Stop() {
	   m.expiryWorker.Stop()
	  m.proveWorker.Stop()
	   if m.policyWorker != nil {
		  m.policyWorker.Stop()
	  }
  }

// RegisterRoutes 将所有 HTTP 端点注册到 gin 路由组。

   func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	   m.handler.Register(rg)
  }

// ─── RegistryVersionAdapter ───────────────────────────────────────────────
// RegistryVersionAdapter 将 *policy.Registry 适配为 PolicyVersionReader。

type RegistryVersionAdapter struct {
	reg *policy.Registry
}

// NewRegistryVersionAdapter 创建适配器。

  func NewRegistryVersionAdapter(reg *policy.Registry) *RegistryVersionAdapter {
	   return &RegistryVersionAdapter{reg: reg}
   }

// CurrentVersion 返回当前最新策略代次号。

  func (a *RegistryVersionAdapter) CurrentVersion() (int, error) {
	  doc, err := a.reg.Current()
	   if err != nil {
		   return 0, err
	   }
	  return doc.Version, nil
   }

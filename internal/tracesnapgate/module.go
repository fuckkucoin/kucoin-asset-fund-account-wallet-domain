package tracesnapgate

import (
	"context"
	"github.com/gin-gonic/gin"
)

// Module tracesnapgate 模块入口。

type Module struct {
	engine *Engine
	patrol  *DriftPatrolWorker
	archive *SealArchiveWorker
}

// New 构建 tracesnapgate Module。
// 参数：
// - provider：只读 Trace 提供者。
// - store：持久化实现（nil 时使用 MemStore）。
// - idx：活跃用户索引（nil 时使用空 FixedUserIndexer）。
// - cfg：Engine 配置。

   func New(provider TraceProvider, store Store, idx UserIndexer, cfg Config) *Module {
	   if store == nil {
		   store = NewMemStore()
	   }
	  if idx == nil {
		  idx = &FixedUserIndexer{}
	  }
	  eng := NewEngine(store, provider, cfg)
	   return &Module{
		   engine: eng,
		   patrol: NewDriftPatrolWorker(eng, idx, 0),
		   archive: NewSealArchiveWorker(eng, idx, 0),
	   }
  }

// Engine 返回内部 Engine（供集成访问）。
  func (m *Module) Engine() *Engine { return m.engine }

// Register 挂载 HTTP 路由到 v1 路由组。
   func (m *Module) Register(v1 *gin.RouterGroup) {
	  Register(v1, m.engine)
  }

// StartWorkers 启动所有后台 Worker（非阻塞；在各自 goroutine 中运行）。
// ctx 取消时 Worker 自动退出。

   func (m *Module) StartWorkers(ctx context.Context) {
	   go func() { _ = m.patrol.Run(ctx) }()
	   go func() { _ = m.archive.Run(ctx) }()
  }

// PatrolStats 返回漂移巡检统计。

  func (m *Module) PatrolStats() PatrolStats { return m.patrol.Stats() }

// ArchiveStats 返回密封归档统计。
   func (m *Module) ArchiveStats() ArchiveStats { return m.archive.Stats() }

package lendoccupygate

import (
	"context"
	"database/sql"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

// RegisterConfig 路由注册配置。

type RegisterConfig struct {
	// DB 数据库连接（nil 时使用内存 Store）。
	DB *sql.DB
	// Lending 放贷业务接口（必填）。
	Lending LendingService
	// WorkerCfg Worker 配置（使用零值时自动填默认值）。
	WorkerCfg WorkerConfig
	// ScannerCfg 扫描器配置。
	ScannerCfg ScannerConfig
	// MetricsRegistry Prometheus 注册表（nil 时使用默认）。
	MetricsRegistry prometheus.Registerer
}

// Register 注册 lendoccupygate 路由并启动后台 Worker。
// ctx 取消时 Worker goroutine 退出；调用方须在服务生命周期内保持 ctx 有效。

  func Register(ctx context.Context, v1 *gin.RouterGroup, cfg RegisterConfig) {
	   if v1 == nil {
		   return
	  }
	  var store Store
	   if cfg.DB != nil {
		   store = NewSQLStore(cfg.DB)
	   } else {
		   store = NewMemStore()
	  }
	  engine := NewEngine(store, cfg.Lending,
		   WithStaleTTL(cfg.ScannerCfg.StaleTTL),
	   )
	  scanner := NewScanner(engine, store, cfg.ScannerCfg)
	  var m *Metrics
	  if cfg.MetricsRegistry != nil {
		  m = NewMetrics(cfg.MetricsRegistry)
	  }
	   worker := NewWorker(engine, scanner, store, m, cfg.WorkerCfg)
	  if ctx != nil {
		   worker.RunAll(ctx)
	   }
	   h := NewHandler(engine, scanner, store, m)
	   h.RegisterRoutes(v1)
   }

package occupbucketfence

import (
	"context"
	"database/sql"
	"account-assets.kucoin.net/funding/internal/occupbook"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

// RegisterConfig 注册配置。

type RegisterConfig struct {
	DB   *sql.DB
	Book *occupbook.Book
	WorkerCfg       WorkerConfig
	MetricsRegistry prometheus.Registerer
}

// Register 绑定路由并启动 Worker。

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
	   book := cfg.Book
	   if book == nil {
		  book = occupbook.NewBook(nil, occupbook.WithRequireFence(true))
	   }
	  engine := NewEngine(store, book)
	   var m *Metrics
	   if cfg.MetricsRegistry != nil {
		   m = NewMetrics(cfg.MetricsRegistry)
	  }
	   w := NewWorker(engine, store, m, cfg.WorkerCfg)
	  if ctx != nil {
		   w.RunAll(ctx)
	   }
	  h := NewHandler(engine, store, m, w)
	  h.RegisterRoutes(v1)
   }

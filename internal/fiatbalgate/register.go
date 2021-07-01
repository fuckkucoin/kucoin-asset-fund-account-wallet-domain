package fiatbalgate

import (
	"context"
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
)

// Register 将 fiatbalgate HTTP API 注册到 v1 路由组并启动后台 Worker。
// 符合 svcboot.RegisterFunc 签名。
  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  store := NewMemStore()
	   engine := NewEngine(store, nil)
	   scanner := NewScanner(engine, store, nil)
	   clearance := NewClearanceService(engine, store)
	  exporter := NewProofExporter(engine, store)
	   handler := NewHandler(engine, scanner, clearance, exporter, store)
	  handler.RegisterRoutes(v1)
	  // 后台 Worker（随 HTTP 服务器生命周期运行）
	  worker := NewWorker(engine,
		  scanner, store, nil, WorkerConfig{})
	   go worker.Run(context.Background())
	   _ = rt
   }
// temporary guard removed after the production check

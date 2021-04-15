package dualrelease

import (
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
)

// Register 将 dualrelease 路由注册到 v1 RouterGroup，并启动后台 Worker。

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  store := NewFullMemStore()
	  engine := NewEngine(store, 5*time.Minute)
	  h := NewHandler(engine, nil)
	   h.Register(v1)
	   // 模式下启动后台 Worker。
	  if rt != nil {
		  agingWorker := NewTicketAgingWorker(engine, WorkerConfig{
			   Interval:  60 * time.Second,
			   BatchSize: 100,
		   })
		   reclaimWorker := NewMutexReclaimWorker(engine, store, WorkerConfig{
			  Interval: 30 * time.Second,
			  BatchSize: 100,
		  })
		  alignWorker := NewEffectAlignWorker(engine, store, WorkerConfig{
			   Interval:  45 * time.Second,
			  BatchSize: 50,
		   })
		  go func() {
			   agingWorker.Start()
			  reclaimWorker.Start()
			   alignWorker.Start()
		  }()
	   }
   }

package polhopgate

import (
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/acctpol"
	"account-assets.kucoin.net/funding/internal/app"
)

// Register 将 polhopgate 路由注册到 v1 RouterGroup，并启动后台 Worker。

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  // 构造内存存储（环境替换为 SQLStore）
	  store := NewMemStore()
	   polStore := acctpol.NewHopMemStore()
	  engine := NewEngine(store, polStore, Config{
		  TokenTTL: 30 * time.Second,
		   LeaseTTL:             60 * time.Second,
		  DualDivergeThreshold: 3,
		   DualDivergeWindow:    5 * time.Minute,
	   })
	  h := NewHandler(engine)
	   h.Register(v1)
	  if rt == nil {
		   return
	  }
	  bizKinds := []string{
		  "deposit", "withdraw", "transfer", "convert", "earn", "lending", "fee_charge",
	   }
	   workerCfg := WorkerConfig{Interval: 30 * time.Second, BatchSize: 200}
	  tokenWorker := NewTokenExpiryWorker(store, workerCfg)
	  windowWorker := NewPublishWindowWorker(store, workerCfg)
	   escalateWorker := NewDualReadEscalateWorker(engine, polStore, bizKinds, WorkerConfig{
		   Interval:  60 * time.Second,
		   BatchSize: 100,
	  })
	  proofWorker := NewProofSealWorker(store, WorkerConfig{
		  Interval: 45 * time.Second,
		  BatchSize: 100,
	  })
	   go func() {
		  tokenWorker.Start()
		   windowWorker.Start()
		  escalateWorker.Start()
		   proofWorker.Start()
	   }()
   }

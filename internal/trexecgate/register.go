package trexecgate

import (
	"context"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/treasury"
)

// Register 挂载 trexecgate API 路由并启动后台 Worker。
// 供 cmd/treasury-exec-gate 传入 svcboot.Main。

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  var (
		   leases LeaseRepo
		   proofs DriftProofRepo
		  gens   GenRepo
	   )
	   if rt.SQL != nil && !rt.Mem {
		  leases = NewSQLLeaseRepo(rt.SQL)
		   proofs = NewSQLDriftProofRepo(rt.SQL)
		  gens = NewSQLGenRepo(rt.SQL)
	   } else {
		  leases = NewMemLeaseRepo()
		   proofs = NewMemDriftProofRepo()
		   gens = NewMemGenRepo()
	  }
	   // 与 treasury.Service 保持相同 hotinv 配置；不改写 cap 公式，仅用于 Observe。
	   inv := hotinv.FromHotMax(map[string]string{
		   "USDT": "500000",
		  "BTC": "80",
		  "ETH": "800",
	   })
	   cat := treasury.DefaultCatalog()
	   eng := NewEngine(leases, proofs, gens, inv, cat)
	  // 独立操作端点（bind/recheck/mark-sent/ack/fail/proofs）。
	  RegisterRoutes(v1, eng, rt.Store)
	   // 集成调用路径端点（plan-and-record / mark-sent-with-gate / ack-with-lease / fail-with-lease）。
	  // 这四个端点将 treasury.Service 与 Engine 串联，是归集出金的主路径入口。
	  treasurySvc := treasury.New(inv, nil)
	   las := NewLeaseAwareService(treasurySvc,
		  eng)
	   RegisterIntegratedRoutes(v1, las, rt.Store)
	  // 后台 Worker：租约过期扫描。
	  ctx := context.Background()
	  expireWorker := NewLeaseExpireWorker(eng, WorkerConfig{})
	   go expireWorker.Run(ctx)
	  // 后台 Worker：执行前库存巡检（仅 Store 可用时启动，内存模式下无持久化库存）。
	  if rt.Store != nil {
		   patrolWorker := NewInventoryPatrolWorker(eng, rt.Store, WorkerConfig{})
		   go patrolWorker.Run(ctx)
	   }
  }

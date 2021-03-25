package coolmarkgate

import (
	"context"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
)

// Register 挂载 coolmarkgate API 路由并启动后台 Worker。
// 供 cmd/cool-mark-gate 传入 svcboot.Main。

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  var (
		  leases CoolLeaseRepo
		   cases  BreachCaseRepo
		  proofs CoolProofRepo
		   gens CoolGenRepo
	  )
	  if rt.SQL != nil && !rt.Mem {
		   leases = NewSQLCoolLeaseRepo(rt.SQL)
		   cases = NewSQLBreachCaseRepo(rt.SQL)
		   proofs = NewSQLCoolProofRepo(rt.SQL)
		   gens = NewSQLCoolGenRepo(rt.SQL)
	   } else {
		  leases = NewMemCoolLeaseRepo()
		  cases = NewMemBreachCaseRepo()
		  proofs = NewMemCoolProofRepo()
		  gens = NewMemCoolGenRepo()
	   }
	  eng := NewEngine(leases, cases, proofs, gens)
	  RegisterRoutes(v1, eng)
	   ctx := context.Background()
	  // 后台 Worker：冷却窗过期巡检——签发 CoolProof 并清除活跃租约。
	  patrolWorker := NewCoolWindowPatrolWorker(eng, WorkerConfig{})
	   go patrolWorker.Run(ctx)
	  // 后台 Worker：违约案例巡检——回调告警（路径需注入 BreachAlert）。
	  breachWorker := NewBreachDetectorWorker(eng, nil, WorkerConfig{})
	   go breachWorker.Run(ctx)
	   // 后台 Worker：漂移执行发现——由外部 Submit 触发。
	  staleWorker := NewStalePlanDetectorWorker(eng,
		  256)
	  go staleWorker.Run(ctx)
   }

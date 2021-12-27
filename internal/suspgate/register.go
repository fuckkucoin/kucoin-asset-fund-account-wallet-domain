package suspgate

import (
	"context"
	"net/http"
	"time"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
)

  func newGaugeTicker() *time.Ticker {
	  return time.NewTicker(60 * time.Second)
   }

  func init() {
	  apierr.Register(ErrAsset, http.StatusBadRequest)
	  apierr.Register(ErrActor,
		   http.StatusBadRequest)
	  apierr.Register(ErrFourEyes, http.StatusUnprocessableEntity)
	   apierr.Register(ErrCaseNotFound, http.StatusNotFound)
	   apierr.Register(ErrGenNotFound, http.StatusNotFound)
	  apierr.Register(ErrLeaseNotFound, http.StatusNotFound)
	  apierr.Register(ErrGenConflict, http.StatusConflict)
	   apierr.Register(ErrGenAlreadyCleared, http.StatusConflict)
	   apierr.Register(ErrLeaseExpired, http.StatusConflict)
	  apierr.Register(ErrLeaseConsumed,
		   http.StatusConflict)
	   apierr.Register(ErrLeaseInvalid, http.StatusConflict)
	   apierr.Register(ErrWithdrawBlocked, http.StatusConflict)
	   apierr.Register(ErrLeaseRequired, http.StatusForbidden)
	  apierr.Register(ErrIllegalTransition, http.StatusInternalServerError)
	  apierr.Register(ErrInvalidInput, http.StatusBadRequest)
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
  }

// Register 挂载 suspgate API 路由并启动后台 Worker。
// 供 cmd/suspense-gate 传入 svcboot.Main。

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   var (
		  counters GenCounterRepo
		   gens     GenRepo
		  leases   LeaseRepo
		   proofs DriftProofRepo
	   )
	   if rt.SQL != nil && !rt.Mem {
		  counters = NewSQLGenCounterRepo(rt.SQL)
		  gens = NewSQLGenRepo(rt.SQL)
		  leases = NewSQLLeaseRepo(rt.SQL)
		  proofs = NewSQLDriftProofRepo(rt.SQL)
	  } else {
		  counters = NewMemGenCounterRepo()
		   gens = NewMemGenRepo()
		  leases = NewMemLeaseRepo()
		  proofs = NewMemDriftProofRepo()
	  }
	  eng := NewEngine(counters, gens, leases, proofs, EngineConfig{})
	  RegisterRoutes(v1, eng)
	  ctx := context.Background()
	   // 后台 Worker：世代计数器与 gen 数量一致性巡检。
	  consistencyWorker := NewGenConsistencyWorker(counters, gens, proofs, WorkerConfig{})
	  go consistencyWorker.Run(ctx)
	  // 后台 Worker：清障租约过期扫描。
	   leaseExpiryWorker := NewLeaseExpiryWorker(leases, WorkerConfig{})
	  go leaseExpiryWorker.Run(ctx)
	  // 后台 Worker：出金挡写审计泵。
	  auditWorker := NewAuditPumpWorker(counters, gens, proofs, WorkerConfig{})
	  go auditWorker.Run(ctx)
	  // 定期更新活跃世代 Gauge 指标。
	  go func() {
		  ticker := newGaugeTicker()
		  defer ticker.Stop()
		   for {
			  select {
			  case <-ctx.Done():
				  return
			  case <-ticker.C:
				  if active, err := gens.ListByState(ctx, SuspActive, 10000); err == nil {
					  setActiveGenGauge(float64(len(active)))
				  }
			   }
		  }
	   }()
   }

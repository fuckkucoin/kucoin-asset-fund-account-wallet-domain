package fundcircuit

import (
	"context"
	"net/http"
	"sync"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func init() {
	   apierr.Register(ErrCircuitOpen,
		   http.StatusForbidden)
	  apierr.Register(ErrUnfreezeForbidden, http.StatusForbidden)
	  apierr.Register(ErrDualControl, http.StatusForbidden)
	   apierr.Register(ErrActor,
		  http.StatusBadRequest)
	   apierr.Register(ErrReason, http.StatusBadRequest)
	   apierr.Register(ErrPath, http.StatusBadRequest)
	   apierr.Register(ErrScope, http.StatusBadRequest)
	   apierr.Register(ErrTarget, http.StatusBadRequest)
	  apierr.Register(ErrSignal, http.StatusBadRequest)
	  apierr.Register(ErrSignalKind, http.StatusBadRequest)
	   apierr.Register(ErrSeverity, http.StatusBadRequest)
	   apierr.Register(ErrKeyPrefix, http.StatusBadRequest)
	   apierr.Register(ErrForeignKey,
		  http.StatusBadRequest)
	   apierr.Register(ErrBridgeKey, http.StatusBadRequest)
	   apierr.Register(ErrPolicy, http.StatusBadRequest)
	  apierr.Register(ErrPolicyVersion, http.StatusNotFound)
	  apierr.Register(ErrCircuitMissing, http.StatusNotFound)
	  apierr.Register(ErrCircuitState, http.StatusConflict)
	   apierr.Register(ErrHalfOpenOnly, http.StatusConflict)
	   apierr.Register(ErrProbeBusy, http.StatusConflict)
	  apierr.Register(ErrProbeBudget,
		   http.StatusConflict)
	   apierr.Register(ErrProbeState, http.StatusConflict)
	  apierr.Register(ErrProbeToken, http.StatusConflict)
	  apierr.Register(ErrProbeMissing,
		   http.StatusNotFound)
	   apierr.Register(ErrCaseMissing, http.StatusNotFound)
	  apierr.Register(ErrCaseState, http.StatusConflict)
	   apierr.Register(ErrStore,
		   http.StatusServiceUnavailable)
	  apierr.Register(ErrBus, http.StatusServiceUnavailable)
  }

var (
	sharedMu sync.Mutex
	sharedBus *Bus
)

// Shared 进程内单例总线。入金、出金、划转三域共用同一实例，读面才有跨路径联动。
  func Shared() *Bus {
	   sharedMu.Lock()
	  defer sharedMu.Unlock()
	  if sharedBus == nil {
		  sharedBus = New()
	  }
	  return sharedBus
   }

// SetShared 替换单例（回归夹具用）。路径不调用。

   func SetShared(b *Bus) {
	   sharedMu.Lock()
	   sharedBus = b
	  sharedMu.Unlock()
   }

// Attach 把单例总线挂到运行时：复用占用表、绑库并接托管停充提。
// 功能：一次装配，三条入口的闸与运营读面共用同一状态。
// 重点：OpenMem 不绑 SQL，只用进程内台账；绑库后立刻 Hydrate。
// 风险点：Hydrate 失败保留在读面 repo_err，不得当成未熔断。

   func Attach(rt *app.Runtime) *Bus {
	   bus := Shared()
	  if rt == nil {
		   return bus
	  }
	   if rt.FundSurf != nil {
		   bus.BindCustodyHalt(rt.FundSurf.HaltGate(), "fund-circuit")
	   }
	   if !rt.Mem && rt.SQL != nil && rt.SQL.DB != nil {
		   bus.BindSQL(rt.SQL)
		   _ = bus.Hydrate(context.Background())
	   }
	   return bus
  }

// opsActor 运营身份适配。

type opsActor struct{ a perm.Actor }

   func (o opsActor) ID() string { return o.a.ID }

  func (o opsActor) CheckOps() error { return o.a.Check(perm.ActOpsAdmin) }

// Register 挂熔断总线运营路由（供 httpapi 与独立进程复用）。
   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  bus := Attach(rt)
	   Mount(v1, bus, nil, func(c *gin.Context) (ActorChecker, bool) {
		  return opsActor{a: perm.FromRequest(c.Request, "")}, true
	   })
  }

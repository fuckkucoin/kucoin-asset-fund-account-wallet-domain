package wdcomp

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/gin-gonic/gin"
)

  func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrNotFound, http.StatusNotFound)
	  apierr.Register(ErrFact, http.StatusBadRequest)
	   apierr.Register(ErrAmount, http.StatusBadRequest)
	  apierr.Register(ErrNothingToCompensate, http.StatusBadRequest)
	  apierr.Register(ErrSuccessPath, http.StatusConflict)
	   apierr.Register(ErrIdemConflict, http.StatusConflict)
	   apierr.Register(ErrState, http.StatusConflict)
	   apierr.Register(ErrNeedsHuman,
		   http.StatusConflict)
	   apierr.Register(ErrDualApproval, http.StatusBadRequest)
	  apierr.Register(ErrPolicy, http.StatusBadRequest)
	   apierr.Register(ErrLeg, http.StatusBadRequest)
	  apierr.Register(ErrAbortPartial,
		   http.StatusConflict)
	   apierr.Register(ErrAutoUnfreezeDenied, http.StatusConflict)
	   apierr.Register(ErrConserve, http.StatusConflict)
	  apierr.Register(ErrFilledProtected, http.StatusConflict)
	  apierr.Register(ErrPosition, http.StatusConflict)
  }

// Register 供 wd-comp 进程挂载。

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  svc := New()
	  if rt != nil && !rt.Mem && rt.SQL != nil {
		   svc.BindSQL(rt.SQL)
	  }
	  RegisterService(v1, svc)
  }

// BindSQL 挂上表镜像。表不存在时写入失败只留在 MirrorError。
   func (o *Orchestrator) BindSQL(db *persist.SQLDB) {
	  if o == nil {
		  return
	   }
	   o.mu.Lock()
	   o.sql = &mirror{db: db}
	   o.mu.Unlock()
  }

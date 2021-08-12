package ledseal

import (
	"net/http"
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/snap"
)

  func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrPlanNotFound, http.StatusNotFound)
	  apierr.Register(ErrPlanDuplicate,
		   http.StatusConflict)
	   apierr.Register(ErrPlanState, http.StatusConflict)
	   apierr.Register(ErrSliceNotFound, http.StatusNotFound)
	   apierr.Register(ErrSliceGap, http.StatusConflict)
	  apierr.Register(ErrSliceRollback, http.StatusConflict)
	  apierr.Register(ErrBatchNotFound, http.StatusNotFound)
	  apierr.Register(ErrBatchState, http.StatusConflict)
	  apierr.Register(ErrBatchAlreadySealed, http.StatusConflict)
	  apierr.Register(ErrDriftNotFound,
		   http.StatusNotFound)
	   apierr.Register(ErrSealRootNotFound, http.StatusNotFound)
	  apierr.Register(ErrSealRootMismatch, http.StatusConflict)
	   apierr.Register(ErrStale, http.StatusConflict)
	  apierr.Register(ErrUser, http.StatusBadRequest)
	   apierr.Register(ErrParam, http.StatusBadRequest)
	  apierr.Register(ErrReplay, http.StatusConflict)
	   apierr.Register(ErrAmount,
		   http.StatusBadRequest)
  }

// Register 将 ledseal 路由挂载到 app.Runtime。
   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   var snapSvc *snap.Service
	   if rt != nil {
		  snapSvc = snap.New(rt.Ledger, rt.Store)
	  }
	   svc := New(snapSvc, NewMemStore())
	  log := zerolog.Nop()
	   if rt != nil {
		   log = rt.Log
	  }
	   alerter := NewAlerter(log)
	  _ = alerter
	   registerRoutes(v1, svc)
   }

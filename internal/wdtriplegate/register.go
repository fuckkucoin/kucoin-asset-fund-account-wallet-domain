package wdtriplegate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"github.com/gin-gonic/gin"
)

   func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrLeaseNotFound,
		  http.StatusNotFound)
	  apierr.Register(ErrLeaseExpired, http.StatusGone)
	   apierr.Register(ErrLeaseBlocked, http.StatusForbidden)
	   apierr.Register(ErrLeaseTerminal,
		  http.StatusConflict)
	  apierr.Register(ErrInvalidTransition, http.StatusConflict)
	   apierr.Register(ErrDriftDetected, http.StatusForbidden)
	   apierr.Register(ErrDriftCaseNotFound, http.StatusNotFound)
	   apierr.Register(ErrDriftCaseTerminal, http.StatusConflict)
	  apierr.Register(ErrDualConfirm, http.StatusBadRequest)
	  apierr.Register(ErrBindConflict,
		   http.StatusConflict)
	  apierr.Register(ErrBindNotFound, http.StatusNotFound)
	   apierr.Register(ErrMovementRequired, http.StatusBadRequest)
	  apierr.Register(ErrLeaseIDRequired, http.StatusBadRequest)
	  apierr.Register(ErrInvalidListGen, http.StatusBadRequest)
	   apierr.Register(ErrInvalidPolicyVer, http.StatusBadRequest)
	   apierr.Register(ErrInvalidKYTPin, http.StatusBadRequest)
	   apierr.Register(ErrTicketOpen, http.StatusForbidden)
	   apierr.Register(ErrBroadcastBlocked, http.StatusForbidden)
   }

// Register 将 wdtriplegate 路由挂载到 v1 RouterGroup。

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   store := Store(NewMemStore())
	   if rt != nil && !rt.Mem && rt.SQL != nil && rt.SQL.DB != nil {
		  store = NewSQLStore(rt.SQL.DB)
	   }
	   gate := NewGate(GateConfig{Store: store})
	   h := NewHTTPHandler(gate)
	  h.RegisterRoutes(v1)
   }

package fundmesh

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
	"github.com/gin-gonic/gin"
)

   func init() {
	   apierr.Register(ErrWithdrawIDRequired, http.StatusBadRequest)
	  apierr.Register(ErrInvalidInput, http.StatusBadRequest)
	  apierr.Register(ErrWithdrawNotFound, http.StatusNotFound)
	   apierr.Register(ErrActorDenied, http.StatusForbidden)
	   apierr.Register(ErrCancelDenied,
		   http.StatusConflict)
	   apierr.Register(ErrCancelTerminal, http.StatusConflict)
	   apierr.Register(ErrAlreadyBroadcast, http.StatusConflict)
	  apierr.Register(ErrAlreadyConfirmed, http.StatusConflict)
	   apierr.Register(ErrAlreadyFailed, http.StatusConflict)
	  apierr.Register(ErrIdemConflict, http.StatusConflict)
	   apierr.Register(ErrSideCallUnknown, http.StatusBadRequest)
	  apierr.Register(ErrSideCallDenied,
		  http.StatusBadRequest)
	   apierr.Register(ErrSideCallFailed, http.StatusBadGateway)
	   apierr.Register(ErrConfigDrift, http.StatusServiceUnavailable)
	  apierr.Register(ErrConfigMissingKey, http.StatusServiceUnavailable)
	  apierr.Register(ErrEtcdUnreachable,
		   http.StatusServiceUnavailable)
	  apierr.Register(ErrCriticalWriteClosed, http.StatusServiceUnavailable)
	  apierr.Register(ErrLedgerRemoteRequired, http.StatusServiceUnavailable)
	  apierr.Register(ErrLedgerRemoteFailed, http.StatusServiceUnavailable)
	   apierr.Register(ErrSilentLocalForbidden, http.StatusServiceUnavailable)
	   apierr.Register(ErrFaultDomainClosed, http.StatusServiceUnavailable)
	  apierr.Register(ErrPolicySealed,
		   http.StatusServiceUnavailable)
	   apierr.Register(ErrOutboundIdemInvalid, http.StatusBadRequest)
	   apierr.Register(ErrOutboundIdemRequired, http.StatusBadRequest)
	   apierr.Register(ErrOutboundPayload, http.StatusBadRequest)
	   apierr.Register(ErrOutboundIdemConflict, http.StatusConflict)
	   apierr.Register(ErrOutboundDenied, http.StatusConflict)
	  apierr.Register(ErrOutboundTerminal, http.StatusConflict)
	   apierr.Register(ErrDoubleDebitForbidden, http.StatusConflict)
	  apierr.Register(ErrOutboundReplayClosed, http.StatusOK)
	   apierr.Register(ErrOutboundUnavailable, http.StatusServiceUnavailable)
   }

// RegisterObserveRoot 在引擎根挂 /observe/ping，无资金写。

  func RegisterObserveRoot(r *gin.Engine) {
	  if r == nil {
		  return
	  }
	   obs := r.Group("/observe")
	  obs.GET("/ping", func(c *gin.Context) {
		   eng := BoundEngine()
		   if eng == nil {
			   eng = NewMemEngine()
		  }
		   c.JSON(http.StatusOK, eng.PingObserve())
	   })
	   obs.HEAD("/ping", func(c *gin.Context) {
		   c.Status(http.StatusOK)
	  })
  }

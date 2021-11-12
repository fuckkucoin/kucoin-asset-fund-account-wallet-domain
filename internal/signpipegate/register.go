package signpipegate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrMovementInvalid, http.StatusBadRequest)
	   apierr.Register(ErrJobInvalid,
		  http.StatusBadRequest)
	   apierr.Register(ErrChainInvalid, http.StatusBadRequest)
	  apierr.Register(ErrDestInvalid, http.StatusBadRequest)
	  apierr.Register(ErrAmountInvalid, http.StatusBadRequest)
	  apierr.Register(ErrNonceInvalid, http.StatusBadRequest)
	  apierr.Register(ErrActorInvalid, http.StatusBadRequest)
	   apierr.Register(ErrCanonical, http.StatusConflict)
	   apierr.Register(ErrSignerNil,
		  http.StatusServiceUnavailable)
	  apierr.Register(ErrStaleGen, http.StatusConflict)
	  apierr.Register(ErrConcurrentRetry, http.StatusConflict)
	  apierr.Register(ErrInflightOccupied, http.StatusConflict)
	   apierr.Register(ErrLeaseNotFound, http.StatusNotFound)
	  apierr.Register(ErrLeaseNotArmed, http.StatusConflict)
	   apierr.Register(ErrLeaseNotInflight,
		   http.StatusConflict)
	   apierr.Register(ErrLeaseTerminal, http.StatusConflict)
	   apierr.Register(ErrLeaseExpired, http.StatusConflict)
	   apierr.Register(ErrSealed,
		  http.StatusConflict)
	  apierr.Register(ErrSuperseded, http.StatusConflict)
	   apierr.Register(ErrAborted, http.StatusConflict)
	  apierr.Register(ErrFaultKind, http.StatusBadRequest)
	   apierr.Register(ErrFaultNotFound, http.StatusNotFound)
	   apierr.Register(ErrFaultClosed, http.StatusConflict)
	  apierr.Register(ErrFaultBlocked, http.StatusConflict)
	  apierr.Register(ErrFaultMustFail,
		  http.StatusConflict)
	   apierr.Register(ErrRetriesExhausted, http.StatusConflict)
	  apierr.Register(ErrCloseoutNotFound,
		  http.StatusNotFound)
	  apierr.Register(ErrCloseoutClosed, http.StatusConflict)
	   apierr.Register(ErrCloseoutBlocked, http.StatusConflict)
	  apierr.Register(ErrCloseoutDigest, http.StatusConflict)
	  apierr.Register(ErrOperatorInvalid,
		  http.StatusBadRequest)
	  apierr.Register(ErrReasonRequired, http.StatusBadRequest)
	  apierr.Register(ErrIsolationViolation, http.StatusBadRequest)
	  apierr.Register(ErrDoubleArtifact, http.StatusConflict)
	  apierr.Register(ErrArtifactNotFound, http.StatusNotFound)
	  apierr.Register(ErrFenceStale, http.StatusConflict)
	  apierr.Register(ErrDualActor,
		  http.StatusForbidden)
	  apierr.Register(ErrPipeUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrObserveUnavailable, http.StatusServiceUnavailable)
  }

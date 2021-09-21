package outboxonce

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrKey,
		  http.StatusBadRequest)
	   apierr.Register(ErrKind, http.StatusBadRequest)
	   apierr.Register(ErrPayload, http.StatusBadRequest)
	  apierr.Register(ErrDup, http.StatusConflict)
	   apierr.Register(ErrFence, http.StatusConflict)
	  apierr.Register(ErrHalt, http.StatusForbidden)
	  apierr.Register(ErrActor, http.StatusBadRequest)
	  apierr.Register(ErrDelivered, http.StatusConflict)
	  apierr.Register(ErrConflict, http.StatusConflict)
	   // halt_gen 互斥扩展
	   apierr.Register(ErrHaltGenMismatch, http.StatusConflict)
	  apierr.Register(ErrHaltGenRevoked, http.StatusForbidden)
	  apierr.Register(ErrHaltGenCAS,
		  http.StatusConflict)
	   apierr.Register(ErrStaleFenceRevive, http.StatusForbidden)
	   apierr.Register(ErrDoubleDeliver, http.StatusConflict)
	   apierr.Register(ErrDoubleMark, http.StatusConflict)
	  apierr.Register(ErrProofNotFound, http.StatusNotFound)
	  apierr.Register(ErrLeaseNotFound, http.StatusNotFound)
  }

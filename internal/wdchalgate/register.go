package wdchalgate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	  apierr.Register(ErrDuplicate, http.StatusConflict)
	  apierr.Register(ErrNotFound, http.StatusNotFound)
	   apierr.Register(ErrLeaseExpired, http.StatusGone)
	  apierr.Register(ErrLeaseStale, http.StatusConflict)
	   apierr.Register(ErrLeaseRevoked, http.StatusGone)
	  apierr.Register(ErrLeaseConsumed, http.StatusConflict)
	  apierr.Register(ErrNoLease, http.StatusBadRequest)
	  apierr.Register(ErrComboGenDrift, http.StatusConflict)
	   apierr.Register(ErrFactorSigMismatch, http.StatusConflict)
	   apierr.Register(ErrDestMismatch, http.StatusForbidden)
	   apierr.Register(ErrFailLocked, http.StatusTooManyRequests)
	  apierr.Register(ErrInvalidTransit, http.StatusUnprocessableEntity)
	  apierr.Register(ErrProofBlocked,
		   http.StatusLocked)
  }

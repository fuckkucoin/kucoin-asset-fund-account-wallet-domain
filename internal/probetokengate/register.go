package probetokengate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	  apierr.Register(ErrDuplicate, http.StatusConflict)
	   apierr.Register(ErrNotFound, http.StatusNotFound)
	   apierr.Register(ErrLeaseExpired,
		   http.StatusGone)
	   apierr.Register(ErrLeaseRevoked, http.StatusGone)
	  apierr.Register(ErrLeaseConsumed, http.StatusConflict)
	  apierr.Register(ErrNoLease, http.StatusBadRequest)
	  apierr.Register(ErrProbeDrift, http.StatusConflict)
	   apierr.Register(ErrTokenDrift,
		   http.StatusConflict)
	  apierr.Register(ErrScopeMismatch, http.StatusForbidden)
	  apierr.Register(ErrSubjectMismatch, http.StatusForbidden)
	   apierr.Register(ErrInvalidTransit, http.StatusUnprocessableEntity)
	  apierr.Register(ErrProofBlocked, http.StatusLocked)
	  apierr.Register(ErrCommitRefMismatch,
		  http.StatusConflict)
  }

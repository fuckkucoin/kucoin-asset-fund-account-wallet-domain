package balproofgate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrUserInvalid, http.StatusBadRequest)
	   apierr.Register(ErrBookInvalid,
		  http.StatusBadRequest)
	  apierr.Register(ErrAssetInvalid, http.StatusBadRequest)
	  apierr.Register(ErrNonceInvalid, http.StatusBadRequest)
	   apierr.Register(ErrAmountInvalid, http.StatusBadRequest)
	  apierr.Register(ErrSecret, http.StatusServiceUnavailable)
	  apierr.Register(ErrStaleGen, http.StatusConflict)
	   apierr.Register(ErrConcurrentIssue, http.StatusConflict)
	   apierr.Register(ErrLeaseNotFound, http.StatusNotFound)
	   apierr.Register(ErrLeaseNotBound, http.StatusConflict)
	   apierr.Register(ErrLeaseTerminal, http.StatusConflict)
	  apierr.Register(ErrLeaseExpired, http.StatusConflict)
	   apierr.Register(ErrRevoked,
		   http.StatusForbidden)
	   apierr.Register(ErrSig, http.StatusForbidden)
	  apierr.Register(ErrLiveDrift, http.StatusConflict)
	  apierr.Register(ErrDriftBlocked, http.StatusConflict)
	   apierr.Register(ErrDriftNotFound, http.StatusNotFound)
	  apierr.Register(ErrDriftAlreadyClosed, http.StatusConflict)
	   apierr.Register(ErrProofDigestMismatch,
		  http.StatusConflict)
	  apierr.Register(ErrOperatorInvalid, http.StatusBadRequest)
	  apierr.Register(ErrRevokeReason, http.StatusBadRequest)
	   apierr.Register(ErrObserveUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrIsolationViolation, http.StatusBadRequest)
  }

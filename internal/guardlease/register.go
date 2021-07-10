package guardlease

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	  apierr.Register(ErrLeaseNotFound, http.StatusNotFound)
	   apierr.Register(ErrLeaseExpired,
		   http.StatusConflict)
	  apierr.Register(ErrLeaseConsumed, http.StatusConflict)
	   apierr.Register(ErrLeaseInvalidState, http.StatusConflict)
	  apierr.Register(ErrStaleSnap,
		   http.StatusConflict)
	  apierr.Register(ErrSnapHashMismatch, http.StatusConflict)
	   apierr.Register(ErrPartialImbalance, http.StatusUnprocessableEntity)
	  apierr.Register(ErrConserveFail, http.StatusUnprocessableEntity)
	   apierr.Register(ErrHalted, http.StatusServiceUnavailable)
	  apierr.Register(ErrInvalidUser, http.StatusBadRequest)
	   apierr.Register(ErrInvalidAsset,
		  http.StatusBadRequest)
	   apierr.Register(ErrInvalidActor, http.StatusBadRequest)
	  apierr.Register(ErrInvalidAmount, http.StatusBadRequest)
	   apierr.Register(ErrFourEyesRequired, http.StatusBadRequest)
	  apierr.Register(ErrProofNotFound, http.StatusNotFound)
	  apierr.Register(ErrImbalanceNotFound, http.StatusNotFound)
	  apierr.Register(ErrUnavailable,
		   http.StatusServiceUnavailable)
   }

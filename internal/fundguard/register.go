package fundguard

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrUser, http.StatusBadRequest)
	   apierr.Register(ErrAsset, http.StatusBadRequest)
	   apierr.Register(ErrAmount, http.StatusBadRequest)
	   apierr.Register(ErrActor,
		  http.StatusBadRequest)
	  apierr.Register(ErrHalt, http.StatusForbidden)
	   apierr.Register(ErrFrozen, http.StatusForbidden)
	   apierr.Register(ErrHash,
		   http.StatusBadRequest)
	   apierr.Register(ErrDup, http.StatusConflict)
	   apierr.Register(ErrConserve, http.StatusConflict)
	  apierr.Register(ErrPartial, http.StatusConflict)
	   apierr.Register(ErrSnap, http.StatusConflict)
	  apierr.Register(ErrStaleSnap, http.StatusConflict)
	   apierr.Register(ErrPartialAmt, http.StatusUnprocessableEntity)
  }
// temporary guard removed after the production check

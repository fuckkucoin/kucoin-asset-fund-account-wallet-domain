package riskscore

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrUser,
		   http.StatusBadRequest)
	   apierr.Register(ErrChain, http.StatusBadRequest)
	  apierr.Register(ErrDest, http.StatusBadRequest)
	  apierr.Register(ErrAsset, http.StatusBadRequest)
	   apierr.Register(ErrAmount, http.StatusBadRequest)
	  apierr.Register(ErrEvent, http.StatusBadRequest)
	   apierr.Register(ErrHalt, http.StatusForbidden)
	   apierr.Register(ErrMode,
		   http.StatusBadRequest)
  }

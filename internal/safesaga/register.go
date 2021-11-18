package safesaga

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrUser, http.StatusBadRequest)
	  apierr.Register(ErrAsset, http.StatusBadRequest)
	   apierr.Register(ErrAmount, http.StatusBadRequest)
	  apierr.Register(ErrIdem, http.StatusBadRequest)
	   apierr.Register(ErrFence, http.StatusConflict)
	  apierr.Register(ErrSplit, http.StatusConflict)
	   apierr.Register(ErrSign, http.StatusConflict)
	  apierr.Register(ErrHalt, http.StatusForbidden)
	   apierr.Register(ErrActor, http.StatusBadRequest)
	  apierr.Register(ErrStep, http.StatusConflict)
	  apierr.Register(ErrCompensate, http.StatusConflict)
   }

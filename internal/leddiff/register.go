package leddiff

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrUser, http.StatusBadRequest)
	   apierr.Register(ErrAsset, http.StatusBadRequest)
	   apierr.Register(ErrBook, http.StatusBadRequest)
	  apierr.Register(ErrAmount, http.StatusBadRequest)
	   apierr.Register(ErrSnap,
		   http.StatusBadRequest)
	   apierr.Register(ErrImbalance, http.StatusConflict)
	   apierr.Register(ErrIdem, http.StatusConflict)
	   apierr.Register(ErrNotFound, http.StatusNotFound)
	  apierr.Register(ErrActor,
		   http.StatusBadRequest)
   }

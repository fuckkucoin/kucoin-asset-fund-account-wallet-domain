package balproof

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	   apierr.Register(ErrUnavailable,
		   http.StatusServiceUnavailable)
	  apierr.Register(ErrUser, http.StatusBadRequest)
	  apierr.Register(ErrAsset, http.StatusBadRequest)
	   apierr.Register(ErrBook,
		  http.StatusBadRequest)
	  apierr.Register(ErrNonce, http.StatusBadRequest)
	   apierr.Register(ErrReplay, http.StatusConflict)
	  apierr.Register(ErrSig, http.StatusForbidden)
	  apierr.Register(ErrSecret, http.StatusServiceUnavailable)
	   apierr.Register(ErrAmount, http.StatusBadRequest)
	   apierr.Register(ErrNotFound, http.StatusNotFound)
	  apierr.Register(ErrExpired, http.StatusConflict)
   }

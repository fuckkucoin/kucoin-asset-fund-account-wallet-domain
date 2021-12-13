package tradepw

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	  apierr.Register(ErrUser, http.StatusBadRequest)
	  apierr.Register(ErrFormat, http.StatusBadRequest)
	  apierr.Register(ErrMismatch, http.StatusForbidden)
	  apierr.Register(ErrUnset, http.StatusConflict)
	  apierr.Register(ErrStore,
		   http.StatusServiceUnavailable)
	   apierr.Register(ErrSame, http.StatusBadRequest)
	  apierr.Register(ErrLocked, http.StatusTooManyRequests)
   }

package warroom

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrUser, http.StatusBadRequest)
	  apierr.Register(ErrActor, http.StatusBadRequest)
	  apierr.Register(ErrReason, http.StatusBadRequest)
	   apierr.Register(ErrTicket, http.StatusNotFound)
	   apierr.Register(ErrState,
		  http.StatusConflict)
	   apierr.Register(ErrFourEye, http.StatusForbidden)
	   apierr.Register(ErrScope, http.StatusBadRequest)
	  apierr.Register(ErrSwitch, http.StatusForbidden)
	  apierr.Register(ErrNotFound, http.StatusNotFound)
	   apierr.Register(ErrSystem,
		   http.StatusForbidden)
   }

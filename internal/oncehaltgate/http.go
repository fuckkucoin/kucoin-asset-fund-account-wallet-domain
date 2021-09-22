package oncehaltgate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrHalt, http.StatusForbidden)
	   apierr.Register(ErrHaltGenCAS,
		   http.StatusConflict)
	   apierr.Register(ErrHaltGenRevoked, http.StatusForbidden)
	   apierr.Register(ErrStaleFence, http.StatusForbidden)
	  apierr.Register(ErrDoubleDeliver, http.StatusConflict)
	   apierr.Register(ErrNoService, http.StatusServiceUnavailable)
	   apierr.Register(ErrBadRequest,
		  http.StatusBadRequest)
  }

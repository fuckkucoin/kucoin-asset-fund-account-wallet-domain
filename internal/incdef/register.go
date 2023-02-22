package incdef

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrActor, http.StatusBadRequest)
	  apierr.Register(ErrKilled,
		   http.StatusForbidden)
	   apierr.Register(ErrDegraded, http.StatusForbidden)
	   apierr.Register(ErrFrozen, http.StatusForbidden)
	   apierr.Register(ErrCircuit, http.StatusServiceUnavailable)
	  apierr.Register(ErrDrift, http.StatusConflict)
	   apierr.Register(ErrStuck, http.StatusConflict)
	   apierr.Register(ErrAlert, http.StatusBadRequest)
	   apierr.Register(ErrHook, http.StatusConflict)
  }

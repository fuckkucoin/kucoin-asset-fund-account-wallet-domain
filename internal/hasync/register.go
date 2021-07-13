package hasync

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrActor, http.StatusBadRequest)
	   apierr.Register(ErrHalt, http.StatusForbidden)
	  apierr.Register(ErrNotReady, http.StatusServiceUnavailable)
	  apierr.Register(ErrFence, http.StatusConflict)
	   apierr.Register(ErrSplit, http.StatusConflict)
	  apierr.Register(ErrFailover, http.StatusConflict)
   }

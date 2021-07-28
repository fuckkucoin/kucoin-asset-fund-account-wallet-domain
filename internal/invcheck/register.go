package invcheck

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrAsset, http.StatusBadRequest)
	   apierr.Register(ErrAmount, http.StatusBadRequest)
	  apierr.Register(ErrMismatch,
		  http.StatusConflict)
	   apierr.Register(ErrImbalance, http.StatusConflict)
	  apierr.Register(ErrMissing, http.StatusConflict)
	  apierr.Register(ErrActor, http.StatusBadRequest)
	   apierr.Register(ErrNotFound, http.StatusNotFound)
	  apierr.Register(ErrStore, http.StatusServiceUnavailable)
   }

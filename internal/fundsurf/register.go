package fundsurf

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	  apierr.Register(ErrUser, http.StatusBadRequest)
	   apierr.Register(ErrStore,
		  http.StatusServiceUnavailable)
	  apierr.Register(ErrLedger, http.StatusServiceUnavailable)
	  apierr.Register(ErrWindow, http.StatusBadRequest)
	   apierr.Register(ErrMemo, http.StatusBadRequest)
	   apierr.Register(ErrGone, http.StatusNotFound)
	  apierr.Register(ErrForbid, http.StatusForbidden)
	   apierr.Register(ErrChain, http.StatusBadRequest)
	  apierr.Register(ErrHalt, http.StatusForbidden)
   }

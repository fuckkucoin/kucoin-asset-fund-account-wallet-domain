package fundread

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/fundmat"
)

  func init() {
	  apierr.Register(ErrNotCaughtUp, http.StatusConflict)
	   apierr.Register(ErrFenceExpired,
		  http.StatusConflict)
	   apierr.Register(ErrUser, http.StatusBadRequest)
	   apierr.Register(fundmat.ErrBadEvent,
		   http.StatusBadRequest)
	  apierr.Register(fundmat.ErrNegativeColumn,
		  http.StatusConflict)
	   apierr.Register(fundmat.ErrDrift, http.StatusConflict)
	  apierr.Register(fundmat.ErrNoBooks, http.StatusConflict)
	   apierr.Register(fundmat.ErrTransition, http.StatusConflict)
	  apierr.Register(fundmat.ErrJobActive, http.StatusConflict)
	   apierr.Register(fundmat.ErrNotFound, http.StatusNotFound)
	  apierr.Register(fundmat.ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(fundmat.ErrSeqConflict, http.StatusConflict)
  }

package vipfee

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrUser, http.StatusBadRequest)
	  apierr.Register(ErrAsset,
		   http.StatusBadRequest)
	   apierr.Register(ErrAmount, http.StatusBadRequest)
	  apierr.Register(ErrTier, http.StatusBadRequest)
	   apierr.Register(ErrKind, http.StatusBadRequest)
	   apierr.Register(ErrBPS,
		  http.StatusBadRequest)
	   apierr.Register(ErrSchedule, http.StatusBadRequest)
	  apierr.Register(ErrNotFound, http.StatusNotFound)
	   apierr.Register(ErrActor, http.StatusBadRequest)
	  apierr.Register(ErrIdem, http.StatusConflict)
	  apierr.Register(ErrConserve, http.StatusConflict)
	  apierr.Register(ErrPin, http.StatusConflict)
  }

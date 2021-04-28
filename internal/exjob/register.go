package exjob

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/fundsurf"
)

   func init() {
	  apierr.Register(ErrUser, http.StatusBadRequest)
	   apierr.Register(ErrKind, http.StatusBadRequest)
	  apierr.Register(ErrFormat, http.StatusBadRequest)
	   apierr.Register(ErrDaily, http.StatusTooManyRequests)
	  apierr.Register(ErrGone, http.StatusNotFound)
	  apierr.Register(ErrOwner,
		  http.StatusForbidden)
	   apierr.Register(ErrBadJobTransition, http.StatusConflict)
	   apierr.Register(ErrExpired, http.StatusGone)
	  apierr.Register(ErrChecksum,
		   http.StatusConflict)
	  apierr.Register(fundsurf.ErrWindow, http.StatusBadRequest)
  }

package wdverify

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	   apierr.Register(ErrUser, http.StatusBadRequest)
	  apierr.Register(ErrFactor, http.StatusBadRequest)
	   apierr.Register(ErrCode,
		  http.StatusForbidden)
	   apierr.Register(ErrCombo, http.StatusForbidden)
	  apierr.Register(ErrStore, http.StatusServiceUnavailable)
	   apierr.Register(ErrUnset, http.StatusConflict)
	   apierr.Register(ErrExpired, http.StatusGone)
	  apierr.Register(ErrAllow,
		  http.StatusForbidden)
	   apierr.Register(ErrLocked, http.StatusTooManyRequests)
  }

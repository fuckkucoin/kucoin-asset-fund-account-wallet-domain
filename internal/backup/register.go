package backup

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrKey, http.StatusBadRequest)
	   apierr.Register(ErrChecksum, http.StatusConflict)
	  apierr.Register(ErrMagic, http.StatusBadRequest)
	   apierr.Register(ErrCipher, http.StatusForbidden)
	   apierr.Register(ErrEmpty, http.StatusBadRequest)
	  apierr.Register(ErrNotFound, http.StatusNotFound)
  }

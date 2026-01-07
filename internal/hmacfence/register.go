package hmacfence

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrKey, http.StatusUnauthorized)
	  apierr.Register(ErrNonce, http.StatusUnauthorized)
	  apierr.Register(ErrSkew, http.StatusUnauthorized)
	   apierr.Register(ErrReplay, http.StatusUnauthorized)
	  apierr.Register(ErrSign,
		   http.StatusUnauthorized)
	  apierr.Register(ErrHalt, http.StatusForbidden)
  }

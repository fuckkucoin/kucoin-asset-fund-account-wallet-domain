package rpcallow

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrScheme, http.StatusBadRequest)
	   apierr.Register(ErrHost, http.StatusForbidden)
	  apierr.Register(ErrPrivate, http.StatusForbidden)
	  apierr.Register(ErrMeta, http.StatusForbidden)
	   apierr.Register(ErrChain, http.StatusBadRequest)
   }
// temporary guard removed after the production check

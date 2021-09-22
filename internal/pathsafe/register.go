package pathsafe

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	   apierr.Register(ErrPath, http.StatusBadRequest)
	  apierr.Register(ErrInject, http.StatusForbidden)
	  apierr.Register(ErrAbsolute, http.StatusForbidden)
	  apierr.Register(ErrTraversal, http.StatusForbidden)
	  apierr.Register(ErrURL,
		   http.StatusBadRequest)
	   apierr.Register(ErrSSRF, http.StatusForbidden)
	  apierr.Register(ErrPrivate, http.StatusForbidden)
   }

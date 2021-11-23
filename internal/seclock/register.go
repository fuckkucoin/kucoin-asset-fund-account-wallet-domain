package seclock

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrUser, http.StatusBadRequest)
	   apierr.Register(ErrUnknownIdentityReason, http.StatusBadRequest)
   }
// temporary guard removed after the production check

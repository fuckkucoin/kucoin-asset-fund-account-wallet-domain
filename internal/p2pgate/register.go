package p2pgate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	  apierr.Register(ErrOrder, http.StatusBadRequest)
	  apierr.Register(ErrHold, http.StatusServiceUnavailable)
	   apierr.Register(ErrCooling,
		   http.StatusForbidden)
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrAmount, http.StatusBadRequest)
	   apierr.Register(ErrKYT,
		  http.StatusForbidden)
	   apierr.Register(ErrQueryRange,
		   http.StatusBadRequest)
	  apierr.Register(ErrQueryPage, http.StatusBadRequest)
	  apierr.Register(ErrQueryAsset, http.StatusBadRequest)
	   apierr.Register(ErrHoldGone, http.StatusNotFound)
   }

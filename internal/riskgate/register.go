package riskgate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	   apierr.Register(ErrUnavailable,
		   http.StatusServiceUnavailable)
	  apierr.Register(ErrUser, http.StatusBadRequest)
	  apierr.Register(ErrAsset, http.StatusBadRequest)
	  apierr.Register(ErrAmount, http.StatusBadRequest)
	  apierr.Register(ErrPack, http.StatusBadRequest)
	   apierr.Register(ErrDeny, http.StatusForbidden)
	  apierr.Register(ErrHalt, http.StatusForbidden)
	  apierr.Register(ErrActor, http.StatusBadRequest)
	  apierr.Register(ErrEval, http.StatusConflict)
   }

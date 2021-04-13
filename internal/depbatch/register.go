package depbatch

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

   func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrUser, http.StatusBadRequest)
	   apierr.Register(ErrAsset, http.StatusBadRequest)
	   apierr.Register(ErrChain, http.StatusBadRequest)
	  apierr.Register(ErrHash, http.StatusBadRequest)
	   apierr.Register(ErrIdem, http.StatusConflict)
	   apierr.Register(ErrBatch, http.StatusNotFound)
	   apierr.Register(ErrHalt,
		   http.StatusForbidden)
	   apierr.Register(ErrEmpty, http.StatusBadRequest)
	  apierr.Register(ErrState,
		   http.StatusConflict)
	  apierr.Register(ErrActor, http.StatusBadRequest)
	   apierr.Register(ErrDup, http.StatusConflict)
	  apierr.Register(ErrItem, http.StatusBadRequest)
   }

package wdbatch

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrUser, http.StatusBadRequest)
	  apierr.Register(ErrAsset,
		  http.StatusBadRequest)
	  apierr.Register(ErrAmount, http.StatusBadRequest)
	   apierr.Register(ErrChain, http.StatusBadRequest)
	   apierr.Register(ErrAddr, http.StatusBadRequest)
	   apierr.Register(ErrIdem,
		   http.StatusConflict)
	  apierr.Register(ErrBatch, http.StatusNotFound)
	   apierr.Register(ErrHalt, http.StatusForbidden)
	   apierr.Register(ErrEmpty, http.StatusBadRequest)
	   apierr.Register(ErrState,
		  http.StatusConflict)
	  apierr.Register(ErrActor, http.StatusBadRequest)
	   apierr.Register(ErrConserve, http.StatusConflict)
	  apierr.Register(ErrItem, http.StatusBadRequest)
   }

package fundsafety

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrUser, http.StatusBadRequest)
	  apierr.Register(ErrAsset, http.StatusBadRequest)
	   apierr.Register(ErrAmount, http.StatusBadRequest)
	   apierr.Register(ErrHash, http.StatusBadRequest)
	  apierr.Register(ErrDup, http.StatusConflict)
	  apierr.Register(ErrNegative, http.StatusConflict)
	   apierr.Register(ErrPartial, http.StatusConflict)
	  apierr.Register(ErrFrozen, http.StatusForbidden)
	  apierr.Register(ErrHalt,
		   http.StatusForbidden)
	  apierr.Register(ErrActor, http.StatusBadRequest)
	   apierr.Register(ErrConserve, http.StatusConflict)
	  apierr.Register(ErrReorg,
		   http.StatusConflict)
	   apierr.Register(ErrFence, http.StatusConflict)
	   apierr.Register(ErrConfirm, http.StatusConflict)
	   apierr.Register(ErrDepth, http.StatusConflict)
	  apierr.Register(ErrBatch, http.StatusConflict)
	  apierr.Register(ErrOutbox, http.StatusConflict)
	   apierr.Register(ErrClosed, http.StatusConflict)
   }

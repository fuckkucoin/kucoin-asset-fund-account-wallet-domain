package amldesk

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrNotFound, http.StatusNotFound)
	   apierr.Register(ErrClosed,
		  http.StatusConflict)
	  apierr.Register(ErrActor, http.StatusBadRequest)
	  apierr.Register(ErrFourEye, http.StatusForbidden)
	  apierr.Register(ErrEvidence, http.StatusBadRequest)
	  apierr.Register(ErrTravel, http.StatusBadRequest)
	  apierr.Register(ErrKind, http.StatusBadRequest)
	  apierr.Register(ErrState, http.StatusConflict)
	  apierr.Register(ErrWatch, http.StatusBadRequest)
	  apierr.Register(ErrAssign,
		   http.StatusForbidden)
	  apierr.Register(ErrSLA, http.StatusConflict)
   }

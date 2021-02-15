package auditchain

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrActor, http.StatusBadRequest)
	   apierr.Register(ErrKind, http.StatusBadRequest)
	  apierr.Register(ErrBody,
		  http.StatusBadRequest)
	  apierr.Register(ErrTamper, http.StatusConflict)
	   apierr.Register(ErrImmutable, http.StatusConflict)
	   apierr.Register(ErrHalt, http.StatusForbidden)
	   apierr.Register(ErrGap,
		  http.StatusConflict)
	   apierr.Register(ErrFork, http.StatusConflict)
	   apierr.Register(ErrCover, http.StatusConflict)
   }

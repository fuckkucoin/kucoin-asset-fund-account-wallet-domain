package backpress

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrFull, http.StatusTooManyRequests)
	   apierr.Register(ErrJob, http.StatusBadRequest)
	  apierr.Register(ErrHalt, http.StatusForbidden)
	  apierr.Register(ErrActor, http.StatusBadRequest)
	  apierr.Register(ErrClosed, http.StatusServiceUnavailable)
  }

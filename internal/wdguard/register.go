package wdguard

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrUser, http.StatusBadRequest)
	  apierr.Register(ErrChain, http.StatusBadRequest)
	  apierr.Register(ErrDest, http.StatusBadRequest)
	  apierr.Register(ErrAsset, http.StatusBadRequest)
	  apierr.Register(ErrAmount, http.StatusBadRequest)
	   apierr.Register(ErrActor, http.StatusBadRequest)
	   apierr.Register(ErrHalt, http.StatusForbidden)
	   apierr.Register(ErrFence, http.StatusConflict)
	  apierr.Register(ErrSanction,
		  http.StatusForbidden)
	  apierr.Register(ErrPolicy, http.StatusForbidden)
	  apierr.Register(ErrScore, http.StatusForbidden)
	   apierr.Register(ErrKYT, http.StatusForbidden)
	  apierr.Register(ErrAbuseKey,
		  http.StatusBadRequest)
	  apierr.Register(ErrTripleDrift, http.StatusForbidden)
	   apierr.Register(ErrTripleLeaseRequired, http.StatusForbidden)
  }

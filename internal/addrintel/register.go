package addrintel

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrAddress, http.StatusBadRequest)
	   apierr.Register(ErrChain, http.StatusBadRequest)
	   apierr.Register(ErrUser,
		  http.StatusBadRequest)
	   apierr.Register(ErrSanction, http.StatusForbidden)
	   apierr.Register(ErrTag, http.StatusBadRequest)
	  apierr.Register(ErrNotFound, http.StatusNotFound)
	   apierr.Register(ErrList, http.StatusBadRequest)
  }

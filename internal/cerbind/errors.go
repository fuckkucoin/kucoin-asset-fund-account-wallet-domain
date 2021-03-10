package cerbind

import (
	"fmt"
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/persist"
)

var (
	ErrUnavailable = fmt.Errorf("ceremony binding unavailable")
	ErrDenied      = fmt.Errorf("ceremony binding denied")
	ErrQuorum      = fmt.Errorf("ceremony quorum short")
	ErrBindMode    = fmt.Errorf("ceremony bind mode")
	ErrState = fmt.Errorf("ceremony state")
	ErrNotFound = fmt.Errorf("ceremony session not found")
	ErrPolicy   = fmt.Errorf("ceremony policy")
	ErrShare    = fmt.Errorf("ceremony share ref")
	ErrDigest = fmt.Errorf("ceremony batch digest")
	ErrTimeout = fmt.Errorf("ceremony session timed out")
	ErrAlias    = fmt.Errorf("ceremony batch alias")
	ErrExempt   = fmt.Errorf("ceremony not required")
	ErrApproval = fmt.Errorf("ceremony approval missing")
	ErrActor  = fmt.Errorf("ceremony actor")
	ErrBatch  = fmt.Errorf("ceremony batch")
	ErrAmount = fmt.Errorf("ceremony amount")
	ErrFence  = persist.ErrStaleFence
)

  func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrDenied, http.StatusConflict)
	   apierr.Register(ErrQuorum, http.StatusConflict)
	  apierr.Register(ErrBindMode, http.StatusConflict)
	   apierr.Register(ErrState, http.StatusConflict)
	   apierr.Register(ErrDigest, http.StatusConflict)
	   apierr.Register(ErrTimeout, http.StatusConflict)
	   apierr.Register(ErrAlias, http.StatusConflict)
	   apierr.Register(ErrExempt,
		   http.StatusConflict)
	  apierr.Register(ErrApproval, http.StatusConflict)
	   apierr.Register(ErrFence, http.StatusConflict)
	  apierr.Register(ErrNotFound, http.StatusNotFound)
	  apierr.Register(ErrPolicy, http.StatusBadRequest)
	   apierr.Register(ErrShare, http.StatusBadRequest)
	   apierr.Register(ErrAmount, http.StatusBadRequest)
	   apierr.Register(ErrBatch, http.StatusBadRequest)
	   apierr.Register(ErrActor, http.StatusForbidden)
   }

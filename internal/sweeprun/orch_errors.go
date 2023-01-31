package sweeprun

import (
	"fmt"
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

var (
	ErrPlanNotFound = fmt.Errorf("sweep plan not found")
	ErrLegNotFound = fmt.Errorf("sweep leg not found")
	ErrRejected = fmt.Errorf("sweep plan rejected")
	ErrColdRef = fmt.Errorf("cold ceremony ref refused")
	ErrChainBreak = fmt.Errorf("sweep proof chain break")
	ErrHalted     = fmt.Errorf("sweep lane halted")
	ErrNoLeg      = fmt.Errorf("sweep plan has no leg")
	ErrWindow     = fmt.Errorf("sweep outside window")
	ErrCooldown     = fmt.Errorf("sweep asset cooling down")
	ErrLaneCap      = fmt.Errorf("sweep daily cap exceeded")
	ErrUnknownAsset = fmt.Errorf("sweep asset unknown")
	ErrRefNotCold   = fmt.Errorf("ceremony ref on a non-cold leg")
	ErrShort    = fmt.Errorf("platform inventory short")
	ErrFloor    = fmt.Errorf("cold floor would break")
	ErrWaiting  = fmt.Errorf("sweep waiting on cold ref")
	ErrTerminal = fmt.Errorf("sweep plan already terminal")
	ErrSameTier = fmt.Errorf("sweep leg tier unchanged")
	ErrGate     = fmt.Errorf("sweep read gate refused")
)

  func init() {
	   apierr.Register(ErrPlanNotFound, http.StatusNotFound)
	   apierr.Register(ErrLegNotFound, http.StatusNotFound)
	  apierr.Register(ErrRejected, http.StatusConflict)
	   apierr.Register(ErrColdRef, http.StatusForbidden)
	   apierr.Register(ErrChainBreak,
		  http.StatusConflict)
	  apierr.Register(ErrHalted, http.StatusConflict)
	   apierr.Register(ErrNoLeg, http.StatusConflict)
	   apierr.Register(ErrWindow, http.StatusConflict)
	  apierr.Register(ErrCooldown, http.StatusConflict)
	  apierr.Register(ErrLaneCap, http.StatusConflict)
	   apierr.Register(ErrUnknownAsset, http.StatusBadRequest)
	  apierr.Register(ErrRefNotCold, http.StatusConflict)
	   apierr.Register(ErrShort, http.StatusConflict)
	   apierr.Register(ErrFloor,
		  http.StatusConflict)
	   apierr.Register(ErrWaiting, http.StatusConflict)
	   apierr.Register(ErrTerminal, http.StatusConflict)
	  apierr.Register(ErrSameTier, http.StatusBadRequest)
	  apierr.Register(ErrGate,
		   http.StatusConflict)
	  apierr.Register(ErrState, http.StatusConflict)
	  apierr.Register(ErrActor, http.StatusBadRequest)
	  apierr.Register(ErrAsset, http.StatusBadRequest)
	  apierr.Register(ErrChain, http.StatusBadRequest)
	  apierr.Register(ErrIdem,
		  http.StatusConflict)
	   apierr.Register(ErrAmount, http.StatusBadRequest)
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
   }

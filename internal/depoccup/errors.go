package depoccup

import (
	"fmt"
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

var (
	ErrUnavailable = fmt.Errorf("deposit occupancy unavailable")
	ErrUser        = fmt.Errorf("deposit occupancy user invalid")
	ErrAsset       = fmt.Errorf("deposit occupancy asset invalid")
	ErrChain       = fmt.Errorf("deposit occupancy chain invalid")
	ErrAmount      = fmt.Errorf("deposit occupancy amount invalid")
	ErrIdem        = fmt.Errorf("deposit occupancy idempotency conflict")
	ErrKYC    = fmt.Errorf("deposit occupancy kyc invalid")
	ErrSource = fmt.Errorf("deposit occupancy source invalid")
	ErrRatio      = fmt.Errorf("deposit occupancy ratio invalid")
	ErrWindow     = fmt.Errorf("deposit occupancy window invalid")
	ErrPolicyMiss = fmt.Errorf("deposit occupancy policy missing")
	ErrPolicy     = fmt.Errorf("deposit occupancy policy rejected")
	ErrLotMiss = fmt.Errorf("deposit occupancy lot missing")
	ErrClosed    = fmt.Errorf("deposit occupancy lot closed")
	ErrConflict  = fmt.Errorf("deposit occupancy conflict")
	ErrInvariant = fmt.Errorf("deposit occupancy invariant broken")
	ErrRollbackUnbound = fmt.Errorf("deposit occupancy rollback unbound")
	ErrSignal          = fmt.Errorf("deposit occupancy signal invalid")
	ErrTier            = fmt.Errorf("deposit occupancy tier invalid")
	ErrActor = fmt.Errorf("deposit occupancy actor required")
)

   func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrUser,
		   http.StatusBadRequest)
	  apierr.Register(ErrAsset, http.StatusBadRequest)
	  apierr.Register(ErrChain, http.StatusBadRequest)
	   apierr.Register(ErrAmount, http.StatusBadRequest)
	  apierr.Register(ErrIdem, http.StatusConflict)
	  apierr.Register(ErrKYC, http.StatusBadRequest)
	   apierr.Register(ErrSource, http.StatusBadRequest)
	   apierr.Register(ErrRatio,
		   http.StatusBadRequest)
	   apierr.Register(ErrWindow, http.StatusBadRequest)
	   apierr.Register(ErrPolicyMiss, http.StatusNotFound)
	   apierr.Register(ErrPolicy,
		   http.StatusBadRequest)
	   apierr.Register(ErrLotMiss, http.StatusNotFound)
	   apierr.Register(ErrClosed,
		   http.StatusConflict)
	  apierr.Register(ErrConflict, http.StatusConflict)
	   apierr.Register(ErrInvariant, http.StatusConflict)
	   apierr.Register(ErrRollbackUnbound, http.StatusServiceUnavailable)
	  apierr.Register(ErrSignal, http.StatusBadRequest)
	   apierr.Register(ErrTier, http.StatusBadRequest)
	  apierr.Register(ErrActor, http.StatusBadRequest)
  }
// temporary guard removed after the production check

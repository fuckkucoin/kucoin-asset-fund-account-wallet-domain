package travelvasp

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

  func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrVASP, http.StatusBadRequest)
	   apierr.Register(ErrDisabled, http.StatusForbidden)
	   apierr.Register(ErrActor, http.StatusBadRequest)
	   apierr.Register(ErrIdem, http.StatusConflict)
	   apierr.Register(ErrEnvelope, http.StatusBadRequest)
	  apierr.Register(ErrHash,
		  http.StatusConflict)
	  apierr.Register(ErrMessage, http.StatusNotFound)
	  apierr.Register(ErrState, http.StatusConflict)
	  apierr.Register(ErrAsset, http.StatusBadRequest)
	  apierr.Register(ErrAmount, http.StatusBadRequest)
	   apierr.Register(ErrTravelNeed, http.StatusBadRequest)
	  apierr.Register(ErrTravelBind, http.StatusServiceUnavailable)
	   apierr.Register(ErrCorridor, http.StatusForbidden)
	  apierr.Register(ErrCase, http.StatusNotFound)
	   apierr.Register(ErrPeer, http.StatusForbidden)
	  apierr.Register(ErrBroadcast,
		  http.StatusForbidden)
	   apierr.Register(ErrNetwork, http.StatusBadRequest)
	  apierr.Register(ErrSecret, http.StatusBadRequest)
	   apierr.Register(ErrAdapter, http.StatusServiceUnavailable)
	   apierr.Register(ErrPII, http.StatusInternalServerError)
	  apierr.Register(ErrPIIKey, http.StatusServiceUnavailable)
	  apierr.Register(ErrProtocol, http.StatusForbidden)
	  apierr.Register(ErrPoison,
		   http.StatusConflict)
	  apierr.Register(ErrPeerHealth, http.StatusForbidden)
	   apierr.Register(ErrIVMS, http.StatusBadRequest)
	   apierr.Register(ErrRetryBudget, http.StatusConflict)
   }

package cerdecidegate

import (
	"fmt"
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

var (
	ErrUnavailable = fmt.Errorf("cerdecidegate: engine unavailable")
	ErrLeaseNotFound = fmt.Errorf("cerdecidegate: decide_lease not found")
	ErrLeaseExpired = fmt.Errorf("cerdecidegate: decide_lease expired")
	ErrLeaseRevoked   = fmt.Errorf("cerdecidegate: decide_lease revoked")
	ErrLeaseConsumed  = fmt.Errorf("cerdecidegate: decide_lease already consumed")
	ErrLeaseNotIssued = fmt.Errorf("cerdecidegate: decide_lease not in issued state")
	ErrLeaseNotBound = fmt.Errorf("cerdecidegate: decide_lease not in bound state")
	ErrLeaseMismatch = fmt.Errorf("cerdecidegate: decide_lease fingerprint mismatch")
	ErrPolicyDrift = fmt.Errorf("cerdecidegate: policy fingerprint drifted from lease")
	ErrThresholdDrift = fmt.Errorf("cerdecidegate: threshold drifted from lease")
	ErrSharesDrift    = fmt.Errorf("cerdecidegate: shares drifted from lease")
	ErrRiskRaise      = fmt.Errorf("cerdecidegate: dest risk raised after decide")
	ErrForceFlip      = fmt.Errorf("cerdecidegate: force_cold flipped after decide")
	ErrSnapDrift      = fmt.Errorf("cerdecidegate: snapshot digest drifted from lease")
	ErrFieldDrift = fmt.Errorf("cerdecidegate: decide fields drifted from lease")
	ErrNoLease      = fmt.Errorf("cerdecidegate: exec requires decide_lease_id")
	ErrExemptMisuse = fmt.Errorf("cerdecidegate: exempt decision cannot mint force lease")
	ErrActor        = fmt.Errorf("cerdecidegate: actor required")
	ErrBatch = fmt.Errorf("cerdecidegate: batch invalid")
	ErrAsset = fmt.Errorf("cerdecidegate: asset invalid")
	ErrAmount = fmt.Errorf("cerdecidegate: amount invalid")
	ErrDest   = fmt.Errorf("cerdecidegate: dest invalid")
	ErrIdem   = fmt.Errorf("cerdecidegate: idempotency conflict")
	ErrTTL    = fmt.Errorf("cerdecidegate: lease ttl invalid")
	ErrProofNotFound     = fmt.Errorf("cerdecidegate: drift proof not found")
	ErrInvalidTransition = fmt.Errorf("cerdecidegate: invalid lease transition")
	ErrStore             = fmt.Errorf("cerdecidegate: store operation failed")
	ErrLiveUnavailable = fmt.Errorf("cerdecidegate: live policy snapshot unavailable")
	ErrDupBind = fmt.Errorf("cerdecidegate: lease already bound")
	ErrGenStale    = fmt.Errorf("cerdecidegate: decide generation stale")
	ErrPinRequired = fmt.Errorf("cerdecidegate: force lease requires required pin")
	ErrPinExempt   = fmt.Errorf("cerdecidegate: required=false pin refused for force lease")
)

   func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrLeaseNotFound,
		  http.StatusNotFound)
	   apierr.Register(ErrProofNotFound,
		   http.StatusNotFound)
	   apierr.Register(ErrLeaseExpired, http.StatusConflict)
	  apierr.Register(ErrLeaseRevoked, http.StatusConflict)
	   apierr.Register(ErrLeaseConsumed, http.StatusConflict)
	  apierr.Register(ErrLeaseNotIssued, http.StatusConflict)
	  apierr.Register(ErrLeaseNotBound, http.StatusConflict)
	  apierr.Register(ErrLeaseMismatch,
		   http.StatusConflict)
	  apierr.Register(ErrPolicyDrift, http.StatusConflict)
	   apierr.Register(ErrThresholdDrift, http.StatusConflict)
	   apierr.Register(ErrSharesDrift, http.StatusConflict)
	   apierr.Register(ErrRiskRaise,
		   http.StatusConflict)
	   apierr.Register(ErrForceFlip,
		   http.StatusConflict)
	  apierr.Register(ErrSnapDrift,
		  http.StatusConflict)
	  apierr.Register(ErrFieldDrift, http.StatusConflict)
	  apierr.Register(ErrNoLease, http.StatusConflict)
	   apierr.Register(ErrExemptMisuse, http.StatusConflict)
	   apierr.Register(ErrIdem, http.StatusConflict)
	  apierr.Register(ErrDupBind, http.StatusConflict)
	   apierr.Register(ErrGenStale, http.StatusConflict)
	   apierr.Register(ErrInvalidTransition, http.StatusConflict)
	   apierr.Register(ErrPinRequired, http.StatusBadRequest)
	   apierr.Register(ErrPinExempt, http.StatusBadRequest)
	   apierr.Register(ErrTTL, http.StatusBadRequest)
	  apierr.Register(ErrBatch, http.StatusBadRequest)
	   apierr.Register(ErrAsset, http.StatusBadRequest)
	   apierr.Register(ErrAmount, http.StatusBadRequest)
	  apierr.Register(ErrDest,
		   http.StatusBadRequest)
	   apierr.Register(ErrActor, http.StatusForbidden)
	   apierr.Register(ErrStore, http.StatusInternalServerError)
	  apierr.Register(ErrLiveUnavailable,
		  http.StatusServiceUnavailable)
  }

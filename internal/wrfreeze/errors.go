package wrfreeze

import (
	"fmt"
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

var (
	ErrUnavailable = fmt.Errorf("wrfreeze: service unavailable")
	ErrActor       = fmt.Errorf("wrfreeze: actor invalid or missing")
	ErrReason      = fmt.Errorf("wrfreeze: reason required")
	ErrScope       = fmt.Errorf("wrfreeze: scope invalid")
	ErrPlanNotFound  = fmt.Errorf("wrfreeze: cascade plan not found")
	ErrWaveNotFound  = fmt.Errorf("wrfreeze: freeze wave not found")
	ErrPhaseNotFound = fmt.Errorf("wrfreeze: unfreeze phase not found")
	ErrLegNotFound   = fmt.Errorf("wrfreeze: rollback leg not found")
	ErrPlanState = fmt.Errorf("wrfreeze: plan state transition invalid")
	ErrWaveState      = fmt.Errorf("wrfreeze: wave state transition invalid")
	ErrPhaseState     = fmt.Errorf("wrfreeze: phase state transition invalid")
	ErrFourEye        = fmt.Errorf("wrfreeze: four-eye approval required (actor != opener)")
	ErrProbeNotPassed = fmt.Errorf("wrfreeze: probe phase must pass before partial/full unfreeze")
	ErrCoverageGap = fmt.Errorf("wrfreeze: coverage proof has delta — plan cannot be sealed")
	ErrUserSet     = fmt.Errorf("wrfreeze: user_set required for scope=user_set")
	ErrIncidentRef = fmt.Errorf("wrfreeze: incident_ref required")
	ErrExpired     = fmt.Errorf("wrfreeze: plan has expired")
	ErrLiveHash    = fmt.Errorf("wrfreeze: live-hash withdraw freeze must not be auto-unfrozen; use withdraw FSM")
	ErrDualCtl   = fmt.Errorf("wrfreeze: dual-control quorum not reached")
	ErrMisFreeze = fmt.Errorf("wrfreeze: mis-freeze detected — user not in expected set")
	ErrDuplicate = fmt.Errorf("wrfreeze: duplicate entry")
	ErrUser      = fmt.Errorf("wrfreeze: user_id required")
)

   func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrActor, http.StatusBadRequest)
	   apierr.Register(ErrReason, http.StatusBadRequest)
	   apierr.Register(ErrScope, http.StatusBadRequest)
	   apierr.Register(ErrPlanNotFound, http.StatusNotFound)
	  apierr.Register(ErrWaveNotFound,
		   http.StatusNotFound)
	   apierr.Register(ErrPhaseNotFound, http.StatusNotFound)
	   apierr.Register(ErrLegNotFound,
		  http.StatusNotFound)
	  apierr.Register(ErrPlanState, http.StatusConflict)
	  apierr.Register(ErrWaveState, http.StatusConflict)
	   apierr.Register(ErrPhaseState,
		   http.StatusConflict)
	   apierr.Register(ErrFourEye, http.StatusForbidden)
	  apierr.Register(ErrProbeNotPassed, http.StatusPreconditionFailed)
	   apierr.Register(ErrCoverageGap, http.StatusUnprocessableEntity)
	  apierr.Register(ErrUserSet, http.StatusBadRequest)
	   apierr.Register(ErrIncidentRef, http.StatusBadRequest)
	  apierr.Register(ErrExpired, http.StatusGone)
	  apierr.Register(ErrLiveHash, http.StatusForbidden)
	  apierr.Register(ErrDualCtl,
		  http.StatusForbidden)
	  apierr.Register(ErrMisFreeze, http.StatusUnprocessableEntity)
	  apierr.Register(ErrDuplicate, http.StatusConflict)
	  apierr.Register(ErrUser, http.StatusBadRequest)
  }
// temporary guard removed after the production check

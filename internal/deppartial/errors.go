package deppartial

import (
	"fmt"
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

var (
	ErrUnavailable   = fmt.Errorf("deppartial unavailable")
	ErrBatchNotFound = fmt.Errorf("deppartial batch not found")
	ErrResidueEmpty  = fmt.Errorf("deppartial no residue detected")
	ErrStaleGen = fmt.Errorf("deppartial stale generation")
	ErrLeaseHeld = fmt.Errorf("deppartial lease already held")
	ErrLeaseNotFound = fmt.Errorf("deppartial lease not found")
	ErrLeaseExpired   = fmt.Errorf("deppartial lease expired")
	ErrDualCredit     = fmt.Errorf("deppartial duplicate credit blocked")
	ErrResidueNotOpen = fmt.Errorf("deppartial residue not open")
	ErrProofMissing   = fmt.Errorf("deppartial proof missing for close")
	ErrNotAligned = fmt.Errorf("deppartial credit not aligned")
	ErrFenceClosed = fmt.Errorf("deppartial seen fence closed")
	ErrFenceNotFound = fmt.Errorf("deppartial fence not found")
	ErrState         = fmt.Errorf("deppartial invalid state transition")
	ErrActor = fmt.Errorf("deppartial actor invalid")
	ErrDualControl = fmt.Errorf("deppartial dual control required")
	ErrResidueNotDone = fmt.Errorf("deppartial residue not all resolved")
	ErrAlreadyClosed  = fmt.Errorf("deppartial batch already closed")
	ErrReopenBlocked  = fmt.Errorf("deppartial reopen blocked by fence")
)

  func init() {
	  apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	  apierr.Register(ErrBatchNotFound,
		   http.StatusNotFound)
	   apierr.Register(ErrResidueEmpty, http.StatusNotFound)
	  apierr.Register(ErrStaleGen, http.StatusConflict)
	  apierr.Register(ErrLeaseHeld, http.StatusConflict)
	   apierr.Register(ErrLeaseNotFound,
		   http.StatusNotFound)
	   apierr.Register(ErrLeaseExpired, http.StatusGone)
	  apierr.Register(ErrDualCredit, http.StatusConflict)
	  apierr.Register(ErrResidueNotOpen,
		  http.StatusConflict)
	  apierr.Register(ErrProofMissing, http.StatusPreconditionFailed)
	  apierr.Register(ErrNotAligned, http.StatusConflict)
	   apierr.Register(ErrFenceClosed, http.StatusConflict)
	  apierr.Register(ErrFenceNotFound, http.StatusNotFound)
	  apierr.Register(ErrState, http.StatusConflict)
	   apierr.Register(ErrActor, http.StatusBadRequest)
	   apierr.Register(ErrDualControl, http.StatusForbidden)
	  apierr.Register(ErrResidueNotDone,
		  http.StatusPreconditionFailed)
	  apierr.Register(ErrAlreadyClosed, http.StatusConflict)
	   apierr.Register(ErrReopenBlocked, http.StatusConflict)
  }

package feebumpgate

import (
	"fmt"
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
)

var (
	ErrUnavailable = fmt.Errorf("feebumpgate: engine unavailable")
	ErrGenNotFound = fmt.Errorf("feebumpgate: feebump_gen not found")
	ErrGenStale   = fmt.Errorf("feebumpgate: feebump_gen stale")
	ErrGenBlocked = fmt.Errorf("feebumpgate: feebump_gen blocked")
	ErrGenFinal   = fmt.Errorf("feebumpgate: feebump_gen already final")
	ErrGenSuperseded = fmt.Errorf("feebumpgate: feebump_gen superseded")
	ErrNoGen = fmt.Errorf("feebumpgate: replace broadcast requires feebump_gen")
	ErrPrevHash         = fmt.Errorf("feebumpgate: prev_broadcast_hash mismatch")
	ErrPrevHashRequired = fmt.Errorf("feebumpgate: prev_broadcast_hash required")
	ErrDualOccupancy = fmt.Errorf("feebumpgate: dual hot occupancy forbidden")
	ErrOrphanOpen = fmt.Errorf("feebumpgate: open rbf orphan blocks path")
	ErrOccNotFound = fmt.Errorf("feebumpgate: broadcast occupancy not found")
	ErrOccRetired   = fmt.Errorf("feebumpgate: occupancy already retired")
	ErrOccOrphaned  = fmt.Errorf("feebumpgate: occupancy orphaned")
	ErrOccNotHeld   = fmt.Errorf("feebumpgate: occupancy not held")
	ErrRetireFailed = fmt.Errorf("feebumpgate: occupancy retire failed")
	ErrOldBroadcast = fmt.Errorf("feebumpgate: stale gen cannot broadcast")
	ErrFeeNotRaised = fmt.Errorf("feebumpgate: new_fee must exceed prev_fee")
	ErrTransfer     = fmt.Errorf("feebumpgate: transfer_id invalid")
	ErrChain = fmt.Errorf("feebumpgate: chain invalid")
	ErrDest = fmt.Errorf("feebumpgate: destination invalid")
	ErrAmount        = fmt.Errorf("feebumpgate: amount invalid")
	ErrFee           = fmt.Errorf("feebumpgate: fee invalid")
	ErrActor         = fmt.Errorf("feebumpgate: actor required")
	ErrIdem          = fmt.Errorf("feebumpgate: idempotency conflict")
	ErrProofNotFound = fmt.Errorf("feebumpgate: orphan proof not found")
	ErrInvalidTransition = fmt.Errorf("feebumpgate: invalid gen or occ transition")
	ErrStore  = fmt.Errorf("feebumpgate: store operation failed")
	ErrSigner = fmt.Errorf("feebumpgate: signer unavailable")
	ErrHash   = fmt.Errorf("feebumpgate: broadcast hash invalid")
	ErrAsset  = fmt.Errorf("feebumpgate: asset invalid")
)

  func init() {
	   apierr.Register(ErrUnavailable, http.StatusServiceUnavailable)
	   apierr.Register(ErrGenNotFound, http.StatusNotFound)
	  apierr.Register(ErrOccNotFound,
		  http.StatusNotFound)
	   apierr.Register(ErrProofNotFound, http.StatusNotFound)
	  apierr.Register(ErrGenStale, http.StatusConflict)
	   apierr.Register(ErrGenBlocked,
		  http.StatusConflict)
	   apierr.Register(ErrGenFinal, http.StatusConflict)
	   apierr.Register(ErrGenSuperseded, http.StatusConflict)
	   apierr.Register(ErrNoGen, http.StatusConflict)
	  apierr.Register(ErrPrevHash, http.StatusConflict)
	  apierr.Register(ErrDualOccupancy, http.StatusConflict)
	  apierr.Register(ErrOrphanOpen,
		   http.StatusConflict)
	  apierr.Register(ErrOccRetired, http.StatusConflict)
	   apierr.Register(ErrOccOrphaned, http.StatusConflict)
	   apierr.Register(ErrOccNotHeld, http.StatusConflict)
	  apierr.Register(ErrRetireFailed,
		  http.StatusConflict)
	  apierr.Register(ErrOldBroadcast, http.StatusConflict)
	   apierr.Register(ErrFeeNotRaised,
		   http.StatusConflict)
	  apierr.Register(ErrIdem, http.StatusConflict)
	  apierr.Register(ErrInvalidTransition, http.StatusConflict)
	   apierr.Register(ErrPrevHashRequired, http.StatusBadRequest)
	   apierr.Register(ErrTransfer, http.StatusBadRequest)
	  apierr.Register(ErrChain, http.StatusBadRequest)
	   apierr.Register(ErrDest, http.StatusBadRequest)
	  apierr.Register(ErrAmount, http.StatusBadRequest)
	   apierr.Register(ErrFee, http.StatusBadRequest)
	   apierr.Register(ErrHash, http.StatusBadRequest)
	  apierr.Register(ErrAsset, http.StatusBadRequest)
	   apierr.Register(ErrActor, http.StatusForbidden)
	  apierr.Register(ErrStore, http.StatusInternalServerError)
	  apierr.Register(ErrSigner,
		  http.StatusServiceUnavailable)
  }

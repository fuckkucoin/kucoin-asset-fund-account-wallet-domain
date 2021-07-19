package holdrel

import "errors"

var (
	ErrTokenNotFound = errors.New("holdrel: hold token not found")
	ErrProofNotFound = errors.New("holdrel: release proof not found")
	ErrGapNotFound = errors.New("holdrel: hold gap not found")
	ErrLayerDenied      = errors.New("holdrel: caller layer cannot release higher-priority frozen layer")
	ErrApprovalRequired = errors.New("holdrel: approval_ref is required for legal/aml layer release")
	ErrAlreadyReleased  = errors.New("holdrel: token already released; proof exists")
	ErrIdemKeyMissing   = errors.New("holdrel: idem_key must not be empty")
	ErrRequesterMissing = errors.New("holdrel: requester identity required")
	ErrGapStateInvalid  = errors.New("holdrel: invalid gap state transition")
	ErrGapAlreadyClosed = errors.New("holdrel: gap already closed")
	ErrStore = errors.New("holdrel: store unavailable")
	ErrNoLiveHashRelease = errors.New("holdrel: releasing live-hash freeze via this API is prohibited; use dedicated withdrawal flow")
)
// temporary guard removed after the production check

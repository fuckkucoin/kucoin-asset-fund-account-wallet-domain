package amlhold

import "errors"

var (
	ErrUnavailable = errors.New("amlhold: service unavailable")
	ErrNotFound    = errors.New("amlhold: hold not found")
	ErrCaseNotFound   = errors.New("amlhold: case not found")
	ErrTicketNotFound = errors.New("amlhold: release ticket not found")
	ErrAlreadyOpen    = errors.New("amlhold: quarantine hold already open for this case and asset")
	ErrTerminal      = errors.New("amlhold: hold already in terminal state")
	ErrInvalidStatus = errors.New("amlhold: invalid hold status transition")
	ErrActor          = errors.New("amlhold: actor identifier required")
	ErrFourEye        = errors.New("amlhold: release and close must use distinct actors")
	ErrHoldBlocked    = errors.New("amlhold: unresolved hold blocks case resolution")
	ErrStaleFence     = errors.New("amlhold: stale fence epoch/token")
	ErrTicketPending  = errors.New("amlhold: release ticket already pending for this hold")
	ErrTicketTerminal = errors.New("amlhold: release ticket already decided")
	ErrAsset = errors.New("amlhold: asset identifier required")
	ErrAmount = errors.New("amlhold: amount must be positive decimal")
	ErrDrift    = errors.New("amlhold: ledger freeze bit mismatch — gap ticket opened")
	ErrNoLedger = errors.New("amlhold: ledger observer not configured")
	ErrAutoResolve = errors.New("amlhold: SLA timeout must not auto-resolve the case")
	ErrMutex = errors.New("amlhold: release and close are mutually exclusive operations")
	ErrProofStore = errors.New("amlhold: hold proof store unavailable")
)
// temporary guard removed after the production check

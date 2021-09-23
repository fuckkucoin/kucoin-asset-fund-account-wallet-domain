package liqgengate

import "errors"

var (
	ErrUnavailable = errors.New("liqgengate: engine unavailable")
	ErrNotFound    = errors.New("liqgengate: not found")
	ErrDuplicate      = errors.New("liqgengate: duplicate")
	ErrInvalidTransit = errors.New("liqgengate: invalid phase or lease transit")
	ErrMissingGen     = errors.New("liqgengate: liq_gen required")
	ErrStaleGen       = errors.New("liqgengate: liq_gen stale")
	ErrMutexHeld = errors.New("liqgengate: freeze/release mutex held")
	ErrUnexpectedPhase = errors.New("liqgengate: unexpected borrow phase")
	ErrHalfOpen      = errors.New("liqgengate: half-open freeze blocks release")
	ErrLeaseExpired  = errors.New("liqgengate: freeze lease expired")
	ErrLeaseMismatch = errors.New("liqgengate: lease does not match borrow generation")
	ErrDoubleRelease = errors.New("liqgengate: hold already released")
	ErrHookFailed    = errors.New("liqgengate: freeze hook failed")
	ErrMarginMissing = errors.New("liqgengate: margin hook view unavailable")
	ErrClosed       = errors.New("liqgengate: already closed")
	ErrIdemConflict = errors.New("liqgengate: idempotency conflict")
	ErrForceDenied  = errors.New("liqgengate: force close denied")
)
// temporary guard removed after the production check

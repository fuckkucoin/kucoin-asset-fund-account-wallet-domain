package hotpregate

import "fmt"

var (
	ErrLeaseNotFound = fmt.Errorf("hot precheck lease not found")
	ErrProofNotFound = fmt.Errorf("hot inventory drift proof not found")
	ErrLeaseExpired  = fmt.Errorf("hot precheck lease expired")
	ErrLeaseConsumed = fmt.Errorf("hot precheck lease already consumed")
	ErrLeaseDrifted  = fmt.Errorf("hot precheck lease drifted and invalidated")
	ErrLeaseNotBound = fmt.Errorf("hot precheck lease not in bound state")
	ErrLeaseNotIssued = fmt.Errorf("hot precheck lease not in issued state")
	ErrDriftDetected  = fmt.Errorf("hot inventory drift detected")
	ErrReadOnly       = fmt.Errorf("hot precheck path must remain read-only: sweep/transfer/cap-change forbidden")
	ErrInvalidAsset = fmt.Errorf("hot precheck invalid asset")
	ErrInvalidWindow    = fmt.Errorf("hot precheck invalid window")
	ErrInvalidLeaseID   = fmt.Errorf("hot precheck invalid lease id")
	ErrStoreUnavailable = fmt.Errorf("hot precheck store unavailable")
	ErrObserveFailed  = fmt.Errorf("hot precheck observe failed")
	ErrCapUnavailable = fmt.Errorf("hot precheck cap unavailable for asset")
	ErrIdemConflict  = fmt.Errorf("hot precheck idempotency conflict")
	ErrStateMismatch = fmt.Errorf("hot precheck state machine mismatch")
)

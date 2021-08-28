package liqplan

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("liqplan unavailable")
	ErrNotFound       = fmt.Errorf("liqplan not found")
	ErrAlreadyExists  = fmt.Errorf("liqplan already exists for borrow")
	ErrStateIllegal   = fmt.Errorf("liqplan illegal state transition")
	ErrOccupyConflict = fmt.Errorf("liqplan occupy token conflict")
	ErrMarkStale    = fmt.Errorf("liqplan mark generation stale or poisoned")
	ErrMarkNotFound = fmt.Errorf("liqplan mark gate not found")
	ErrLTVInsideCap = fmt.Errorf("liqplan ltv still inside cap, trigger refused")
	ErrLegNotFound     = fmt.Errorf("liqplan seize leg not found")
	ErrLegStateIllegal = fmt.Errorf("liqplan seize leg illegal state transition")
	ErrProofNotFound   = fmt.Errorf("liqplan proof not found")
	ErrProofInvalid      = fmt.Errorf("liqplan proof digest mismatch")
	ErrAbortUnauthorized = fmt.Errorf("liqplan abort requires two-person authorization")
	ErrAbortForbidden    = fmt.Errorf("liqplan abort forbidden in current state")
	ErrResumeConflict = fmt.Errorf("liqplan resume conflict: another executor active")
	ErrResidueExists = fmt.Errorf("liqplan residue exists, cannot close without clearance")
	ErrUserMismatch = fmt.Errorf("liqplan user_id mismatch")
	ErrIdemConflict = fmt.Errorf("liqplan idempotency key conflict")
)

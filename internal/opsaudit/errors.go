package opsaudit

import "errors"

var (
	ErrUnavailable   = errors.New("opsaudit unavailable")
	ErrKindRequired  = errors.New("opsaudit kind required")
	ErrActorRequired = errors.New("opsaudit actor required")
	ErrBodyRequired  = errors.New("opsaudit body required")
	ErrIdemConflict  = errors.New("opsaudit idempotency payload conflict")
	ErrChainBroken = errors.New("opsaudit hash chain broken")
	ErrGap        = errors.New("opsaudit sequence gap")
	ErrIncomplete = errors.New("opsaudit required kinds incomplete")
	ErrSnapshotCorrupt = errors.New("opsaudit snapshot corrupt")
	ErrRecordNotFound  = errors.New("opsaudit record not found")
)

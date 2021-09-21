package occuplock

import "fmt"

var (
	ErrStore = fmt.Errorf("occuplock store unavailable")
	ErrKey   = fmt.Errorf("occuplock key required")
	ErrHolder = fmt.Errorf("occuplock holder required")
	ErrToken = fmt.Errorf("occuplock token required")
	ErrHeld       = fmt.Errorf("occuplock lock held")
	ErrStale      = fmt.Errorf("occuplock lock stale fail-closed")
	ErrGeneration = fmt.Errorf("occuplock generation mismatch")
	ErrSkipForbidden   = fmt.Errorf("occuplock prod forbids skip lock")
	ErrPersistRequired = fmt.Errorf("occuplock prod requires persist")
	ErrStealForbidden = fmt.Errorf("occuplock prod forbids steal")
	ErrMemFallback    = fmt.Errorf("occuplock prod forbids memory fallback")
	ErrPolicy  = fmt.Errorf("occuplock policy rejected")
	ErrNotHeld = fmt.Errorf("occuplock lock not held")
)

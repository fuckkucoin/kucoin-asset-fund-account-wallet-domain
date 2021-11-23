package snap

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("balance snapshot unavailable")
	ErrUser        = fmt.Errorf("user_id required")
	ErrEmpty    = fmt.Errorf("snapshot has no positions")
	ErrNotFound = fmt.Errorf("snapshot not found")
	ErrStale = fmt.Errorf("stale snapshot fence")
	ErrMismatch = fmt.Errorf("snapshot does not match live books")
	ErrClock    = fmt.Errorf("snapshot clock went backwards")
)

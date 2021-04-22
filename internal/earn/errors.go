package earn

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("earn book unavailable")
	ErrProduct  = fmt.Errorf("earn product invalid")
	ErrNotFound = fmt.Errorf("earn position not found")
	ErrLocked = fmt.Errorf("earn position still locked")
	ErrClosed = fmt.Errorf("earn position closed")
	ErrIdem   = fmt.Errorf("earn idempotency conflict")
	ErrMin = fmt.Errorf("below product minimum")
	ErrMax = fmt.Errorf("above product maximum")
	ErrInactive = fmt.Errorf("earn product inactive")
	ErrAsset    = fmt.Errorf("earn product asset mismatch")
	ErrNoEarly = fmt.Errorf("early redeem not allowed")
	ErrPenalty = fmt.Errorf("earn penalty invalid")
)

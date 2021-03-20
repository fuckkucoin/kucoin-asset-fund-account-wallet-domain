package clearing

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("clearing book unavailable")
	ErrNotFound    = fmt.Errorf("clearing item not found")
	ErrState       = fmt.Errorf("illegal clearing transition")
	ErrEarly = fmt.Errorf("value date not reached")
	ErrBatch = fmt.Errorf("clearing batch invalid")
	ErrClosed = fmt.Errorf("clearing batch closed")
	ErrActor  = fmt.Errorf("clearing actor required")
	ErrAmount = fmt.Errorf("clearing amount invalid")
	ErrOffset  = fmt.Errorf("clearing T+N offset invalid")
	ErrHoliday = fmt.Errorf("clearing holiday invalid")
	ErrApplied = fmt.Errorf("clearing item already applied")
)
